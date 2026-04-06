package main

import (
	"fmt"
	"strings"
)

type Node[T any] struct {
	next  *Node[T]
	value T
}

type Stack[T any] struct {
	head *Node[T]
}

func (s *Stack[T]) Push(value T) {
	s.head = &Node[T]{next: s.head, value: value}
}

func (s *Stack[T]) Pop() (value T, ok bool) {
	if s.head == nil {
		return value, false
	}
	value = s.head.value
	s.head = s.head.next
	return value, true
}

func (s *Stack[T]) Peek() (value T, ok bool) {
	if s.head == nil {
		return value, false
	}
	return s.head.value, true
}

func (s *Stack[T]) Empty() bool {
	return s.head == nil
}

func main() {
	//st := &Stack[int]{}
	//st.Push(1)
	//st.Push(2)
	//st.Push(3)
	//st.Push(4)
	//fmt.Println(st.Pop())
	//fmt.Println(st.Pop())
	//fmt.Println(st.Peek())
	//fmt.Println(st.Peek())
	//fmt.Println(st.Peek())

	pStack := &Stack[string]{}

	pths := []string{
		"/fmt/bar/gaz/../././",
	}
	pth := strings.Split(pths[0], "/")

	for _, v := range pth {
		if v == ".." {
			pStack.Pop()
			continue
		}
		if v == "." || v == "" {
			continue
		}
		pStack.Push(v)
	}
	for !pStack.Empty() {
		v, _ := pStack.Pop()
		fmt.Print("/", v)
	}
}
