package main

import (
	"container/heap"
	"fmt"
)

// Определяем тип MinHeap
type MinHeap []int

func (h MinHeap) Len() int           { return len(h) }
func (h MinHeap) Less(i, j int) bool { return h[i] < h[j] } // Минимальный элемент наверху
func (h MinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

// Методы для работы с heap.Interface
func (h *MinHeap) Push(x interface{}) {
	*h = append(*h, x.(int))
}

func (h *MinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Функция для получения 10 наибольших элементов
func TopKElements(arr []int, k int) []int {
	h := &MinHeap{}
	heap.Init(h)

	for _, num := range arr {
		if h.Len() < k {
			heap.Push(h, num) // Добавляем в кучу, пока не наберется k элементов
		} else if num > (*h)[0] {
			heap.Pop(h)       // Удаляем минимальный элемент
			heap.Push(h, num) // Добавляем новый
		}
	}

	// Преобразуем кучу в срез и сортируем для наглядности
	result := make([]int, h.Len())
	for i := range result {
		result[i] = heap.Pop(h).(int)
	}
	return result
}

func main() {
	arr := []int{12, 3, 17, 8, 34, 25, 99, 45, 67, 5, 19, 21, 23, 88, 100}
	k := 10

	topElements := TopKElements(arr, k)
	fmt.Println("Top 10 largest elements:", topElements)
}
