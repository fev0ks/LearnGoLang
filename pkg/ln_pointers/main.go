package main

import "fmt"

func main() {
	orig := []string{"one", "two", "three"}
	var ptr []*string
	for _, item := range orig {
		fmt.Printf("%s: %p\n", item, &item)
		ptr = append(ptr, &item)
	}
	for _, p := range ptr {
		fmt.Println(*p)
	}
}
