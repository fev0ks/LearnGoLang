package main

import (
	"fmt"
	"time"
)

type signer struct {
	now func() time.Time
}

func newSigner() *signer {
	return &signer{
		now: time.Now,
	}
}

func (s *signer) Sign(msg string) {
	now := s.now()
	fmt.Println(msg, now)
}

func main() {
	s1 := newSigner()
	s1.Sign("kek")
	time.Sleep(10 * time.Second)
	s1.Sign("kek2")
}
