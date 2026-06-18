package main

import "testing"

func TestGetRoute(t *testing.T) {
	// getRoute восстанавливает крайние точки маршрута [старт, финиш] по набору
	// перелётов, идущих в произвольном порядке. Эталон — поле dist каждого кейса.
	for i, tc := range testCases {
		got, ok := getRoute(tc.flights)
		if !ok {
			t.Errorf("testCases[%d]: getRoute() ok = false, want true", i)
			continue
		}
		if got != tc.dist {
			t.Errorf("testCases[%d]: getRoute() = %v, want %v", i, got, tc.dist)
		}
	}
}

func TestGetRouteEmpty(t *testing.T) {
	if _, ok := getRoute(nil); ok {
		t.Error("getRoute(nil) ok = true, want false")
	}
}
