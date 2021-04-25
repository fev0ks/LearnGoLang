package multithreading

import (
	"fmt"
	"math"
	"time"
)

const sleepMs = 10

func server(i int) {
	for {
		println(i)
		time.Sleep(sleepMs)
	}
}

func StartThreads(count int) {
	for i := 0; i < count; i++ {
		go server(i)
	}
	var g int
	go func(i int) {
		s := 0
		for j := 0; j < i; j++ {
			s += j
			fmt.Printf("%v ", s)
		}
		g = s
		fmt.Printf("\ng in thread = %v\n", g)
	}(10)
	fmt.Printf("base g = %v\n", g) //is not updated
	time.Sleep(time.Duration(sleepMs * 2 * count))
}

func Chan() {
	//nonBufferChan()
	//time.Sleep(time.Duration(100 * sleepMs))
	//bufferChan()
	//time.Sleep(time.Duration(100 * sleepMs))
	//bufferChanReadAll()
	//time.Sleep(time.Duration(100 * sleepMs))
	positivesChan(15)
	time.Sleep(time.Duration(1000 * sleepMs))
}

func nonBufferChan() {
	in := make(chan string, 0) // Создание небуферизованного канала in
	go write(5, in, "nonBufferChan")
	go read(5, in)
	time.Sleep(time.Duration(10 * sleepMs))
}

func bufferChan() {
	out := make(chan string, 2) // Создание буферизованного канала out
	go write(7, out, "bufferChan")
	go read(6, out)
	time.Sleep(time.Duration(10 * sleepMs))
	r2, ok := <-out // чтение с проверкой закрытия канала
	if ok {         // если ok == true - канал открыт
		fmt.Printf("chan still open r2 = %v\n", r2)
	} else { // если канал закрыт, делаем что-то ещё
		fmt.Printf("chan closed r2 = %v\n", r2)
	}
	close(out)
	r2, ok = <-out // чтение с проверкой закрытия канала
	if ok {        // если ok == true - канал открыт
		fmt.Printf("chan stiil open r2 = %v\n", r2)
	} else { // если канал закрыт, делаем что-то ещё
		fmt.Printf("chan closed r2 = '%v'\n", r2)
	}
}

func bufferChanReadAll() {
	out := make(chan string, 2) // Создание буферизованного канала out
	go write(7, out, "bufferChanReadAll")
	go readAllString(out)
	time.Sleep(time.Duration(10 * sleepMs))
}

func write(count int, in chan string, typeChan string) {
	for i := 0; i < count; i++ {
		message := fmt.Sprintf("Im a %v %v", typeChan, i)
		in <- message
		fmt.Printf("write %v message = %v\n", i, message)
	}
	close(in)
}

func read(count int, in chan string) {
	for i := 0; i < count; i++ {
		message, ok := <-in
		fmt.Printf("read %v, message = %v, ok = %v\n", i, message, ok)
	}
}

func readAllString(in chan string) {
	for next := range in {
		fmt.Printf("read next = %v\n", next)
	}
}

func positivesChan(count int) {
	in := make(chan int)
	go writeNumbers(count, in)
	result := positives(in)
	go readChanInt(result)
}

func readChanInt(in <-chan int) {
	var next int
	ok := true
	for ok {
		next, ok = <-in
		if ok {
			fmt.Printf("read next = %v\n", next)
		} else {
			fmt.Printf("finish read next = %v\n", next)
		}
	}
}

func writeNumbers(count int, in chan int) {
	for i := 0; i < count; i++ {
		message := i * int(math.Pow(-1, float64(i)))
		fmt.Printf("write %v message = %v\n", i, message)
		in <- message
	}
	close(in)
}

func positives(in <-chan int) <-chan int {
	out := make(chan int, 2)
	go func() {
		// Цикл далее будет выполняться, пока канал in не закрыт
		for next := range in {
			if next >= 0 {
				fmt.Printf("write to positive chan = %v\n", next)
				out <- next
			}
		}
		close(out)
	}()
	return out
}
