package multithreading

import (
	"fmt"
	"sync"
	"time"
)

type Box struct {
	c      chan string
	closed bool
	mutex  sync.Mutex
	once   sync.Once
}

func Start() {
	box := &Box{
		make(chan string),
		false,
		sync.Mutex{},
		sync.Once{},
	}
	go writeVal(1, 5, box)
	go writeVal(2, 10, box)
	go readVal(box)
	time.Sleep(1 * time.Second)
	go writeVal(3, 3, box)
	//go box.safeClose()
	go box.safeCloseOnce()
	time.Sleep(3 * time.Second)
}

func (box *Box) IsClosed() bool {
	box.mutex.Lock()
	defer box.mutex.Unlock()
	return box.closed
}

func (box *Box) safeCloseOnce() {
	box.once.Do(func() {
		close(box.c)
		box.closed = true
	})
}

func (box *Box) safeClose() {
	box.mutex.Lock()
	defer box.mutex.Unlock()
	fmt.Println("closing")
	if !box.closed {
		fmt.Println("box is closed")
		box.closed = true
		close(box.c)
	} else {
		fmt.Println("box is already closed")
	}
}

func writeVal(number int, count int, box *Box) {
	for i := 0; i < count; i++ {
		val := fmt.Sprintf("%d = %d", number, i)
		if !box.IsClosed() {
			box.c <- val
			fmt.Println("write ", val)
		} else {
			fmt.Println(number, "box was closed ")
			return
		}
	}
	fmt.Println(number, "end writing")
	box.safeCloseOnce()
	//box.safeClose()
}

func readVal(box *Box) {
	for value := range box.c {
		fmt.Printf("Read value of %s\n", value)
	}
}
