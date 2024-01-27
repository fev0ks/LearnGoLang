package everything

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	shutdownTimeout = 8 * time.Second
)

func TryProperExit() {
	log.Println("Starting application...")

	workFinished := make(chan bool)
	ProperDefer(workFinished)
	someGoWork()
	workFinished <- true

	doSomeWork()
}

func someGoWork() {
	waitGroup := &sync.WaitGroup{}

	waitGroup.Add(1)
	go func(waitGroup *sync.WaitGroup) {
		log.Println("Starting work goroutine...")
		defer waitGroup.Done()
		doSomeWork()
		return
	}(waitGroup)
	log.Println("Wait.")
	waitGroup.Wait()
	log.Println("Done.")
}

func ProperDefer(workFinished chan bool) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	go func() {
		select {
		case <-signals:
			fmt.Println("received a signal for termination, shutting down gracefully...")
			waitGoroutinesFinish(workFinished)
		case <-workFinished:
			fmt.Println("Not graceful shut down case")
			return
		}
	}()
}

func waitGoroutinesFinish(shutdownChannel chan bool) {
	select {
	case <-shutdownChannel:
		fmt.Println("graceful shut down")
		os.Exit(0)
	case <-time.After(shutdownTimeout):
		fmt.Printf("didn't shut down gracefully in time (%v), exiting\n", shutdownTimeout)
		os.Exit(1)
	}
}

func doSomeWork() {
	log.Println("Do work.")
	time.Sleep(10 * time.Second)
	log.Println("Finish work")
}
