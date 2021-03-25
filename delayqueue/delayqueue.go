/**
* @Author:zhoutao
* @Date:2021/3/24 下午3:27
* @Desc:
 */

package delayqueue

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"time"
)

// 优先级队列的实现，借鉴了https://github.com/nsqio/nsq/blob/master/internal/pqueue/pqueue.go

type item struct {
	Value    interface{}
	Priority int64 //权重
	Index    int
}

//这是一个使用最小堆实现的优先级队列
//第0个元素是最小值

type priorityQueue []*item

func newPriorityQueue(capacity int) priorityQueue {
	return make(priorityQueue, 0, capacity)
}

func (p priorityQueue) Len() int {
	return len(p)
}

func (p priorityQueue) Less(i, j int) bool {
	return p[i].Priority < p[j].Priority
}

func (p priorityQueue) Swap(i, j int) {
	//交换值
	p[i], p[j] = p[j], p[i]
	//交换索引
	p[i].Index = i
	p[j].Index = j
}

func (p *priorityQueue) Push(x interface{}) {
	n := len(*p)
	c := cap(*p)
	//存储的数量 > 容量
	if n+1 > c {
		//扩容
		np := make(priorityQueue, n, c*2)
		//数据迁移到扩容后的地址
		copy(np, *p)
		//切换
		*p = np
	}
	//todo
	*p = (*p)[0 : n+1]
	item := x.(*item)
	//设置当前item的索引号
	item.Index = n
	//add
	(*p)[n] = item
}

func (p *priorityQueue) Pop() interface{} {
	n := len(*p)
	c := cap(*p)
	//缩容
	if n < (c/2) && c > 25 {
		np := make(priorityQueue, n, c/2)
		copy(np, *p)
		*p = np
	}
	//获取最后一个元素
	item := (*p)[n-1]
	item.Index = -1
	//删除最后一个元素
	*p = (*p)[0 : n-1]
	return item
}

//查找然后移动
func (p *priorityQueue) PeekAndShift(max int64) (*item, int64) {
	if p.Len() == 0 {
		return nil, 0
	}
	//第一个元素
	item := (*p)[0]
	//第一个元素的优先级更大
	if item.Priority > max {
		return nil, item.Priority - max
	}
	//max 更大,移除第一个元素
	heap.Remove(p, 0)
	return item, 0
}

type DelayQueue struct {
	C chan interface{}

	mu sync.Mutex
	pq priorityQueue

	sleeping int32 //类似runtime.timers的sleeping状态  0唤醒状态 1睡眠状态
	wakeupC  chan struct{}
}

//设置指定大小的延迟队列的实例
func New(size int) *DelayQueue {
	return &DelayQueue{
		C:       make(chan interface{}),
		pq:      newPriorityQueue(size),
		wakeupC: make(chan struct{}),
	}
}

//插入元素到当前的队列
func (d *DelayQueue) Offer(element interface{}, expiration int64) {
	item := item{Value: element, Priority: expiration}

	//lock
	d.mu.Lock()
	//将item加入延迟队列
	heap.Push(&d.pq, item)
	index := item.Index
	d.mu.Unlock()
	//增加了一个未在延迟队列出现的新元素
	if index == 0 {
		//切换为唤醒状态
		if atomic.CompareAndSwapInt32(&d.sleeping, 1, 0) {
			//唤起
			d.wakeupC <- struct{}{}
		}
	}
}

//使用无限循环，等待一个元素的过期，然后发送过期的元素的元素到 延迟队列的channel C
func (d *DelayQueue) Poll(exitC chan struct{}, nowF func() int64) {
	for {
		now := nowF()

		//lock
		d.mu.Lock()
		//todo 当now比第一个元素大时，移除第一个元素
		item, delta := d.pq.PeekAndShift(now)
		if item == nil {

			//我们必须确保在整个操作中，PeekAndShift和StoreInt32的原子性，避免在Offer和Poll之间可能发送的竞争

			//没有item，就sleep一会
			atomic.StoreInt32(&d.sleeping, 1)
		}
		d.mu.Unlock()

		if item == nil {
			if delta == 0 {
				//没有剩余的item
				select {
				case <-d.wakeupC:
					//被唤醒,等待新元素的到来
					continue
				case <-exitC:
					//结束
					goto exit
				}

			} else if delta > 0 {
				//最少有一个元素在添加中
				select {
				case <-d.wakeupC:
					//一个比当前时间更早的元素被添加
					continue
				case <-time.After(time.Duration(delta) * time.Millisecond):
					//当前最早的元素过期
					//当不需要从wakeupC中获取信号的时候就重新设置sleeping的状态
					if atomic.SwapInt32(&d.sleeping, 0) == 0 {
						//当发送信号到wakeupC中的时候，Offer的调用会被阻塞
						//从wakeupC取出信号，来解除Offer调用的阻塞
						<-d.wakeupC
					}
					continue
				case <-exitC:
					goto exit
				}

			}
		}

		//获得到过期的数据
		select {
		case d.C <- item.Value:
		//已经过期的元素已经被成功的发送出去
		case <-exitC:
			goto exit
		}
	}

exit:
	atomic.StoreInt32(&d.sleeping, 0)
}
