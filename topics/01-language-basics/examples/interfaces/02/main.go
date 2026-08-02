package main

import "fmt"

type Counter struct{ n int }

func (c *Counter) Inc() { c.n++ }

type Incer interface{ Inc() }

func main() {
	// Это работает:
	c1 := Counter{}
	var i Incer = &c1 // OK: &c адресуема
	fmt.Println(i)

	// Это тоже работает (компилятор может взять адрес):
	c2 := Counter{}
	c2.Inc() // компилятор переписывает в (&c).Inc() — c адресуема
	fmt.Println(c2.n)

	// Это НЕ работает (non-addressable):
	//var i2 Incer = c2 // ОШИБКА: Cannot use c2 (type Counter) as the type Incer 	Type does not implement Incer as the Inc method has a pointer receiver
	//var i2 Incer = Counter{} // ОШИБКА: Counter{} — временное значение, не адресуемо
	//Counter{}.Inc()         // ОШИБКА: нельзя взять адрес временного значения

	//m := map[string]Counter{}
	//m["x"].Inc() // ОШИБКА: элемент map не адресуем
}
