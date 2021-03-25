/**
* @Author:zhoutao
* @Date:2021/3/24 下午3:15
* @Desc:
 */

package timewheels

import (
	"container/list"
	"sync"
	"sync/atomic"
	"unsafe"
)

//Timer是时间轮执行的最小单元,是定时任务的封装，到期后会调用task来执行任务
type Timer struct {
	expiration int64          //到期时间 ms
	task       func()         //要被执行的具体任务
	b          unsafe.Pointer // timer所在的bucket的指针
	element    *list.Element  //bucket列表中对应的元素
}

func (t *Timer) getBucket() *bucket {
	return (*bucket)(atomic.LoadPointer(&t.b))
}

func (t *Timer) setBucket(b *bucket) {
	atomic.StorePointer(&t.b, unsafe.Pointer(b))
}

type bucket struct {
	expiration int64      //任务的过期时间
	mu         sync.Mutex //lock
	timers     *list.List //相同过期时间的任务队列
}

func newBucket() *bucket {
	return &bucket{
		timers:     list.New(),
		expiration: -1,
	}
}

//根据bucket里面timers列表进行遍历插入到ts数组中，然后调用addOrRun方法
func (b *bucket) Flush(reinsert func(*Timer)) {
	var ts []*Timer

	//lock
	b.mu.Lock()
	//将bucket中的节点添加到ts中，并将其在bucket中移除
	for e := b.timers.Front(); e != nil; {
		next := e.Next()
		t := e.Value.(*Timer)
		//将头节点移除bucket列表
		b.remove(t)
		ts = append(ts, t)
		//下一个
		e = next
	}
	b.mu.Unlock()

	//todo
	b.SetExpiration(-1)
	//对每个元素执行reinsert
	for _, t := range ts {
		reinsert(t)
	}
}

func (b *bucket) Expiration() int64 {
	return atomic.LoadInt64(&b.expiration)
}

func (b *bucket) SetExpiration(expiration int64) bool {
	return atomic.SwapInt64(&b.expiration, expiration) != expiration
}

func (b *bucket) Add(t *Timer) {
	b.mu.Lock()
	//添加到链表的尾部
	e := b.timers.PushBack(t)
	t.setBucket(b)
	t.element = e
	b.mu.Unlock()
}

func (b *bucket) remove(t *Timer) bool {
	if t.getBucket() != b {
		//todo
		return false
	}
	b.timers.Remove(t.element)
	t.setBucket(nil)
	t.element = nil
	return true
}

func (b *bucket) Remove(t *Timer) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.remove(t)
}
