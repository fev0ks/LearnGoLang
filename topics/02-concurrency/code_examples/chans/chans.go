package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

//writer - генерирует 10 чисел
//doubler - удваивает на 2 с задержкой 500мс
//reader - выводит на экран

func main() {
	//reader(double(writer()))
	//predictableWork(2)
	poolWorkers()
}

func reader(in <-chan int) {
	for v := range in {
		fmt.Println(v)
	}
}

func writer() <-chan int {
	c := make(chan int)
	go func() {
		for i := range 10 {
			c <- i + 1
		}
		close(c)
	}()
	return c
}

func double(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		for n := range in {
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(1000)))
			out <- n * n
		}
		close(out)
	}()
	return out
}

func longWork() {
	time.Sleep(time.Second * 1)
}

func predictableWork(maxDuration time.Duration) {

	ch := make(chan struct{})
	go func() {
		longWork()
		close(ch)
	}()

	select {
	case <-time.After(maxDuration * time.Second):
		fmt.Println("timeout")
	case <-ch:
		fmt.Println("finish work")
	}

}

func worker(id int, f func(int) int, in <-chan int, out chan<- int) {
	for v := range in {
		fmt.Println("worker", id, "got value", f(v))
		out <- f(v)
	}
}

func poolWorkers() {
	count := 3
	f := func(i int) int {
		return i * i
	}

	tasks := make(chan int)
	results := make(chan int)

	wg := &sync.WaitGroup{}
	wg.Add(count)
	for i := range count {
		go func() {
			defer wg.Done()
			worker(i, f, tasks, results)
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	go func() {
		for i := range 10 {
			tasks <- i
		}
		close(tasks)
	}()

	for v := range results {
		fmt.Println(v)
	}

}
