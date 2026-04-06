package main

import (
	"fmt"
	//"golang.org/x/net/netpoll"
	"io"
	"log"
	"net"
)

func main() {
	// Создаём netpoll
	poller, err := netpoll.New(nil)
	if err != nil {
		log.Fatal(err)
	}

	// Создаём сервер
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	fmt.Println("Сервер слушает :8080...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Ошибка Accept:", err)
			continue
		}

		// Делаем сокет неблокирующим
		desc := netpoll.Must(netpoll.HandleRead(conn.(*net.TCPConn)))

		// Регистрируем обработчик событий
		poller.Start(desc, func(ev netpoll.Event) {
			if ev&netpoll.EventReadHup != 0 { // Клиент отключился
				poller.Stop(desc)
				conn.Close()
				return
			}

			// Читаем данные
			buf := make([]byte, 1024)
			n, err := conn.Read(buf)
			if err == io.EOF {
				poller.Stop(desc)
				conn.Close()
				return
			}
			if err != nil {
				log.Println("Ошибка чтения:", err)
				return
			}

			// Отправляем ответ
			conn.Write([]byte(fmt.Sprintf("Получено: %s\n", buf[:n])))
		})
	}
}
