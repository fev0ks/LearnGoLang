package main

import (
	"fmt"
	"net"
)

func main() {
	host := "dnkbit.one" // замените на нужный домен
	ips, err := net.LookupIP(host)
	if err != nil {
		fmt.Println("Ошибка:", err)
		return
	}

	for _, ip := range ips {
		fmt.Println("IP:", ip)
	}
}
