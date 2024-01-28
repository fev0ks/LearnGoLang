package main

import (
	"fmt"
	"sync"
)

func main() {
	//mapInit()
	m := make(map[int]int)
	fmt.Println(len(m))

	//m := map[int]int{1: 1, 2: 2, 3: 3}
	//kek(m)
	//fmt.Println(m)
	//lel()
	//testSlices2()

	//s := [5]string{"a", "b", "c", "d"}
	//s1 := s[2:5]
	//fmt.Println(s1, len(s1), cap(s1))
	//s1 = append(s1, "e")
	//fmt.Println(s1, len(s1), cap(s1))
	//fmt.Println(s, len(s), cap(s))
	//testAsyncMap()
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

func testSlices1() {
	a := []string{"a", "b", "c"}
	b := a[1:2]
	b[0] = "q"

	fmt.Printf("%s\n", a) // что отобразится после вызова?
}

func testSlices2() {
	a := []byte{'a', 'b', 'c'}
	fmt.Printf("%d %d\n", cap(a), len(a))
	c := a[1:1]
	fmt.Printf("%d %d\n", cap(c), len(c))
	b := append(a[1:2], 'd')
	fmt.Printf("%d %d\n", cap(b), len(b))
	fmt.Printf("%s\n", b)
	b[0] = 'z'

	fmt.Printf("%s\n", a) // что отобразится после вызова?
}

func testSlices3() {
	a := []int{1, 2, 3}
	b := a
	b = append(b, 4)
	c := b
	b[0] = 0
	e := append(c, 5)
	b[2] = 7

	fmt.Println(a, b, c, e)
}
