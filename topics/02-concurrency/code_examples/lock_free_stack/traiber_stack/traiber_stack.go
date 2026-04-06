package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

type item struct {
	value int
	next  unsafe.Pointer
}

type Stack struct {
	head unsafe.Pointer
}

func New() *Stack {
	return &Stack{}
}

func (s *Stack) Push(value int) {
	node := &item{value: value}

	for {
		head := atomic.LoadPointer(&s.head)
		node.next = head

		if atomic.CompareAndSwapPointer(&s.head, head, unsafe.Pointer(node)) {
			return
		}
	}
}

func (s *Stack) Pop() int {
	for {
		head := atomic.LoadPointer(&s.head)
		if head == nil {
			return -1
		}

		next := atomic.LoadPointer(&(*item)(s.head).next)
		if atomic.CompareAndSwapPointer(&s.head, head, next) {
			return (*item)(head).value
		}
	}
}

func main() {
	stack := New()

	wg := sync.WaitGroup{}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				stack.Push(i*1000 + j)
			}
		}()
	}
	wg.Wait()

	var count int
	for i := stack.Pop(); i != -1; i = stack.Pop() {
		count++
	}

	fmt.Println(count)
}
