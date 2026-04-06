package main

import (
	"log"
	"os"
	"runtime/trace"
	"time"
)

func main() {
	f, err := os.Create("trace.out")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Включаем трассировку
	if err := trace.Start(f); err != nil {
		log.Fatal(err)
	}
	defer trace.Stop()

	// Создаём несколько горутин, которые быстро завершаются
	for i := 0; i < 5; i++ {
		go func(i int) {
			log.Printf("Goroutine #%d is running", i)
			time.Sleep(100 * time.Millisecond)
			log.Printf("Goroutine #%d is done", i)
		}(i)
	}

	time.Sleep(1 * time.Second) // Даем время горутинам завершиться
}
