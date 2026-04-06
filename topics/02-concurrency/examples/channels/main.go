package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Файл показывает базовые ловушки с каналами:
// nil channel, closed channel, копирование channel value и работу select.
func main() {
	//copyInChan()
	//ctxCancel()
	//buffCh()

	//closeChStr()
	//closeCh()
	nilChan()
	//fmt.Println(distributedQueryOnyOne())
	//fmt.Println(distributedQueryAll())
	//newChan()
	//chanInSelectNil()
	//copyChan()
}

type A struct {
	s  string
	s2 *string
}

func copyChan() {
	ch1 := make(chan string)
	ch2 := ch1 // обе переменные указывают на один и тот же channel object

	go func() {
		for v := range ch1 {
			fmt.Println("ch1", v)
		}
	}()
	go func() {
		for v := range ch2 {
			fmt.Println("ch2", v)
		}
	}()
	ch1 <- "kek"
	ch2 <- "kek2"
	close(ch2) // закрыли общий канал; это то же самое, что close(ch1)
	ch1 <- "kek" // panic: send on closed channel
	close(ch1)
}

func k(ch *chan string) {
	<-*ch

}

func nilChan() {
	var ch chan string
	fmt.Printf("nil %p, is nil %t\n", &ch, ch == nil)
	go func() {

		//fmt.Printf("ch val1: %s\n", <-ch)
		//time.Sleep(time.Second)
		//fmt.Printf("ch val2: %s\n", <-ch)

		for {
			fmt.Printf("for make %p, is nil %t\n", &ch, ch == nil)
			select {
			case v, ok := <-ch:
				// Пока ch == nil, этот case навсегда disabled.
				// После присваивания make(...) тот же select начнет реально читать канал.
				if !ok {
					return
				}
				fmt.Printf("ch val1: %s\n", v)
			default:
				time.Sleep(500 * time.Millisecond)
			}
		}

		//for str := range ch {
		//	fmt.Printf("loop %p\n", &ch)
		//	fmt.Println(str)
		//}
	}()
	time.Sleep(time.Second)
	ch = make(chan string, 2)
	ch <- "kek"
	fmt.Printf("make %p, is nil %t\n", &ch, ch == nil)
	time.Sleep(time.Second)
	ch = make(chan string, 3)
	ch <- "kek2"
	fmt.Printf("make2 %p, is nil %t\n", &ch, ch == nil)
	close(ch)
	//for v := range ch {
	//	fmt.Println(v)
	//}
	time.Sleep(3 * time.Second)
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
	fmt.Println(<-ch) // 0: чтение из закрытого канала возвращает zero value
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
		aCopy.s = "new s1;" // строковое поле скопировалось по значению
		//str2 := "new s2;" // new adr
		//aCopy.s2 = &str2 // another addr
		*aCopy.s2 = "new s2;" // а pointer внутри struct по-прежнему смотрит на те же данные
		fmt.Println(aCopy.s, *aCopy.s2, aCopy.s2)
	}()
	ch1 <- a
	wg.Wait()
	fmt.Println(a.s, *a.s2, a.s2) // s не изменился, а вот s2 изменился через общий адрес

	//var ch2 chan string
	//go func() {
	//	fmt.Println(<-ch2) //goroutine 1 [chan send (nil chan)]:
	//}()
	//ch2 <- "" // fatal error: all goroutines are asleep - deadlock!

	ch3 := make(chan string)
	close(ch3)
	//ch3 <- "" //panic: send on closed channel

	v, ok := <-ch3
	fmt.Printf("from closed chan '%s' closed?=%v", v, ok) // "" false
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
			close(sc) // дальше чтения из sc вернут zero value
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
	fmt.Println(<-sc) // 1
	fmt.Println(<-sc) // 0
	fmt.Println(<-sc) // 0
	fmt.Println(<-sc) // 0
}

func closeChStr() {
	sc := make(chan struct{})
	//sc := make(chan int)
	//cA, clA := context.WithCancel(context.Background())
	//cB, _ := context.WithTimeout(context.Background(), 4*time.Second)

	for i := 0; i < 5; i++ {
		i := i
		go func() {
		Loop:
			for {
				select {
				case <-sc:
					fmt.Printf("finished %d\n", i)
					break Loop
					//sc <- 1
					//close(sc) // if there is no close then panic deadlock
				default:
					fmt.Printf("working %d\n", i)
					time.Sleep(time.Second)
				}
			}
		}()
	}

	time.Sleep(5 * time.Second)
	close(sc)
	time.Sleep(5 * time.Second)
	fmt.Println("done")
}

type Replica struct {
}

func (replica *Replica) Query(query string) string {
	rand.NewSource(time.Now().UnixNano())
	sec := rand.Intn(3) + 1
	time.Sleep(time.Duration(sec) * time.Second)
	return fmt.Sprintf("%d %s", sec, query)
}

func distributedQueryOnyOne() string {
	replicas := [3]*Replica{}
	responseCh := make(chan string, 1) // буфер 1 нужен, чтобы первый ответ успел записаться даже если reader еще не читает
	for i, replica := range replicas {
		i = i
			query := fmt.Sprintf("%s-%d", "query", i)
			go func() {
				select {
				case responseCh <- replica.Query(query): // Query выполняется до попытки записи в канал
				default:
					fmt.Println("late resp", query)
					//responseCh <- "default!"
			}
		}()
	}
	time.Sleep(4 * time.Second)
	return <-responseCh
}

func distributedQueryAll() string {
	wg := &sync.WaitGroup{}
	replicas := [3]*Replica{}
	responseCh := make(chan string, len(replicas))

	for i, replica := range replicas {
		i = i
		wg.Add(1)
		go func() {
			defer wg.Done()
			query := fmt.Sprintf("%s-%d", "query", i)
			responseCh <- replica.Query(query)
		}()
	}
	go func() {
		wg.Wait()
		close(responseCh)
	}()
	result := ""
	for resp := range responseCh {
		result = fmt.Sprintf("%s\n%s", result, resp)
	}
	return result
}

func newChan() {
	ch1 := make(chan string)
	ch2 := make(chan string)

	go func() {
		ch1 <- "ch1"
	}()

	ch1 = ch2

	go func() {
		ch2 <- "ch2"
	}()

	fmt.Println(<-ch1)

	chBuff1 := make(chan string, 5)
	chBuff2 := make(chan string, 5)

	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(time.Second)
			chBuff1 <- fmt.Sprintf("ch1 - %d", i)
		}
		close(chBuff1)
	}()

	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(time.Second)
			chBuff2 <- fmt.Sprintf("ch2 - %d", i)
		}
		close(chBuff2)
	}()

	go func() {
		time.Sleep(2 * time.Second)
		chBuff1 = chBuff2
	}()

	//go func() {
	//	for val := range chBuff1 {
	//		fmt.Println(val)
	//	}
	//}()

	time.Sleep(3 * time.Second)
	go func() {
		for val := range chBuff1 {
			fmt.Println(val)
		}
	}()
	//for val := range chBuff1 {
	//	fmt.Println(val)
	//}
	time.Sleep(6 * time.Second)
}

func chanInSelectNil() {
	var ch1 chan string
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		for {
			time.Sleep(time.Millisecond * 300)
			select {
			case s := <-ch1:
				fmt.Println(s)
			case <-ctx.Done():
				fmt.Println("done")
				return
			default:
				fmt.Println("empty val")
			}
		}
	}()

	time.Sleep(time.Second)
	ch1 = make(chan string)
	ch1 <- "kek"
	time.Sleep(time.Second)
	cancel()
	time.Sleep(time.Second)
}
