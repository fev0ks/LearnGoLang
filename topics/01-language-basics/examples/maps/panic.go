package main

import (
	"fmt"
	"sync"
)

func cuncMapAccess() {

	wg := sync.WaitGroup{}
	wg.Add(2)
	mp := map[int]int{}
	go func() {
		defer wg.Done()
		safeMapAccess(mp)
	}()
	go func() {
		defer wg.Done()
		writeToMap(mp)
	}()
	wg.Wait()
}

func safeMapAccess(m map[int]int) {

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("panic caught:", r)
			return
		}
	}()

	var i *int
	_ = *i

	//for i := 0; i < 1000; i++ {
	//	_ = m[i]
	//}
}

func writeToMap(m map[int]int) {
	//for i := 0; i < 1000; i++ {
	//	m[i] = i
	//}
}
