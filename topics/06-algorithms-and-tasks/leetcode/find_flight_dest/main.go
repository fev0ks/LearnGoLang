package main

import "fmt"

//[["SFO", "EWR"]] => ["SFO", "EWR"]
//[["ATL", "EWR"], ["SFO", "ATL"]]  => ["SFO", "EWR"]
//[["IND", "EWR"], ["SFO", "ATL"], ["GSO", "IND"], ["ATL", "GSO"]] => ["SFO", "EWR"]

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
	for _, t := range testCases {
		fmt.Println(getDist(t))
	}
}

//type dist struct {
//	flight
//	start dist
//	end   dist
//}

func getDist(test testCase) []int {
	route := make(map[int]int)
	isRouteEnd := make(map[int]bool)
	for _, t := range test.flights {
		isRouteEnd[t.end] = true
		route[t.start] = t.end
	}

	var start int
	for _, t := range test.flights {
		if !isRouteEnd[t.start] {
			start = t.start
			break
		}
	}

	path := []int{start}
	for next, ok := route[start]; ok; next, ok = route[next] {
		if _, hasStart := route[next]; !hasStart {
			path = append(path, next)
			break
		}
	}
	return path
}
