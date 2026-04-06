package main

import (
	"fmt"
	"time"
)

func spawnMessages(n int, closeCh chan struct{}) chan string {
	ch := make(chan string, 1)
	go func() {
		for i := 0; i < n; i++ {
			time.Sleep(1 * time.Second)
			select {
			case ch <- fmt.Sprintf("msg %d", i+1):
			case <-closeCh:
				return
			}

		}
		close(ch)
	}()
	return ch
}

func main() {
	n := 10
	closeCh := make(chan struct{})
	ch := spawnMessages(n, closeCh)

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Println("received:", msg)
		case <-time.After(500 * time.Millisecond):
			close(closeCh)
			fmt.Println("timeout")
			return

		}
	}
}
