/**
* @Author:zhoutao
* @Date:2021/3/24 下午3:20
* @Desc:
 */

package timewheels

import (
	"sync"
	"time"
)

type waitGroupWrapper struct {
	sync.WaitGroup
}

func (wg *waitGroupWrapper) Wrap(cb func()) {
	wg.Add(1)
	go func() {
		cb()
		wg.Done()
	}()
}

//返回x四舍五入后的结果（向零取整），m的倍数的结果
func truncate(x, m int64) int64 {
	if m <= 0 {
		return x
	}
	return x - x%m
}

//返回一个整数，以毫秒为单位表示t
func timeToMs(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

//返回与给定的Unix时间相对应的UTC时间
func msToTime(t int64) time.Time {
	return time.Unix(0, t*int64(time.Millisecond)).UTC()
}
