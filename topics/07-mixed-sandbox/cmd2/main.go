package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type MyClosable interface {
	Close()
}

type Closable struct {
}

func (clos *Closable) Close() {
	fmt.Println("Closable")
}

func main() {
	var c MyClosable
	c = &Closable{}
	if closable, ok := c.(interface{ Close() }); ok {
		closable.Close()
	}

	fmt.Println(12 & 10)

	//i := math.MaxInt32
	//i = i + 1
	//fmt.Println(i)
	//m := make(map[*string]int)
	//m[nil] = 0
	//s := "key"
	//s1 := &s
	//m[s1] = 123
	//*s1 = "hello"
	//s2 := &s
	//mu := &sync.Mutex{}
	//fmt.Println(len(m))
	//fmt.Println(m)
	//fmt.Println(m[s2])
	//fmt.Println(s, *s1, *s2)
	//k2()
}

func k() error {
	var errCustom error
	defer func() {
		if errCustom != nil {
			fmt.Println("Rollback")
		}
	}()
	//
	//i := 0
	//_ = 10 / i

	//err =
	//fmt.Println(err)
	errCustom = fmt.Errorf("some error")
	return errCustom
}

func k2() {
	wg := sync.WaitGroup{}
	var counter *int
	var counter2 = 0
	var counterA int32 = 0
	z := 0
	counter = &z
	for i := 0; i < 20000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter2++
			*counter = *counter + 1
			atomic.AddInt32(&counterA, 1)
		}()
	}
	wg.Wait()
	fmt.Println(*counter)
	fmt.Println(counter2)
	fmt.Println(atomic.LoadInt32(&counterA))
}
