package main

import (
	"context"
	"fmt"
	"time"
)

type Message string

type Do struct {
	do func(message Message)
}

func main() {
	ctxCancel()
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
