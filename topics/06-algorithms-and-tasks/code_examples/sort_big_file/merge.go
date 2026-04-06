package main

import (
	"bufio"
	"container/heap"
	"fmt"
	"os"
	"strconv"
)

type Item struct {
	value int
	index int // Из какого чанка число
}

type MinHeap []Item

func (h MinHeap) Len() int {
	return len(h)
}

func (h MinHeap) Less(i, j int) bool {
	return h[i].value < h[j].value
}

func (h MinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *MinHeap) Push(x interface{}) {
	*h = append(*h, x.(Item))
}
func (h *MinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func mergeChunks(chunkFiles []string, outputFile string) {
	outFile, _ := os.Create(outputFile)
	defer outFile.Close()
	writer := bufio.NewWriter(outFile)

	h := &MinHeap{}
	heap.Init(h)

	files := make([]*bufio.Scanner, len(chunkFiles))

	// Открываем файлы и загружаем первые числа в кучу
	for i, chunk := range chunkFiles {
		file, _ := os.Open(chunk)
		scanner := bufio.NewScanner(file)
		files[i] = scanner

		if scanner.Scan() {
			num, _ := strconv.Atoi(scanner.Text())
			heap.Push(h, Item{value: num, index: i})
		}
	}

	// Основной цикл слияния
	for h.Len() > 0 {
		minItem := heap.Pop(h).(Item)
		fmt.Fprintln(writer, minItem.value)

		// Читаем следующее число из соответствующего чанка
		if files[minItem.index].Scan() {
			num, _ := strconv.Atoi(files[minItem.index].Text())
			heap.Push(h, Item{value: num, index: minItem.index})
		}
	}

	writer.Flush()
}
