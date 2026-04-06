package main

//
//import "fmt"
//
//func main() {
//	a := []string{"a", "b", "#", "c"}
//	b := []string{"a", "c", "c", "#"}
//	//fmt.Println(backspaceCompare(a, b))
//	fmt.Println(backspaceCompare2(a, b))
//
//	a2 := []string{"#", "#"}
//	b2 := []string{}
//	//fmt.Println(backspaceCompare(a2, b2))
//	fmt.Println(backspaceCompare2(a2, b2))
//
//	a3 := []string{"a", "b", "#", "#"}
//	b3 := []string{"c", "d", "#", "#"}
//	//fmt.Println(backspaceCompare(a3, b3))
//	fmt.Println(backspaceCompare2(a3, b3))
//
//	a4 := []string{"a", "b", "#", "#"}
//	b4 := []string{"c", "d", "#", "#", "d"}
//	//fmt.Println(backspaceCompare(a3, b3))
//	fmt.Println(backspaceCompare2(a4, b4))
//}
//
//func backspaceCompare2(a, b []string) bool {
//	return process(a) == process(b)
//}
//
//func process(s []string) string {
//	stack := make([]rune, 0)
//	for _, v := range s {
//		if v != "#" {
//			stack = append(stack, '#')
//		} else {
//			if len(stack) > 0 {
//				stack = stack[:len(stack)-1]
//			}
//		}
//	}
//	return string(stack)
//}
//
//func backspaceCompare(a, b []string) bool {
//	aInx := 0
//	bInx := 0
//	for _, v := range a {
//		if v == "#" {
//			if len(a) > 2 {
//				a = append(a[:aInx-1], a[aInx+1:]...) // 1-0 2-1 3-2 #-3
//				aInx = aInx - 1
//				continue
//			} else {
//				a = []string{}
//				break
//			}
//		}
//		aInx++
//	}
//	for _, v := range b {
//		if v == "#" {
//			if len(b) > 2 {
//				b = append(b[:bInx-1], b[bInx+1:]...) // 1-0 2-1 3-2 #-3
//				bInx = bInx - 1
//				continue
//			} else {
//				b = []string{}
//				break
//			}
//		}
//		bInx++
//	}
//	fmt.Println(a, b)
//	if len(a) != len(b) {
//		return false
//	}
//	if len(a) == 0 && len(b) == 0 {
//		return true
//	}
//	for i := range a {
//		if a[i] != b[i] {
//			return false
//		}
//	}
//	return true
//}
