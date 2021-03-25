/**
* @Author:zhoutao
* @Date:2021/3/25 下午2:59
* @Desc:
 */

package main

import (
	timewheels "github.com/ztaoing/timeWheels"
	"time"
)

func main() {
	tw := timewheels.NewTimeWheel(time.Second, 10)
	tw.Start()
}
