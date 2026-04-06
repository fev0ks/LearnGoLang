package benchmarks

import (
	"testing"
)

type SmallStruct struct {
	A int
	B int
	C int
	D int
}

func consumeByValue(s SmallStruct) int {
	return s.A + s.B + s.C + s.D
}

func consumeByPointer(s *SmallStruct) int {
	return s.A + s.B + s.C + s.D
}

// 0.2610 ns/op
func BenchmarkByValue(b *testing.B) {
	s := SmallStruct{1, 2, 3, 4}
	for i := 0; i < b.N; i++ {
		_ = consumeByValue(s)
	}
}

// 0.2686 ns/op
func BenchmarkByPointer(b *testing.B) {
	s := SmallStruct{1, 2, 3, 4}
	for i := 0; i < b.N; i++ {
		_ = consumeByPointer(&s)
	}
}
