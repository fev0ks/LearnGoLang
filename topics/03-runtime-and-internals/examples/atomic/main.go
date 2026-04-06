package main

import (
	"fmt"
	"sync/atomic"
)

func main() {
	i := atomic.Int32{}

	i.Add(int32(1))
	i.Load()

	i2 := atomic.Uint64{}
	i2.Add(uint64(1))
	i2.Load()

	var i3 int32 = 123
	atomic.AddInt32(&i3, 123)

	v := atomic.Value{}
	v.Store(i3)
	fmt.Println(v.Load().(int32))
}
