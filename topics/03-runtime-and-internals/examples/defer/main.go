package main

import (
	"fmt"
	"time"
)

func main() {
	panicCheck()
}

func deferStack() {
	defer fmt.Println("1")

	defer func() {
		fmt.Println("2.1")
		defer fmt.Println("2.2")
	}()

	defer func() {
		defer fmt.Println("3.1")
		defer func() {
			fmt.Println("3.2.1")
			defer fmt.Println("3.2")
		}()
		defer func() {
			fmt.Println("3.3.1")
			defer fmt.Println("3.3")
		}()
	}()

	defer fmt.Println("4")
	fmt.Println("5")
}

func panicCheck() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("got recover", err)
		}
	}()

	go func() {
		defer func() {
			if err := recover(); err != nil {
				fmt.Println("got recover in go1", err)
			}
		}()
		panic("panic1")
	}()

	//go func() {
	//	panic("panic2")
	//}()

	time.Sleep(1 * time.Second)
}
