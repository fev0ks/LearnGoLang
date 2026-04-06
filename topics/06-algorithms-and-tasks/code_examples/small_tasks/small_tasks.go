package main

import "fmt"

func main() {
	//fmt.Println(reverse("qwertyu"))
	fmt.Println(isPolindrome("qwerttrewq"))

}

func reverse(s string) string {
	runes := []rune(s)
	for i := 0; i < (len(runes))/2; i++ {
		runes[i], runes[len(runes)-i-1] = runes[len(runes)-i-1], runes[i]
	}
	return string(runes)
}

func isPolindrome(s string) bool {
	runes := []rune(s)
	for i := 0; i < len(runes)/2; i++ {
		if runes[i] != runes[len(runes)-i-1] {
			return false
		}
	}
	for i, r := range s {
		fmt.Println(i, string(r))
	}
	for i := 0; i < len(runes)/2; i++ {
		if runes[i] != runes[len(runes)-i-1] {
			return false
		}
	}
	return true
}
