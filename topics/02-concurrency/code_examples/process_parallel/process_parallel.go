package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

//реализовать функцию и processParallel
//прокинуть контекст

func main() {
	in := make(chan int)
	out := make(chan int)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go func() {
		defer close(in)

		for i := range 100 {
			select {
			case in <- i:
			case <-ctx.Done():
				fmt.Println("ctx in done")
				return
			}
		}
	}()

	start := time.Now()
	//processParallel(in, out, 5)

	processParallelCtx(ctx, in, out, 5)

	for v := range out {
		fmt.Println("v =", v)
	}

	fmt.Println("main duration:", time.Since(start))
}

func processParallel(in, out chan int, numWorkers int) {
	wg := &sync.WaitGroup{}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for v := range in {
				out <- processData(v)
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()
}

func processParallelCtx(ctx context.Context, in, out chan int, numWorkers int) {
	wg := sync.WaitGroup{}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		//i := i
		go func() {
			defer wg.Done()

			res := make(chan int)
			go func() {
				for v := range in {
					res <- processData(v)
				}
				close(res)
			}()

			for {
				select {
				case <-ctx.Done():
					fmt.Println("ctx done")
					return
				case v, ok := <-res:
					if !ok {
						return
					}
					out <- v
					fmt.Printf("%d - %d\n", i, v)
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()
}

func processData(v int) int {
	time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
	return v * 2
}
