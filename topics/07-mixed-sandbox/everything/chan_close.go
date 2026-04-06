package everything

import (
	"fmt"
	"runtime"
	"time"
)

func ChanClose() {
	fmt.Println("Start")
	doneChan := make(chan bool)
	go func() {
		someWork()
		doneChan <- true
	}()
	fmt.Println(runtime.NumGoroutine())
	select {
	case result := <-doneChan:
		fmt.Printf("done %v\n", result)
	case <-time.After(time.Second * 1):
		close(doneChan)
		fmt.Printf("timeout\n")
	}

	fmt.Println(runtime.NumGoroutine())
	time.Sleep(time.Second * 5)
	fmt.Println(runtime.NumGoroutine())
}

func someWork() {
	time.Sleep(time.Second * 2)
	fmt.Println("work done")
}
