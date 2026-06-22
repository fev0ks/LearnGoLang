package smaller_numbers

import (
	"reflect"
	"testing"
)

// commonCases — кейсы из условия 1365, корректные для всех трёх реализаций
// (значения в диапазоне 0..100).
var commonCases = []struct {
	nums []int
	want []int
}{
	{[]int{8, 1, 2, 2, 3}, []int{4, 0, 1, 1, 3}},
	{[]int{6, 5, 4, 8}, []int{2, 1, 0, 3}},
	{[]int{7, 7, 7, 7}, []int{0, 0, 0, 0}},
	{[]int{0}, []int{0}},               // один элемент
	{[]int{0, 0, 100}, []int{0, 0, 2}}, // граничные значения диапазона
}

func TestSmallerNumbersThanCurrent(t *testing.T) {
	for _, c := range commonCases {
		if got := SmallerNumbersThanCurrent(c.nums); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SmallerNumbersThanCurrent(%v) = %v, want %v", c.nums, got, c.want)
		}
	}
}

// genericCases — значения вне диапазона 0..100: отрицательные и большие. Counting
// sort их не обработал бы, поэтому проверяем только универсальные реализации.
var genericCases = []struct {
	nums []int
	want []int
}{
	{[]int{-5, -1, -5, 0}, []int{0, 2, 0, 3}},            // отрицательные и дубли
	{[]int{10000000, 3, 10000000, 1}, []int{2, 1, 2, 0}}, // большие значения
}

func TestSmallerNumbersGeneric(t *testing.T) {
	cases := append(append([]struct {
		nums []int
		want []int
	}{}, commonCases...), genericCases...)

	for _, c := range cases {
		if got := SmallerNumbersGeneric(c.nums); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SmallerNumbersGeneric(%v) = %v, want %v", c.nums, got, c.want)
		}
		if got := SmallerNumbersBinary(c.nums); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SmallerNumbersBinary(%v) = %v, want %v", c.nums, got, c.want)
		}
	}
}
