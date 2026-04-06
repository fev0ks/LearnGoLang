package types

import "fmt"

type Stringer interface {
	String() string
}

type Type interface {
	GetType() string
}

type Animal struct {
	AnimalType string
	Name       string
	Weight     float32
	Height     int
}

type Cat struct {
	AnimalType string
	Name       string
	Weight     float32
	Height     int
}

type Dog struct {
	Animal
}

func NewCat(name string, weight float32, height int) *Cat {
	return &Cat{"cat", name, weight, height}
}

func NewDog(name string, weight float32, height int) *Dog {
	return &Dog{newAnimal("dog", name, weight, height)}
}

func newAnimal(animalType string, name string, weight float32, height int) Animal {
	return Animal{animalType, name, weight, height}
}

func Print(s Stringer) {
	fmt.Println(s.String())
}

func PrintAnimalType(s Type) {
	fmt.Printf("\nType of animal = %s", s.GetType())
}

func (a Animal) String() string {
	return fmt.Sprintf("Animal{type = %s, name = '%s', weight = %v, height = %v}", a.AnimalType, a.Name, a.Weight, a.Height)
}

func (c Cat) String() string {
	return fmt.Sprintf("Animal{type = %s, name = '%s', weight = %v, height = %v}", c.AnimalType, c.Name, c.Weight, c.Height)
}

func (a Animal) GetType() string {
	return a.AnimalType
}

func (a *Animal) UpdateAnimalName(name string) {
	a.Name = name
	fmt.Printf("\na.String(): %v", a.String())
}
