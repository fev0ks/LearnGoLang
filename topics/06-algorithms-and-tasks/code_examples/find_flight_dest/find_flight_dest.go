package main

import "fmt"

// Задача: дан набор перелётов (пар «откуда -> куда»), идущих в произвольном
// порядке и образующих один сквозной маршрут без разрывов и развилок.
// Нужно восстановить крайние точки маршрута — стартовый и конечный город.
//
//   [[SFO, EWR]]                                  -> [SFO, EWR]
//   [[ATL, EWR], [SFO, ATL]]                      -> [SFO, EWR]
//   [[IND, EWR], [SFO, ATL], [GSO, IND], [ATL, GSO]] -> [SFO, EWR]
//
// (в коде города закодированы числами для краткости тестов).

type flight struct {
	start int
	end   int
}

type testCase struct {
	flights []flight
	dist    flight
}

var testCases = []testCase{
	{
		flights: []flight{{1, 2}},
		dist:    flight{1, 2},
	},
	{
		flights: []flight{
			{1, 2},
			{2, 3},
			{3, 4},
		},
		dist: flight{1, 4},
	},
	{
		flights: []flight{
			{1, 2},
			{3, 4},
			{2, 3},
			{4, 5},
		},
		dist: flight{1, 5},
	},
	{
		flights: []flight{
			{3, 4},
			{2, 3},
			{1, 2},
			{4, 5},
		},
		dist: flight{1, 5},
	},
}

func main() {
	for _, tc := range testCases {
		route, ok := getRoute(tc.flights)
		fmt.Println(route, ok)
	}
}

// getRoute восстанавливает крайние точки маршрута по набору перелётов.
//
// Идея за O(n):
//   - next: карта переходов «откуда -> куда»;
//   - hasIncoming: множество городов, в которые кто-то прилетает;
//   - старт — единственный город без входящих перелётов;
//   - от старта идём по next до города, из которого вылетов уже нет, — это финиш.
//
// Второй результат — false, если перелётов нет.
func getRoute(flights []flight) (flight, bool) {
	if len(flights) == 0 {
		return flight{}, false
	}

	next := make(map[int]int, len(flights))
	hasIncoming := make(map[int]bool, len(flights))
	for _, f := range flights {
		next[f.start] = f.end
		hasIncoming[f.end] = true
	}

	// Старт — город, в который никто не прилетает.
	start := flights[0].start
	for _, f := range flights {
		if !hasIncoming[f.start] {
			start = f.start
			break
		}
	}

	// Идём по цепочке до конца. Не более len(flights) переходов — это и
	// ограничивает цикл на случай некорректных (зацикленных) данных.
	end := start
	for range flights {
		dest, ok := next[end]
		if !ok {
			break
		}
		end = dest
	}

	return flight{start: start, end: end}, true
}
