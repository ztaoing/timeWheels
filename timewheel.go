/**
* @Author:zhoutao
* @Date:2021/3/24 下午2:55
* @Desc:
 */

package timewheels

import (
	"github.com/ztaoing/timeWheels/delayqueue"
	"sync/atomic"
	"time"
	"unsafe"
)

type TimeWheel struct {
	tick          int64     //时间跨度，单位毫秒
	wheelSize     int64     //时间轮的个数
	interval      int64     //总跨度,单个时间的轮的总时间，例如：第一层时间轮的总时间为10ms
	currentTime   int64     //当前时间
	buckets       []*bucket //时间格列表,存储着相同时间的任务队列,到期后会从队列中取出来执行
	queue         *delayqueue.DelayQueue
	overflowWheel unsafe.Pointer //上层时间轮的引用
	exitC         chan struct{}  //接收结束信号的chan
	waitGroup     waitGroupWrapper
}

func newTimeWheel(tickMs int64, wheelSize int64, startMs int64, queue *delayqueue.DelayQueue) *TimeWheel {
	buckets := make([]*bucket, wheelSize)
	//初始化每个bucket
	for i := range buckets {
		buckets[i] = newBucket()
	}
	return &TimeWheel{
		tick:        tickMs,
		wheelSize:   wheelSize,
		currentTime: truncate(startMs, tickMs),
		interval:    tickMs * wheelSize,
		buckets:     buckets,
		queue:       queue,
		exitC:       make(chan struct{}),
	}
}

func (t *TimeWheel) Start() {
	//在无限循环中，将到期的元素放入到队列的C中
	t.waitGroup.Wrap(func() {
		t.queue.Poll(t.exitC, func() int64 {
			return timeToMs(time.Now().UTC())
		})
	})

	//开启无限循环获取队列中的C的数据,即到期的数据
	t.waitGroup.Wrap(func() {
		for {
			select {
			//获取到期的数据
			case elem := <-t.queue.C:
				b := elem.(*bucket)
				//时间轮会将当前时间往前移动到bucket的到期时间
				t.advanceClock(b.Expiration())
				//取出bucket队列中的数据，并调用addOrRun方法执行
				b.Flush(t.addOrRun)
			case <-t.exitC:
				return
			}
		}
	})
}

//停止当前的时间轮
func (t *TimeWheel) Stop() {
	//向exitC中发送信号
	close(t.exitC)
	//等待所有goroutine结束
	t.waitGroup.Wait()
}

//将当前时间移动到bucket的到期时间,根据到期时间重新设置currentTime,从而推进时间轮
func (t *TimeWheel) advanceClock(expiration int64) {
	currentTime := atomic.LoadInt64(&t.currentTime)
	//过期时间 >= 当前时间+tick
	if expiration >= currentTime+t.tick {
		//将currentTime设置为expiration,来推进currentTime
		currentTime = truncate(expiration, t.tick)
		atomic.StoreInt64(&t.currentTime, currentTime)

		//如果有上层时间轮，就递归调用上层时间轮的引用
		overflowWheel := atomic.LoadPointer(&t.overflowWheel)
		if overflowWheel != nil {
			(*TimeWheel)(overflowWheel).advanceClock(currentTime)
		}
	}
}

func (t *TimeWheel) addOrRun(time *Timer) {
	//如果已经过期，则直接执行
	if !t.add(time) {
		//异步执行
		go time.task()
	}
}

//根据任务的到期时间和需要执行的任务封装成task
func (t *TimeWheel) AfterFunc(d time.Duration, f func()) *Timer {
	task := &Timer{
		expiration: timeToMs(time.Now().UTC().Add(d)),
		task:       f,
	}
	t.addOrRun(task)
	return task
}

//根据到期时间分成三类：
//1:小于当前时间+tick,表示已经到期，返回false，执行任务即可
//2:判断expiration是否小于时间轮的跨度，如果小于，则表定时任务放入到当前的时间轮中，通过取模找到buckets的时间格，并放入到bucket队列中
//3:该定时任务的时间跨度超过了当前时间轮，需要升级到上一层时间轮中。上一层的时间轮的tick是当前时间轮的interval
func (t *TimeWheel) add(time *Timer) bool {
	currentTime := atomic.LoadInt64(&t.currentTime)
	//已经过期
	if time.expiration < currentTime+t.tick {
		//已经过期
		return false
	} else if time.expiration < currentTime+t.interval {
		//任务的到期时间在第一层环中
		//时间轮的位置
		virtualID := time.expiration / t.tick
		//通过取模获取位置
		b := t.buckets[virtualID%t.wheelSize]
		//加入到bucket任务队列中
		b.Add(time)
		//如果是相同的时间，则返回false，防止被多次插入到队列中
		if b.SetExpiration(virtualID * t.tick) {
			//将该bucket加入到延迟队列中
			t.queue.Offer(b, b.expiration)
		}
		return true
	} else {
		//如果此到期任务的时间超过第一层时间轮，则放到上一层中
		overflowWheel := atomic.LoadPointer(&t.overflowWheel)
		if overflowWheel == nil {
			atomic.CompareAndSwapPointer(&t.overflowWheel,
				nil,
				unsafe.Pointer(newTimeWheel(t.interval, t.wheelSize, currentTime, t.queue)),
			)
			overflowWheel = atomic.LoadPointer(&t.overflowWheel)
		}
		//放入到上层
		//向上递归
		return (*TimeWheel)(overflowWheel).add(time)
	}
}

//决定了当前任务的执行计划
type Scheduler interface {
	//Next 返回给定时间的下一个执行时间
	//如果没有下一个要调度的时间的任务，就返回0
	//使用的时间必须是UTC，即本地时间
	Next(time.Time) time.Time
}

//s为实现了Scheduler接口的所有对象
//根据s自己的执行计划调用f，返回的是一个Timer，它可以通过自己的cancel方法停止此Timer的运行
//如果调用者想决定定正在执行中的任务的话，它必须停止timer，并且确保timer停止才可以，从当前的实现中，在过期时间和重启时间之间有一个间隔，这个等待的时间是为了确保时间是短的，因为这个间隔很短。
//
func (t *TimeWheel) ScheduleFunc(s Scheduler, f func()) (timer *Timer) {
	expiration := s.Next(time.Now().UTC())
	if expiration.IsZero() {
		//没有timer被调度
		return
	}
	timer = &Timer{
		expiration: timeToMs(expiration),
		task: func() {
			//在下一个时间，调度任务去执行
			expiration := s.Next(msToTime(timer.expiration))
			if !expiration.IsZero() {
				timer.expiration = timeToMs(expiration)
				t.addOrRun(timer)
			}
			//执行任务
			f()
		},
	}
	t.addOrRun(timer)
	return
}
