package median_finder

import "testing"

func TestMedianFinder(t *testing.T) {
	mf := Constructor()

	mf.AddNum(1)
	mf.AddNum(2)
	if got := mf.FindMedian(); got != 1.5 {
		t.Errorf("после 1,2: FindMedian() = %v, want 1.5", got)
	}

	mf.AddNum(3)
	if got := mf.FindMedian(); got != 2.0 {
		t.Errorf("после 1,2,3: FindMedian() = %v, want 2.0", got)
	}
}

func TestMedianFinderUnsorted(t *testing.T) {
	mf := Constructor()
	for _, n := range []int{5, 2, 8, 1, 9} {
		mf.AddNum(n)
	}
	// Отсортировано: [1,2,5,8,9], медиана — 5.
	if got := mf.FindMedian(); got != 5.0 {
		t.Errorf("FindMedian() = %v, want 5.0", got)
	}

	mf.AddNum(3) // [1,2,3,5,8,9] -> медиана (3+5)/2 = 4
	if got := mf.FindMedian(); got != 4.0 {
		t.Errorf("FindMedian() = %v, want 4.0", got)
	}
}
