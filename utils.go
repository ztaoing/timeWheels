/**
* @Author:zhoutao
* @Date:2021/3/24 下午3:20
* @Desc:
 */

package timewheels

import "sync"

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
