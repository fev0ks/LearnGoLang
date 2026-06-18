package longest_substring

import "testing"

func TestLengthOfLongestSubstring(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"abcabcbb", 3},
		{"bbbbb", 1},
		{"pwwkew", 3},
		{"", 0},
		{" ", 1},
		{"au", 2},
		{"dvdf", 3},
		{"abba", 2},
	}

	for _, c := range cases {
		t.Run(c.s, func(t *testing.T) {
			if got := LengthOfLongestSubstring(c.s); got != c.want {
				t.Errorf("LengthOfLongestSubstring(%q) = %d, want %d", c.s, got, c.want)
			}
		})
	}
}
