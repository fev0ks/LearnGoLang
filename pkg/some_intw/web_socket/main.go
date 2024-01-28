package websocket_example

import (
	"sync"
	"sync/atomic"
)

type Message struct {
	UserID  int
	Payload string
}

type Connection struct {
	UserID   int
	DeviceID string
}

func (c *Connection) Write(p []byte) (n int, err error) {
	// Pretend it is implemented
	return 0, nil
}

type WSServer struct {
	connectedClientsCount uint64
	connections           map[int]*Connection
	sync.RWMutex
}

func (w *WSServer) handleConnect(c Connection) {
	atomic.AddUint64(&w.connectedClientsCount, 1)
	w.Lock()
	w.connections[c.UserID] = &c
}

func (w *WSServer) handleDisconnect(c Connection) {
	atomic.AddUint64(&w.connectedClientsCount, -1)
}

func (w *WSServer) totalConnectedClients() uint64 {
	return w.connectedClientsCount
}

func (w *WSServer) handleQueueMessages(messages []Message) (int, error) {
	for i, m := range messages {
		err := w.sendToConnectedDevices(m)
		if err != nil {
			return i, err
		}
	}
	return len(messages), nil
}

func (w *WSServer) sendToConnectedDevices(m Message) error {
	// TODO
	return nil
}
