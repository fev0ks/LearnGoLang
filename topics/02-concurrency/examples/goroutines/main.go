package main

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"runtime"
	"sync"
	"time"
)

type Message string

type Do struct {
	do func(message Message)
}

func main() {
	//appendSlice()
	//ctxCancel()
	//semaphoreWorkers()
	//errgroupLrn()
	sumCuncurrent()
}

var sem = semaphore.NewWeighted(2) // Разрешаем 2 горутинам работать одновременно

func semaphoreWorkers() {
	for i := 0; i < 5; i++ {
		go worker(i)
	}
	time.Sleep(6 * time.Second)
}

func worker(id int) {
	ctx := context.TODO()
	if err := sem.Acquire(ctx, 1); err != nil {
		return
	}
	defer sem.Release(1)

	fmt.Println("Горутина", id, "работает")
	time.Sleep(2 * time.Second)
}

func appendSlice() {

	sl := make([]int, 0)
	ch := make(chan int, 10)
	wg := sync.WaitGroup{}
	finish := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i < 10; i++ {
			ch <- i
		}
		time.Sleep(time.Millisecond * 300)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 10; i < 20; i++ {
			ch <- i
		}
		time.Sleep(time.Millisecond * 200)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 20; i < 30; i++ {
			ch <- i
		}
		time.Sleep(time.Millisecond * 100)
	}()

	go func() {
		for i := range ch {
			sl = append(sl, i)
		}
		time.Sleep(time.Millisecond * 1000)
		finish <- struct{}{}
	}()

	wg.Wait()
	close(ch)
	<-finish
	fmt.Println(sl, len(sl), cap(sl))
}

func updateVar() {
	var m Message

	go func() {
		//time.Sleep(100)
		m = "Kek"
	}()
	d := Do{
		func(m Message) {
			fmt.Println(m)
		}}
	//time.Sleep(300)
	d.do(m)

	time.Sleep(1000)
}

func ctxCancel() {
	sc := make(chan struct{})
	contextA, cancelContextA := context.WithCancel(context.Background())
	contextB, _ := context.WithTimeout(context.Background(), 4*time.Second)

	go func() {
		for {
			select {
			case <-contextA.Done():
				fmt.Println("A")
				sc <- struct{}{}
			case <-contextB.Done():
				fmt.Println("B")
				sc <- struct{}{}
			}
		}
	}()

	time.Sleep(2 * time.Second)
	cancelContextA()
	<-sc
	fmt.Println("end")
}

func errgroupLrn() {
	var g errgroup.Group

	for i := 1; i <= 3; i++ {
		i := i // Копируем переменную, чтобы избежать ошибки замыкания
		g.Go(func() error {
			return task(i)
		})
	}

	// Ждем завершения всех горутин
	if err := g.Wait(); err != nil {
		fmt.Println("Ошибка:", err)
	} else {
		fmt.Println("Все задачи выполнены успешно")
	}
}

func task(id int) error {
	time.Sleep(time.Duration(id) * time.Second)
	if id == 2 {
		return fmt.Errorf("ошибка в task %d", id)
	}
	fmt.Println("task", id, "завершен")
	return nil
}

func sumCuncurrent() {
	runtime.GOMAXPROCS(1) // если 1, то сумма будет 1000, тк нет одновременного изменения горутинами
	var wg sync.WaitGroup
	sum := 0

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			sum = sum + 1
			wg.Done()
		}()
	}

	// wait until all 1000 goroutines are done
	wg.Wait()

	// value of i should be 1000
	fmt.Println("value of sum after 1000 operations is", sum)
}
