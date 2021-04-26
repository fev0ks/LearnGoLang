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
	fmt.Printf("\n***\nStartThreads")
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
	fmt.Printf("\n***\nChan\n")
	nonBufferChan()
	time.Sleep(time.Duration(1000 * sleepMs))
	bufferChan()
	time.Sleep(time.Duration(1000 * sleepMs))
	bufferChanReadAll()
	time.Sleep(time.Duration(1000 * sleepMs))
	positivesChan(7)
	time.Sleep(time.Duration(1000 * sleepMs))
}

//***
//nonBufferChan
//write 0 message = Im a nonBufferChan 0
//read 0, message = Im a nonBufferChan 0, ok = true
//read 1, message = Im a nonBufferChan 1, ok = true
//write 1 message = Im a nonBufferChan 1
//write 2 message = Im a nonBufferChan 2
//read 2, message = Im a nonBufferChan 2, ok = true
//read 3, message = Im a nonBufferChan 3, ok = true
//write 3 message = Im a nonBufferChan 3
//write 4 message = Im a nonBufferChan 4
//read 4, message = Im a nonBufferChan 4, ok = true

func nonBufferChan() {
	fmt.Printf("\n***\nnonBufferChan\n")
	in := make(chan string, 0) // Создание небуферизованного канала in
	go write(5, in, "nonBufferChan")
	go read(5, in)
	time.Sleep(time.Duration(10 * sleepMs))
}

//***
//bufferChan
//write 0 message = Im a bufferChan 0
//write 1 message = Im a bufferChan 1
//read 0, message = Im a bufferChan 0, ok = true
//read 1, message = Im a bufferChan 1, ok = true
//write 2 message = Im a bufferChan 2
//read 2, message = Im a bufferChan 2, ok = true
//read 3, message = Im a bufferChan 3, ok = true
//write 3 message = Im a bufferChan 3
//write 4 message = Im a bufferChan 4
//write 5 message = Im a bufferChan 5
//write 6 message = Im a bufferChan 6
//read 4, message = Im a bufferChan 4, ok = true
//read 5, message = Im a bufferChan 5, ok = true
//chan still open r2 = Im a bufferChan 6
//chan closed r2 = ''

func bufferChan() {
	fmt.Printf("\n***\nbufferChan\n")
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
	r2, ok = <-out // чтение с проверкой закрытия канала
	if ok {        // если ok == true - канал открыт
		fmt.Printf("chan stiil open r2 = %v\n", r2)
	} else { // если канал закрыт, делаем что-то ещё
		fmt.Printf("chan closed r2 = '%v'\n", r2)
	}
}

//***
//bufferChanReadAll
//write 0 message = Im a bufferChanReadAll 0
//read next = Im a bufferChanReadAll 0
//write 1 message = Im a bufferChanReadAll 1
//write 2 message = Im a bufferChanReadAll 2
//read next = Im a bufferChanReadAll 1
//read next = Im a bufferChanReadAll 2
//write 3 message = Im a bufferChanReadAll 3
//write 4 message = Im a bufferChanReadAll 4
//write 5 message = Im a bufferChanReadAll 5
//read next = Im a bufferChanReadAll 3
//read next = Im a bufferChanReadAll 4
//read next = Im a bufferChanReadAll 5
//read next = Im a bufferChanReadAll 6
//write 6 message = Im a bufferChanReadAll 6

func bufferChanReadAll() {
	fmt.Printf("\n***\nbufferChanReadAll\n")
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

//***
//positivesChan
//write 0 message = 0
//write 1 message = -1
//write to positive chan = 0
//write 2 message = 2
//write 3 message = -3
//write to positive chan = 2
//write 4 message = 4
//write 5 message = -5
//read next = 0
//write to positive chan = 4
//read next = 2
//read next = 4
//write 6 message = 6
//write to positive chan = 6
//read next = 6
//finish read next = 0

func positivesChan(count int) {
	fmt.Printf("\n***\npositivesChan\n")
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
