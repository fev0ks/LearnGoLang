package main

import (
	"fmt"
)

func main() {
	c := make(chan int, 1000)

	for i := 0; i < 100; i++ {
		go foo(c)

	}
	sum := 0
	for i := range c {
		sum += i

	}

	fmt.Println(sum)
}

func foo(c chan int) {

	//val := rand.Int()
	val := 10
	for i := range val {
		c <- i
	}

}
