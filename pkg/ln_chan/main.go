package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func main() {
	//copyInChan()
	//ctxCancel()
	buffCh()

	//closeCh()
}

type A struct {
	s  string
	s2 *string
}

func closeCh() {
	//var ch chan struct{}
	//close(ch) //panic: close of nil channel
}

func buffCh() {
	ch := make(chan int, 5)
	ch <- 1
	ch <- 2
	ch <- 3
	ch <- 4
	ch <- 5
	close(ch)

	for i := range ch {
		fmt.Println(i) // 1 2 3 4 5
	}
	fmt.Println(<-ch) //0
}

func copyInChan() {
	ch1 := make(chan A)
	str := "s2;"
	a := A{
		s:  "s1;",
		s2: &str,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		aCopy := <-ch1
		aCopy.s = "new s1;"
		//str2 := "new s2;" // new adr
		//aCopy.s2 = &str2 // another addr
		*aCopy.s2 = "new s2;" // the same addr
		fmt.Println(aCopy.s, *aCopy.s2, aCopy.s2)
	}()
	ch1 <- a
	wg.Wait()
	fmt.Println(a.s, *a.s2, a.s2)

	//var ch2 chan string
	//go func() {
	//	fmt.Println(<-ch2) //goroutine 1 [chan send (nil chan)]:
	//}()
	//ch2 <- "" // fatal error: all goroutines are asleep - deadlock!

	ch3 := make(chan string)
	close(ch3)
	//ch3 <- "" //panic: send on closed channel

	v, ok := <-ch3
	fmt.Printf("from closed chan '%s' closed?=%v", v, ok)
}

func ctxCancel() {
	//sc := make(chan struct{})
	sc := make(chan int)
	cA, clA := context.WithCancel(context.Background())
	cB, _ := context.WithTimeout(context.Background(), 4*time.Second)

	go func() {
		//for {
		select {
		case <-cA.Done():
			fmt.Println("A")
			sc <- 1
			close(sc) // if there is no close then panic deadlock
		case <-cB.Done():
			fmt.Println("B")
			sc <- 1
			close(sc) //
		}
		//}
	}()

	time.Sleep(2 * time.Second)
	//time.Sleep(5 * time.Second)
	clA()
	fmt.Println(<-sc)
	fmt.Println(<-sc)
	fmt.Println(<-sc)
	fmt.Println(<-sc)
}
