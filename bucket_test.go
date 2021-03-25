/**
* @Author:zhoutao
* @Date:2021/3/25 下午2:36
* @Desc:
 */

package timewheels

import "testing"

func TestBucket_Flush(t *testing.T) {
	b := newBucket()

	b.Add(&Timer{})
	b.Add(&Timer{})

	l1 := b.timers.Len()
	if l1 != 2 {
		t.Fatalf("Got (%+v) != wanted (%+v)", l1, 2)
	}
}
