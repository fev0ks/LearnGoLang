package main

import (
	"fmt"
	"sync"
)

func main() {
	var m1 map[string]int
	fmt.Println(m1["kek"])
	delete(m1, "kek") //If m is nil or there is no such element, delete is a no-op.
	m1["kek"] = 100   //panic: assignment to entry in nil map

	//mapInit()
	m := make(map[int]int, 1)
	//fmt.Println(len(m))
	l := &m
	for i := 0; i < 1000000; i++ {
		m[i] = i
	}
	fmt.Println(len(*l))

	//testSlices1()

	//m := map[int]int{1: 1, 2: 2, 3: 3}
	//kek(m)
	//fmt.Println(m)
	//fmt.Println(m)
	//lel()

	//s := [5]string{"a", "b", "c", "d"}
	//s1 := s[2:5]
	//fmt.Println(s1, len(s1), cap(s1))
	//s1 = append(s1, "e")
	//fmt.Println(s1, len(s1), cap(s1))
	//fmt.Println(s, len(s), cap(s))
	//testAsyncMap()

	//cuncMapAccess()
}

func mapInit() {
	//var m map[string]string
	//m["qwe"] = "qwe" //panic: assignment to entry in nil map
}

func testAsyncMap() {
	m := make(map[int]int)
	l := sync.RWMutex{}
	wg := sync.WaitGroup{}
	wg.Add(10)
	for i := 0; i < 10; i++ {
		i := i
		go func() {
			defer wg.Done()
			l.Lock()
			defer l.Unlock()
			m[i] = i
		}()
	}
	wg.Wait()
	fmt.Println(m)
}

func kek(m map[int]int) {
	//l := make(map[int]int, len(m))
	l := m
	delete(l, 1)
	delete(l, 2)
	fmt.Println(m)
	fmt.Println(l)
}

func lel() {
	l := make(map[int][]int)
	m := map[int]int{1: 1, 2: 1, 3: 1, 4: 4}
	for val, key := range m {
		if a, ok := l[key]; ok {
			l[key] = append(a, val)
		} else {
			l[key] = []int{val}
		}
	}
	fmt.Println(l)
}
