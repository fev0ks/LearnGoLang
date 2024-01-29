package main

import "fmt"

func main() {
	orig := []string{"one", "two", "three"}
	var ptr []*string
	for _, item := range orig {
		fmt.Printf("%s: %p\n", item, &item)
		//three: 0xc00005e260
		//two: 0xc00005e260
		//one: 0xc00005e260
		ptr = append(ptr, &item)
	}
	fmt.Printf("ptr: %v\n", ptr) // ptr: [0xc00005e260 0xc00005e260 0xc00005e260]
	for _, p := range ptr {
		fmt.Printf("%v: %p %s\n", p, &p, *p)
		//0xc00005e260: 0xc00000a038 three
		//0xc00005e260: 0xc00000a038 three
		//0xc00005e260: 0xc00000a038 three
	}
}
