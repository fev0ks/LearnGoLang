package main

import (
	"fmt"
	"math"
)

func main() {
	sl := []int{1, 12, -5, -6, 50, 3}
	fmt.Println(findMaxAverage2(sl, 4))
}

func findMaxAverage(nums []int, k int) float64 {
	maxVal := math.MinInt32

	for i := 0; i < len(nums)-k; i++ {
		sum := nums[i]
		for j := i + 1; j < i+k; j++ {
			sum += nums[j]
		}
		if sum > maxVal {
			maxVal = sum
		}
	}
	return float64(maxVal) / float64(k)
}

func findMaxAverage2(nums []int, k int) float64 {
	maxVal := math.MinInt32
	sum := 0

	for j := 0; j < k; j++ {
		sum += nums[j]
	}
	maxVal = sum
	for i := k; i < len(nums); i++ {
		sum += -nums[i-k] + nums[i]
		if sum > maxVal {
			maxVal = sum
		}
	}
	return float64(maxVal) / float64(k)
}
