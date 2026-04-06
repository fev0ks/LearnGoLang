package main

import (
	"golang.org/x/sys/unix"
	"time"
)

func main() {
	for i := 0; i < 50000; i++ {
		go func() {
			var buf [1]byte
			unix.Read(unix.Stdin, buf[:])
		}()
	}

	time.Sleep(time.Minute)
}
