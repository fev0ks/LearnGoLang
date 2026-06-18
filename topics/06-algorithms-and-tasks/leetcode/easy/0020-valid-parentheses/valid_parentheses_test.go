package valid_parentheses

import "testing"

func TestIsValid(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"", true},
		{"()", true},
		{"()[]{}", true},
		{"{[]}", true},
		{"(]", false},
		{"([)]", false},
		{"(", false},
		{")", false},
		{"((", false},
		{"){", false},
	}

	for _, c := range cases {
		t.Run(c.s, func(t *testing.T) {
			if got := IsValid(c.s); got != c.want {
				t.Errorf("IsValid(%q) = %v, want %v", c.s, got, c.want)
			}
		})
	}
}
