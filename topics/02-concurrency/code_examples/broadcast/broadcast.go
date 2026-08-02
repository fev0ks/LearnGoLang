package broadcast

import (
	"errors"
	"sync"
)

/*
Один писатель пушит значения, N подписчиков получают каждое.

	Подписчик может отписаться в любой момент.
	Писатель не должен блокироваться на медленном подписчике.
	После Close() все каналы подписчиков закрываются.
*/
type Broadcast[T any] struct {
	mu        sync.RWMutex
	observers map[chan T]struct{}
	isClosed  bool
}

func New[T any]() *Broadcast[T] {
	return &Broadcast[T]{
		observers: make(map[chan T]struct{}),
	}
}

func (b *Broadcast[T]) Broadcast(s T) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.isClosed {
		return errors.New("already closed")
	}

	for ch := range b.observers {
		select {
		case ch <- s:
		default:
		}
		//go func() { ch <- s }()
	}
	return nil
}

func (b *Broadcast[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.isClosed {
		return
	}
	b.isClosed = true
	for ch := range b.observers {
		close(ch)
	}
	clear(b.observers)
}

func (b *Broadcast[T]) Subscribe() chan T {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan T, 10)
	if b.isClosed {
		close(ch)
	} else {
		b.observers[ch] = struct{}{}
	}

	return ch
}

func (b *Broadcast[T]) Unsubscribe(ch chan T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.isClosed {
		return
	}
	for c := range b.observers {
		if c == ch {
			delete(b.observers, c)
			close(c)
			return
		}
	}

	delete(b.observers, ch)
}
