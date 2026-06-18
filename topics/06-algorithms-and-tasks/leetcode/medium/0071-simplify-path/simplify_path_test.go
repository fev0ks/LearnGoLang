package simplify_path

import "testing"

func TestSimplifyPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/home/", "/home"},
		{"/../", "/"},
		{"/home//foo/", "/home/foo"},
		{"/a/./b/../../c/", "/c"},
		{"/", "/"},
		{"/a/../../b/../c//.//", "/c"},
		{"/fmt/bar/gaz/../././", "/fmt/bar"},
	}

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if got := SimplifyPath(c.path); got != c.want {
				t.Errorf("SimplifyPath(%q) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}
