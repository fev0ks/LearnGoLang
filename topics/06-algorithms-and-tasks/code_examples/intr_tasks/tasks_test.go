package intr_tasks

import (
	"fmt"
	"testing"
	"time"
)

func callCallbacks(a, b func()) {
	go a()
	go b()
}

func TestPrint(t *testing.T) {
	example()
	time.Sleep(100 * time.Millisecond)
	fmt.Println()
	exampleWithNil()
}

func example() {
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})

	callCallbacks(
		func() {
			fmt.Printf("a")
			close(firstDone)
		},
		func() {
			fmt.Printf("b")
			close(secondDone)
		},
	)

	count := 0
	for count < 2 {
		select {
		case <-firstDone:
			count++
		case <-secondDone:
			count++
		}
	}

	fmt.Printf("%d", count) // a2 b2 ab2 ba2 a2b b2a
	// ...
	// exit
}

func exampleWithNil() {
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})

	callCallbacks(
		func() {
			fmt.Printf("a")
			close(firstDone)
		},
		func() {
			fmt.Printf("b")
			close(secondDone)
		},
	)

	count := 0
	for count < 2 {
		select {
		case <-firstDone:
			count++
			firstDone = nil
		case <-secondDone:
			count++
			secondDone = nil
		}
	}

	fmt.Printf("%d", count) // ab2 ba2
	// ...
	// exit
}
