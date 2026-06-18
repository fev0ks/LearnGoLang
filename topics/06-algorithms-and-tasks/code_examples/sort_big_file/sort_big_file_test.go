package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"
)

// TestExternalSort проверяет полный цикл внешней сортировки: разбиение на чанки
// + слияние через кучу. Работаем во временной директории, т.к. chunks.go и
// merge.go создают файлы по относительным путям (chunk_*.txt, output.txt).
func TestExternalSort(t *testing.T) {
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	input := []int{42, 7, 7, 100, -3, 0, 15, 8, 23, 4, 16}

	// Готовим входной файл (по числу в строке).
	const inputFile = "input.txt"
	f, err := os.Create(inputFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range input {
		fmt.Fprintln(f, n)
	}
	f.Close()

	// Прогон: split -> merge.
	const outputFile = "output.txt"
	chunks := splitAndSortChunks(inputFile)
	if len(chunks) == 0 {
		t.Fatal("splitAndSortChunks вернул пустой список чанков")
	}
	mergeChunks(chunks, outputFile)

	// Читаем результат.
	out, err := os.Open(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	var got []int
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		n, err := strconv.Atoi(scanner.Text())
		if err != nil {
			t.Fatalf("не число в выводе: %q", scanner.Text())
		}
		got = append(got, n)
	}

	want := append([]int(nil), input...)
	sort.Ints(want)

	if len(got) != len(want) {
		t.Fatalf("длина результата = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("результат не отсортирован: got %v, want %v", got, want)
		}
	}
}
