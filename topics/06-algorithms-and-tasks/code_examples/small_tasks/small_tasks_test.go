package main

import "testing"

func TestReverse(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"a":       "a",
		"abc":     "cba",
		"привет":  "тевирп", // проверка корректной работы с рунами
		"qwertyu": "uytrewq",
	}
	for in, want := range cases {
		if got := reverse(in); got != want {
			t.Errorf("reverse(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsPolindrome(t *testing.T) {
	cases := map[string]bool{
		"":           true,
		"a":          true,
		"abba":       true,
		"qwerttrewq": true,
		"abc":        false,
		"abca":       false,
	}
	for in, want := range cases {
		if got := isPolindrome(in); got != want {
			t.Errorf("isPolindrome(%q) = %v, want %v", in, got, want)
		}
	}
}
