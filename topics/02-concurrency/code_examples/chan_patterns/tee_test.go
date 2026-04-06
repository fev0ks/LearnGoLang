package chan_patterns

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestTee(t *testing.T) {
	//ch1, ch2 := split1(generator())
	//ch1, ch2 := split2(generator())
	//ch1, ch2 := split3(generator())
	//ch1, ch2 := split4(generator())
	ch1, ch2 := split5(generator())
	wg := sync.WaitGroup{}

	wg.Add(2)
	go func() {
		defer wg.Done()
		for v := range ch1 {
			fmt.Println("ch1 = ", v)
		}
	}()

	go func() {
		defer wg.Done()
		for v := range ch2 {
			time.Sleep(time.Millisecond * 100)
			fmt.Println("ch2 = ", v)
		}
	}()
	wg.Wait()
}

func generator() chan int {
	ch := make(chan int)
	go func() {
		for i := range 10 {
			ch <- i
		}
		close(ch)
	}()
	return ch
}

func split1(in chan int) (chan int, chan int) {

	out1 := make(chan int)
	out2 := make(chan int)

	go func() {
		defer close(out1)
		defer close(out2)

		for v := range in {
			out1 <- v
			out2 <- v
		}
	}()

	return out1, out2
}

func split2(in chan int) (chan int, chan int) {
	out1 := make(chan int)
	out2 := make(chan int)

	go func() {
		defer close(out1)
		defer close(out2)

		for v := range in {
			var out1, out2 = out1, out2
			for range 2 {
				select {
				case out1 <- v:
					out1 = nil
				case out2 <- v:
					out2 = nil
				}
			}
		}
	}()
	return out1, out2
}

func split3(in chan int) (chan int, chan int) {
	out1 := make(chan int)
	out2 := make(chan int)

	go func() {
		defer close(out1)
		defer close(out2)

		for v := range in {
			wg := sync.WaitGroup{}

			wg.Add(1)
			go func() {
				defer wg.Done()
				out1 <- v
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				out2 <- v
			}()

			wg.Wait()
		}
	}()
	return out1, out2
}

func split4(in chan int) (chan int, chan int) {
	out1 := make(chan int)
	out2 := make(chan int)

	go func() {
		defer close(out1)
		defer close(out2)

		wg1 := sync.WaitGroup{}
		wg2 := sync.WaitGroup{}
		for v := range in {
			wg1.Add(1)
			go func() {
				defer wg1.Done()
				out1 <- v
			}()

			wg2.Add(1)
			go func() {
				defer wg2.Done()
				out2 <- v
			}()
		}
		wg1.Wait()
		wg2.Wait()
	}()
	return out1, out2
}

func split5(in chan int) (chan int, chan int) {
	out1 := make(chan int)
	i1 := 0

	out2 := make(chan int)
	i2 := 0

	done := false

	sl := []int{}
	mu := sync.RWMutex{}

	go func() {
		for v := range in {
			mu.Lock()
			sl = append(sl, v)
			mu.Unlock()
		}
		done = true
	}()

	go func() {
		defer close(out1)
		for {
			mu.RLock()
			if i1 < len(sl) && !done {
				out1 <- sl[i1]
				i1++
			}
			mu.RUnlock()
			if done {
				break
			}
		}
	}()

	go func() {
		defer close(out2)
		for {
			mu.RLock()
			if i2 < len(sl) && !done {
				out2 <- sl[i2]
				i2++
			}
			mu.RUnlock()
			if done {
				break
			}
		}
	}()

	return out1, out2
}
