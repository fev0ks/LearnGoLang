package benchmarks

import (
	"fmt"
	"testing"
)

func BenchmarkMapsIntKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IntKey()
	}
}

func BenchmarkMapsIntStructKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ShortInt32StructKey()
	}
}

func BenchmarkMapsStringStructKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ShortStringStructKey()
	}
}

func IntKey() {
	m := make(map[int32]string, 10000)

	for i := 0; i < 10000; i++ {
		m[int32(i)] = fmt.Sprintf("%d", i)
	}
}

type KeyStruct struct {
	Key int32
}

func ShortInt32StructKey() {
	m := make(map[KeyStruct]string, 10000)

	for i := 0; i < 10000; i++ {
		m[KeyStruct{Key: int32(i)}] = fmt.Sprintf("%d", i)
	}
}

type ShortKeyStruct struct {
	Key string
}

func ShortStringStructKey() {
	m := make(map[ShortKeyStruct]string, 10000)

	for i := 0; i < 10000; i++ {
		m[ShortKeyStruct{Key: fmt.Sprintf("%d", i)}] = fmt.Sprintf("%d", i)
	}
}
