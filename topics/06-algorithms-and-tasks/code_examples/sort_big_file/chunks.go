package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
)

const chunkSize = 1 << 30 // 1GB

func splitAndSortChunks(inputFile string) []string {
	file, _ := os.Open(inputFile)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	chunkCount := 0
	var numbers []int

	var chunkFiles []string

	for scanner.Scan() {
		num, _ := strconv.Atoi(scanner.Text())
		numbers = append(numbers, num)

		// Если накопили 1GB данных, сортируем и пишем в файл
		if len(numbers)*8 >= chunkSize {
			sort.Ints(numbers)
			chunkFile := fmt.Sprintf("chunk_%d.txt", chunkCount)
			writeChunkToFile(chunkFile, numbers)
			chunkFiles = append(chunkFiles, chunkFile)
			numbers = nil // Очистка памяти
			chunkCount++
		}
	}

	// Записываем последний чанк
	if len(numbers) > 0 {
		sort.Ints(numbers)
		chunkFile := fmt.Sprintf("chunk_%d.txt", chunkCount)
		writeChunkToFile(chunkFile, numbers)
		chunkFiles = append(chunkFiles, chunkFile)
	}

	return chunkFiles
}

func writeChunkToFile(filename string, numbers []int) {
	file, _ := os.Create(filename)
	defer file.Close()
	writer := bufio.NewWriter(file)

	for _, num := range numbers {
		fmt.Fprintln(writer, num)
	}
	writer.Flush()
}
