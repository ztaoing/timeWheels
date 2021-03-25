/**
* @Author:zhoutao
* @Date:2021/3/25 下午2:48
* @Desc:
 */

package timewheels

import (
	"testing"
	"time"
)

func TestTimeWheel_AfterFunc(t *testing.T) {
	tw := NewTimeWheel(time.Millisecond, 20)
}
