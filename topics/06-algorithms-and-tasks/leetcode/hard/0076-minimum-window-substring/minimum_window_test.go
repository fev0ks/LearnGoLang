package minimum_window

import "testing"

func TestMinWindow(t *testing.T) {
	cases := []struct {
		s, t string
		want string
	}{
		{"ADOBECODEBANC", "ABC", "BANC"},
		{"a", "a", "a"},
		{"a", "aa", ""},
		{"aa", "aa", "aa"},
		{"cabwefgewcwaefgcf", "cae", "cwae"},
		{"", "a", ""},
	}
	for _, c := range cases {
		if got := MinWindow(c.s, c.t); got != c.want {
			t.Errorf("MinWindow(%q, %q) = %q, want %q", c.s, c.t, got, c.want)
		}
	}
}
