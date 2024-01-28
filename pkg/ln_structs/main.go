package main

import "fmt"

type A struct {
	Val *string
	B
}

func (a *A) Kek() {
	fmt.Println("A")
}

type B struct {
}

func (b *B) Kek() {
	fmt.Println("B")
}

type car struct {
	model string
	owner *string
}

// https://waclawthedev.medium.com/golang-trap-how-to-copy-structs-properly-9cb2dd4c0832
func main() {
	copyPoint()
	//a := &A{}
	//a.Kek()
	//
	//str := "kek"
	//a1 := A{Val: &str}
	//var a2 A
	//a2 = a1
	//
	//*a2.Val = "lul"
	//fmt.Println(*a1.Val)
	//fmt.Println(*a2.Val)
	//a.b.Kek()

}
func copyPoint() {
	c := car{
		model: "BMW",
		owner: getStrPtr("John"),
	}

	cBadCopy := c

	fmt.Printf("cBadCopy %s owner's name: %s\n", cBadCopy.model, *cBadCopy.owner)
	*c.owner = "Antony1"
	c.model = "Audi1"
	fmt.Printf("cBadCopy  %s owner's name: %s\n", cBadCopy.model, *cBadCopy.owner)

	cCopy := c
	cCopy.owner = new(string) //Create new pointer replacing original
	*cCopy.owner = *c.owner   //Copying by accessing to data

	fmt.Printf("cCopy %s owner's name: %s\n", cCopy.model, *cCopy.owner)
	*c.owner = "Antony2"
	c.model = "Audi2"
	fmt.Printf("cCopy %s owner's name: %s", cCopy.model, *cCopy.owner)
}

func getStrPtr(s string) *string {
	return &s
}
