package backspace_compare

import "testing"

func TestBackspaceCompare(t *testing.T) {
	cases := []struct {
		name string
		s, t string
		want bool
	}{
		{"ab#c vs ad#c", "ab#c", "ad#c", true},
		{"a##c vs #a#c", "a##c", "#a#c", true},
		{"a#c vs b", "a#c", "b", false},
		{"оба пустеют", "a#", "b#", true},
		{"лишний backspace на пустом", "###", "", true},
		{"разная длина результата", "ab##", "c#d#e", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := BackspaceCompare(c.s, c.t); got != c.want {
				t.Errorf("BackspaceCompare(%q, %q) = %v, want %v", c.s, c.t, got, c.want)
			}
			if got := BackspaceCompareTwoPointers(c.s, c.t); got != c.want {
				t.Errorf("BackspaceCompareTwoPointers(%q, %q) = %v, want %v", c.s, c.t, got, c.want)
			}
		})
	}
}
