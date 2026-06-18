package climbing_stairs

import "testing"

func TestClimbStairs(t *testing.T) {
	cases := map[int]int{
		1:  1,
		2:  2,
		3:  3,
		4:  5,
		5:  8,
		10: 89,
	}
	for n, want := range cases {
		if got := ClimbStairs(n); got != want {
			t.Errorf("ClimbStairs(%d) = %d, want %d", n, got, want)
		}
	}
}
