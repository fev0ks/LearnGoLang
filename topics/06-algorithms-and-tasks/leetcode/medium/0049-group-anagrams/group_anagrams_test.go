package group_anagrams

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// normalize приводит результат к каноничному виду (сортировка внутри групп и
// между ними), т.к. порядок из map недетерминирован.
func normalize(groups [][]string) [][]string {
	out := make([][]string, len(groups))
	for i, g := range groups {
		cp := append([]string(nil), g...)
		sort.Strings(cp)
		out[i] = cp
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.Join(out[i], ",") < strings.Join(out[j], ",")
	})
	return out
}

func TestGroupAnagrams(t *testing.T) {
	cases := []struct {
		in   []string
		want [][]string
	}{
		{
			[]string{"eat", "tea", "tan", "ate", "nat", "bat"},
			[][]string{{"ate", "eat", "tea"}, {"bat"}, {"nat", "tan"}},
		},
		{
			[]string{""},
			[][]string{{""}},
		},
		{
			[]string{"a"},
			[][]string{{"a"}},
		},
	}

	for _, c := range cases {
		got := normalize(GroupAnagrams(c.in))
		want := normalize(c.want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GroupAnagrams(%v) = %v, want %v", c.in, got, want)
		}
	}
}
