package valid_anagram

import "testing"

func TestIsAnagram(t *testing.T) {
	cases := []struct {
		s, t string
		want bool
	}{
		{"anagram", "nagaram", true},
		{"rat", "car", false},
		{"", "", true},
		{"a", "ab", false},
		{"ab", "ba", true},
		{"aacc", "ccac", false},
	}
	for _, c := range cases {
		if got := IsAnagram(c.s, c.t); got != c.want {
			t.Errorf("IsAnagram(%q, %q) = %v, want %v", c.s, c.t, got, c.want)
		}
	}
}
