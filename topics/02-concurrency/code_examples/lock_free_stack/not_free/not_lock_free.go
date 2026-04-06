package main

import "fmt"

type item struct {
	value int
	next  *item
}

type Stack struct {
	head *item
}

func NewStack() *Stack {
	return &Stack{}
}

func (s *Stack) Push(v int) {
	s.head = &item{value: v, next: s.head}
}

func (s *Stack) Pop() int {
	if s.head == nil {
		return -1
	}

	v := s.head.value
	s.head = s.head.next
	return v
}

func main() {
	stack := NewStack()
	stack.Push(1)
	stack.Push(2)
	stack.Push(3)

	fmt.Println(stack.Pop())
	fmt.Println(stack.Pop())
	fmt.Println(stack.Pop())
	fmt.Println(stack.Pop())
}
