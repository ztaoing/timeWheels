/**
* @Author:zhoutao
* @Date:2021/3/24 下午3:15
* @Desc:
 */

package timewheels

import (
	"container/list"
	"sync"
	"unsafe"
)

type bucket struct {
	expiration int64      //任务的过期时间
	mu         sync.Mutex //lock
	timers     *list.List //相同过期时间的任务队列
}

//Timer是时间轮执行的最小单元,是定时任务的封装，到期后会调用task来执行任务
type Timer struct {
	expiration int64          //到期时间 ms
	task       func()         //要被执行的具体任务
	b          unsafe.Pointer // timer所在的bucket的指针
	element    *list.List     //bucket列表中对应的元素
}
