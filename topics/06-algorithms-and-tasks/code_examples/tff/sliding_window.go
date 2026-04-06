package main

//
//import "fmt"
//
////Есть массив из положительных чисел.Найти непрерывный подмассив с максимальной суммой, в котором не более двух различных элементов. Вернуть сумму его элементов.
//
//// maxSum ( [1, 2]) == 3
//// maxSum([1, 10, 5, 10, 5, 10, 3, 10, 5]) = 40 (10, 5, 10, 5, 10))
//// maxSum( [10, 10, 3, 5, 5, 5]) == 23 (10, 10, 3)
////
//// maxSum ( [1, 2]) == 3
//// maxSum([1, 10, 5, 10, 5, 10, 3, 10, 5]) = 40
//// maxSum( [10, 10, 3, 5, 5, 5]) == 23
//func main() {
//	ar1 := []int{1, 2}
//	fmt.Println(processSum(ar1))
//	ar2 := []int{1, 10, 5, 10, 5, 10, 3, 10, 5}
//	fmt.Println(processSum(ar2))
//	ar3 := []int{10, 10, 3, 5, 5, 5}
//	fmt.Println(processSum(ar3))
//}
//
//func processSum(nums []int) int {
//	sub := make(map[int]int)
//	left, sum, maxSum := 0, 0, 0
//
//	for right := range nums {
//		rightNum := nums[right]
//		sum += rightNum
//		sub[rightNum]++
//
//		for len(sub) > 2 {
//			leftNum := nums[left]
//			sub[leftNum]--
//			sum -= leftNum
//			if sub[leftNum] == 0 {
//				delete(sub, leftNum)
//			}
//			left++
//		}
//
//		if sum > maxSum {
//			maxSum = sum
//		}
//	}
//	return maxSum
//}
//
//// ar [1 2]; sum 3, idx 0, len 2
//// [1 2] 3
//// ar [1 10 5 10 5 10 3 10 5]; sum 30, idx 2, len 4
//// [5 10] 30
//// ar [10 10 3 5 5 5]; sum 23, idx 0, len 3
//// [10 10 3] 23
//func processSumWrong(ar []int) ([]int, int) {
//	sub := make(map[int]struct{})
//
//	curSum := 0
//	curIdx := 0
//	curLen := 0
//
//	maxSum := 0
//	maxIdx := 0
//	maxLen := 0
//	for i, v := range ar {
//		if _, ok := sub[v]; !ok {
//			if len(sub) == 2 {
//				sub = make(map[int]struct{})
//				if curSum > maxSum {
//					maxSum = curSum
//					maxIdx = curIdx
//					maxLen = curLen
//				}
//				sub[v] = struct{}{}
//				curIdx = i
//				curSum = 0
//				curLen = 0
//			} else {
//				sub[v] = struct{}{}
//			}
//		}
//		curSum += v
//		curLen++
//	}
//
//	if curSum > maxSum {
//		maxSum = curSum
//		maxIdx = curIdx
//		maxLen = curLen
//	}
//	fmt.Printf("ar %v; sum %d, idx %d, len %d\n", ar, maxSum, maxIdx, maxLen)
//
//	return ar[maxIdx:maxLen], maxSum
//
//}
