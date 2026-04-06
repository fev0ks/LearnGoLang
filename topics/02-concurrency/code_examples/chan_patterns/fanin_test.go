package chan_patterns

import (
	"fmt"
	"sync"
	"testing"
)

func TestFanIn(t *testing.T) {
	var chans [10]chan int

	for i := range chans {
		chans[i] = make(chan int)

		go func(ch chan int) {
			for j := range 10 {
				v := 100*i + j + 1
				fmt.Println(v)
				ch <- v
			}
			close(ch)
		}(chans[i])
	}

	out := fanin(chans)
	for v := range out {
		fmt.Println(v)
	}
}

func fanin(chans [10]chan int) chan int {
	out := make(chan int)

	go func() {
		wg := sync.WaitGroup{}

		for _, ch := range chans {
			wg.Add(1)
			go func() {
				defer wg.Done()
				fmt.Println("goroutine")
				for v := range ch {
					out <- v
				}
				//for {
				//	select {
				//	case <-ctx.Done():
				//		fmt.Println("done")
				//		return
				//	case v, ok := <-ch:
				//		if !ok {
				//			return
				//		}
				//		fmt.Printf("out %d ", v)
				//		out <- v
				//	}
				//}
			}()
		}

		wg.Wait()
		close(out)
	}()

	return out
}
