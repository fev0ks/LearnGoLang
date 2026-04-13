package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

type UserEvent struct {
	Kind string
}

type StreamAck struct {
	Message string
}

type userEventStream interface {
	Context() context.Context
	Send(*UserEvent) error
	Recv() (*StreamAck, error)
	Close() error
}

type localStream struct {
	ctx    context.Context
	cancel context.CancelFunc
	sendCh chan *UserEvent
	recvCh chan *StreamAck
	once   sync.Once
}

func newLocalStream(parent context.Context) *localStream {
	ctx, cancel := context.WithCancel(parent)
	s := &localStream{
		ctx:    ctx,
		cancel: cancel,
		sendCh: make(chan *UserEvent, 1),
		recvCh: make(chan *StreamAck, 1),
	}

	go func() {
		defer close(s.recvCh)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-s.sendCh:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case s.recvCh <- &StreamAck{Message: fmt.Sprintf("ack for %s", event.Kind)}:
				}
			}
		}
	}()

	return s
}

func (s *localStream) Context() context.Context {
	return s.ctx
}

func (s *localStream) Send(event *UserEvent) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sendCh <- event:
		return nil
	}
}

func (s *localStream) Recv() (*StreamAck, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case ack, ok := <-s.recvCh:
		if !ok {
			return nil, io.EOF
		}
		return ack, nil
	}
}

func (s *localStream) Close() error {
	s.once.Do(func() {
		s.cancel()
		close(s.sendCh)
	})
	return nil
}

func main() {
	// Самодостаточный пример bidirectional stream-паттерна без внешних proto import'ов.
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	createStream := func() (userEventStream, error) {
		stream := newLocalStream(ctx)
		log.Println("Stream успешно создан")
		return stream, nil
	}

	// Инициализация stream
	stream, err := createStream()
	if err != nil {
		return
	}

	// Запускаем горутину для отправки ping каждые 5 секунд
	go func() {
		for {
			if stream == nil {
				log.Println("stream is nil")
				break
			}

			select {
			case <-stream.Context().Done():
				log.Println("context done")
				return
			default:
				err := stream.Send(&UserEvent{Kind: "ping"})
				if err != nil {
					log.Printf("Ошибка отправки ping: %v", err)
					return
				}
			}

			log.Println("Ping отправлен")
			time.Sleep(5 * time.Second) // Ждем 5 секунд перед отправкой
		}
	}()

	// Получаем ответы (Pong)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
					log.Printf("Поток завершен: %v", err)
					return
				}
				log.Printf("Ошибка получения ответа: %v", err)
				break
			}
			log.Printf("Получен Pong: %s", resp.Message)
		}
	}()

	<-ctx.Done()
	if err := stream.Close(); err != nil {
		log.Printf("Ошибка закрытия stream: %v", err)
	}
}
