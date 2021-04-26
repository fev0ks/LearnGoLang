package main

import (
	"fmt"
	"helloWorld/funcs"
	"helloWorld/logging"
	"helloWorld/multithreading"
	_ "helloWorld/pkg/logging"
	aliasForLogging "helloWorld/pkg/logging"
	"helloWorld/types"
	. "math"
	"reflect"
	"time"
)

const (
	_ = iota
	One
	Two
	Three = iota
	Four
)

func main() {
	//fmt.Printf("one = %v, two = %v, tree = %v, four = %v", One, Two, Three, Four)
	//fmt.Println(muFunc()) // = 1

	//var i interface {}
	//if i == nil { //true
	//}

	//funcs.ThrowPanic(12, 0)

	testVariables()
	testLogger()
	testArray()
	testLoops()
	testTrash()
	testTypes()
	testFunc()
	multithreading.StartThreads(5)
	multithreading.Chan()
	multithreading.WaitGroup()
	testSwitch(2)
	testSwitch(1)
	testSwitch(0)
	testSwitch(99)

}

func muFunc() (i int) {
	defer func() {
		i++
	}()
	i = 2
	return 0
}

//***
//testVariables
//v1 = 100
//v2 = Hello!
//v3 = [0 1 2 3 4 5 6 7 8 9]
//v4 = [1000 2000 12334]
//v5 = 50
//v6 = 0xc00000a098
//v66 = 100
//v7[one] = 1
//v7[zero] = 0
//v8(10) = 11

func testVariables() {
	fmt.Printf("\n***\ntestVariables")
	var v1 int = 100
	fmt.Printf("\nv1 = %v ", v1)

	var v2 string = "Hello!"
	fmt.Printf("\nv2 = %v ", v2)

	var v3 = [10]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9} //[0 1 2 3 4 5 6 7 8 9]
	fmt.Printf("\nv3 = %v ", v3)

	var v4 = []int{1000, 2000, 12334}
	fmt.Printf("\nv4 = %v ", v4) //[1000 2000 12334]

	var v5 = struct{ f int }{50}
	fmt.Printf("\nv5 = %v ", v5.f)

	var v6 *int = &v1
	fmt.Printf("\nv6 = %v ", v6) //0xc0000ac058

	var v66 = *v6
	fmt.Printf("\nv66 = %v ", v66) //100

	var v7 = map[string]int{"one": 1, "two": 2, "three": 3}
	fmt.Printf("\nv7[one] = %v ", v7["one"])   //v7[one] = 1
	fmt.Printf("\nv7[zero] = %v ", v7["zero"]) //v7[zero] = 0

	var v8 = func(a int) int { return a + 1 }
	fmt.Printf("\nv8(10) = %v ", v8(10)) //v8(10) = 11
}

//***
//testFunc
//first = 2, second = 3, message = incremented by 1
//result = 3, message = sum
//fileName = , err = Ошибка при чтении файла ImNotFile: &{%!g(string=open) %!g(string=ImNotFile) %!g(syscall.Errno=2)}
//imSumFunc(3,5) = 8
//err is not nil PANIC!!!! Ошибка при чтении файла ImNotFile: &{%!g(string=open) %!g(string=ImNotFile) %!g(syscall.Errno=2)}
func testFunc() {
	fmt.Printf("\n***\ntestFunc")
	first, second, message := funcs.IncTwo(1, 2)
	fmt.Printf("\nfirst = %v, second = %v, message = %v", first, second, message)

	result, message := funcs.Sum(1, 2)
	fmt.Printf("\nresult = %v, message = %v", result, message)

	_, _ = funcs.Sum(1, 2)

	fileName, err := funcs.ReadFileName("ImNotFile")
	fmt.Printf("\nfileName = %v, err = %v", fileName, err)

	imSumFunc := func(a, b int) (result int) {
		result = a + b
		return
	}
	//panic(1)
	fmt.Printf("\nimSumFunc(3,5) = %v", imSumFunc(3, 5))
	//funcs.ThrowParsePanic("123", "12.4")
	defer func() {
		if err != nil {
			fmt.Printf("\nerr is not nil PANIC!!!! %v", err)
			//panic(err)
			//Error strconv.ParseInt: parsing "123": value out of range "strconv.ParseInt: parsing "123": value out of range"
			//panic: Ошибка при чтении файла ImNotFile: &{%!g(string=open) %!g(string=ImNotFile) %!g(syscall.Errno=2)}
		}
	}()
	//sort.Slice()
}

//***
//testArray
//updated myNumber: 123 123 123 123 123
//not initiated myNumbers: [0 0 0 0 0]
//notUpdated myNumber: 1 2 3 4 5
//initiated myNumbers2: [1 2 3 4 5]
//[123 123 ]
//
//slice: [1 2   ], slice.len: 5
//slice: [1 2 3 4 ], slice.len: 5
//slice: ['updated in slice 2' 2 3 4 ], slice2: ['updated in slice 2' 2 3 4 ]
//im in slice...
//im in slice...
//im in slice...
//im in slice...
//im in slice...

func testArray() {
	fmt.Printf("\n***\ntestArray")
	var myNumbers [5]int
	fmt.Print("\nupdated myNumber: ")
	for _, myNumber := range myNumbers {
		myNumber = 123
		fmt.Printf("%v ", myNumber) // myNumber is a *copy* of myNumbers's element
	}
	fmt.Printf("\nnot initiated myNumbers: %v ", myNumbers)

	fmt.Println()
	fmt.Print("notUpdated myNumber: ")
	var myNumbers2 = []int{1, 2, 3, 4, 5}
	for _, myNumber := range myNumbers2 {
		fmt.Printf("%v ", myNumber)
	}
	fmt.Printf("\ninitiated myNumbers2: %v ", myNumbers2)

	fmt.Println()

	var array [3]string
	array[0] = "123"
	array[1] = "123"
	fmt.Println(array)

	var slice = make([]string, 5)
	slice[0] = "1"
	slice[1] = "2"
	fmt.Printf("\nslice: %v, slice.len: %v", slice, len(slice))
	slice[2] = "3"
	slice[3] = "4"
	//slice[len(slice)+1] = "out of length" //runtime error: index out of range [11] with length 10
	fmt.Printf("\nslice: %v, slice.len: %v", slice, len(slice))

	var slice2 = slice
	slice2[0] = "'updated in slice 2'"
	fmt.Printf("\nslice: %v, slice2: %v", slice, slice2)
	for range slice {
		fmt.Printf("\nim in slice...")
	}
}

//***
//testSwitch value=2
//>One 2
//***
//testSwitch value=1
//One 1
//Zero 1
//***
//testSwitch value=0
//Zero 0
//***
//testSwitch value=99
//default 99
//99 - 'a' + 10 = 120

func testSwitch(value int) {
	fmt.Printf("\n***\ntestSwitch value=%v", value)
	switch value {
	case 2, 3, 4, 5, 6, 7, 8, 9:
		fmt.Printf("\n>One %v", value)
	case 1:
		fmt.Printf("\nOne %v", value)
		fallthrough
	case 0:
		fmt.Printf("\nZero %v", value)
	default:
		fmt.Printf("\ndefault %v", value)
	}

	//?????????
	switch {
	case '0' <= value && value <= '9':
		fmt.Printf("\n%v  - '0' = %v", value, value-'0')
	case 'a' <= value && value <= 'f':
		fmt.Printf("\n%v - 'a' + 10 = %v", value, value-'a'+10)
	case 'A' <= value && value <= 'F':
		fmt.Printf("\n%v  - 'A' + 10 = %v", value, value-'A'+10)
	}
}

//***
//testLoops
//sumVar: 45
//1 2 3 4 5 6 7 8 9 10 11 finish
//m = map[one:1 three:3 two:2]
//key = blabla, val = 123
//key = blabla, val = 123
//key = blabla, val = 123
//m = map[one:1 three:3 two:2]

func testLoops() {
	fmt.Printf("\n***\ntestLoops")
	var sumVar = 0
	for i := 0; i < 10; i++ {
		sumVar += i
	}
	fmt.Printf("\nsumVar: %v\n", sumVar)

	var count = 0
	for {
		if count > 10 {
			fmt.Printf("finish")
			break
		} else {
			count++
			fmt.Printf("%v ", count)
		}
	}

	var m = map[string]int{"one": 1, "two": 2, "three": 3}
	fmt.Printf("\nm = %v ", m)
	for key, val := range m {
		key = "blabla"
		val = 123
		fmt.Printf("\nkey = %v, val = %v ", key, val)
	}
	fmt.Printf("\nm = %v ", m)
}

//***
//testLogger
//nilLogger = <nil>
//copyOfNilLogger copy of nil as Interface is not nil but nil = <nil>
//reflect.ValueOf(copyOfNilLogger).IsNil copyOfNilLogger = <nil>
//defaultLogger = { false}[Info]  Debug is turned off; defaultLogger
//
//
//bigNullLogger = {{ false} { false}}[Info]  Debug is turned off; bigNullLogger
//[Info]  Debug is turned off; bigNullLogger
//
//[Info] 2021-04-26T11:42:57+03:00 loggerTurnOn - This is a Info statement...
//[Debug] 2021-04-26T11:42:57+03:00 loggerTurnOn - This is a Debug statement...
//[Error] 2021-04-26T11:42:57+03:00 loggerTurnOn - This is a Error statement...
//[Warn] 2021-04-26T11:42:57+03:00 loggerTurnOn - This is a Warn statement...
//
//[Info] 2021-04-26T11:42:57+03:00 Debug is turned off; loggerTurnOff - This is a Info statement...
//[Debug] 2021-04-26T11:42:57+03:00 Debug is turned off; loggerTurnOff - This is a Debug statement...
//[Error] 2021-04-26T11:42:57+03:00 loggerTurnOff - This is a Error statement...
//[Warn] 2021-04-26T11:42:57+03:00 loggerTurnOff - This is a Warn statement...
//
//[Info] 2021-04-26T11:42:57+03:00 Debug is turned off; newLoggerTurnOff - This is a Info statement...
//[Error] 2021-04-26T11:42:57+03:00 newLoggerTurnOff - This is a Error statement...
//[Info] 2021-04-26T11:42:57+03:00 Debug is turned off; loggerTurnOn.SwitchDebug(false) - This is a Info statement...
//
//[Info] 2021-04-26T11:42:57+03:00 loggerTurnOn.SwitchDebug(true) - This is a Info statement...
//
//2021-04-26T11:42:57+03:00 Im aliasForLogging
//[Info] 2021-04-26T11:42:57+03:00 I'm logger[0] = &{2006-01-02T15:04:05Z07:00 true}
//[Info] 2021-04-26T11:42:57+03:00 I'm logger[1] = &{2006-01-02T15:04:05Z07:00 true}

func testLogger() {
	fmt.Printf("\n***\ntestLogger")
	//var newLogger = new (logging.Logger{new (logging.MyInterface), "123", true})
	//logging.Info = logging.Debug //so sad...
	var nilLogger *logging.Logger = nil
	var copyOfNilLogger logging.MyInterface = nilLogger

	if nilLogger == nil {
		fmt.Printf("\nnilLogger = %v", nilLogger)
	}
	//nilLogger.SetDebug(true) // invalid memory address or nil pointer dereference

	if copyOfNilLogger != nil {
		fmt.Printf("\ncopyOfNilLogger copy of nil as Interface is not nil but nil = %v", copyOfNilLogger)
	}

	if reflect.ValueOf(copyOfNilLogger).IsNil() {
		fmt.Printf("\nreflect.ValueOf(copyOfNilLogger).IsNil copyOfNilLogger = %v", copyOfNilLogger)
	}

	var defaultLogger logging.Logger
	fmt.Printf("\ndefaultLogger = %v", defaultLogger)
	defaultLogger.Log(logging.Info, "defaultLogger")

	fmt.Println()
	var bigNullLogger logging.BigLogger
	fmt.Printf("\nbigNullLogger = %v", bigNullLogger)
	bigNullLogger.Log(logging.Info, "bigNullLogger")
	//bigNullLogger.SetDebug(true) //hmmmmm.....doesn't work when there are 2 interfaces with the same methods
	//fmt.Printf("bigNullLogger.GetDebug() = %v\n", bigNullLogger.GetDebug()) //hmmmmm.....invalid memory address or nil pointer dereference - GetDebug is not implemented but available and compiled
	bigNullLogger.Log(logging.Info, "bigNullLogger")

	fmt.Println()
	loggerTurnOn := logging.New(time.RFC3339, true)
	loggerTurnOn.Log(logging.Info, "loggerTurnOn - This is a Info statement...")
	loggerTurnOn.Log(logging.Debug, "loggerTurnOn - This is a Debug statement...")
	loggerTurnOn.Log(logging.Error, "loggerTurnOn - This is a Error statement...")
	loggerTurnOn.Log(logging.Warn, "loggerTurnOn - This is a Warn statement...")

	fmt.Println()
	loggerTurnOff := logging.New(time.RFC3339, false)
	loggerTurnOff.Log(logging.Info, "loggerTurnOff - This is a Info statement...")
	loggerTurnOff.Log(logging.Debug, "loggerTurnOff - This is a Debug statement...")
	loggerTurnOff.Log(logging.Error, "loggerTurnOff - This is a Error statement...")
	loggerTurnOff.Log(logging.Warn, "loggerTurnOff - This is a Warn statement...")

	fmt.Println()
	newLoggerTurnOff := loggerTurnOn.SwitchDebug()
	newLoggerTurnOff.Log(logging.Info, "newLoggerTurnOff - This is a Info statement...")
	newLoggerTurnOff.Log(logging.Error, "newLoggerTurnOff - This is a Error statement...")
	loggerTurnOn.Log(logging.Info, "loggerTurnOn.SwitchDebug(false) - This is a Info statement...")

	fmt.Println()
	loggerTurnOn.SwitchDebug()
	loggerTurnOn.Log(logging.Info, "loggerTurnOn.SwitchDebug(true) - This is a Info statement...")

	fmt.Println()
	aliasForLogging.Debug(true)
	aliasForLogging.Log("Im aliasForLogging")

	//arrayOfLoggersOfDefaults := []logging.Logger{defaultLogger} //doesn't work for * objects
	arrayOfLoggers := []*logging.Logger{loggerTurnOn, newLoggerTurnOff} //doesn't work for not * objects
	//arrayOfBigLoggersOfDefaults := []logging.BigLogger{bigNullLogger} //doesn't work for Logger objects
	sliceOfLoggers := make([]*logging.Logger, 0)
	sliceOfLoggers = append(sliceOfLoggers, nilLogger)
	//sliceOfLoggers  = append(sliceOfLoggers, copyOfNilLogger) //doesn't work for objects of Interface
	//logging.PrintLoggers(arrayOfLoggersOfDefaults) //doesn't work for * objects
	logging.PrintLoggers(arrayOfLoggers)
	//logging.PrintLoggers(sliceOfLoggers) //invalid memory address or nil pointer dereference
	//logging.PrintLoggers(arrayOfBigLoggersOfDefaults) //doesn't work for BigLogger objects

}

//***
//testTypes
//cat: Animal{type = cat, name = 'Snow', weight = 7.5, height = 40}
//cat.String(): Animal{type = cat, name = 'Snow', weight = 7.5, height = 40}Animal{type = cat, name = 'Snow', weight = 7.5, height = 40}
//
//nullCat: Animal{type = , name = '', weight = 0, height = 0}
//dog: Animal{type = dog, name = 'Sharik', weight = 70.5, height = 80}
//dog.GetType(): dog
//Type of animal = dog
//cat.GetType(): cat
//Type of animal = cat
//newCat: Animal{type = cat, name = 'Snow', weight = 7.5, height = 40}
//newCat.String(): Animal{type = cat, name = 'Snow', weight = 7.5, height = 40}Animal{type = cat, name = 'Snow', weight = 7.5, height = 40}
//
//newDog: Animal{type = dog, name = 'Sharik', weight = 70.5, height = 80}
//newDog.GetType(): dog
//Type of animal = dog

func testTypes() {
	fmt.Printf("\n***\ntestTypes")
	cat := types.Animal{AnimalType: "cat", Name: "Snow", Weight: 7.5, Height: 40}
	fmt.Printf("\ncat: %v", cat)
	fmt.Printf("\ncat.String(): %v", cat.String())

	types.Print(cat)

	var nullCat types.Animal
	fmt.Printf("\nnullCat: %v", nullCat)

	dog := types.Dog{Animal: types.Animal{AnimalType: "dog", Name: "Sharik", Weight: 70.5, Height: 80}}
	fmt.Printf("\ndog: %v", dog)
	//types.Print(dog) //Type does not implement 'Stringer' as some methods are missing: String() string

	fmt.Printf("\ndog.GetType(): %v", dog.GetType())
	types.PrintAnimalType(dog)
	fmt.Printf("\ncat.GetType(): %v", cat.GetType())
	types.PrintAnimalType(cat)

	newCat := types.NewCat("Snow", 7.5, 40)
	fmt.Printf("\nnewCat: %v", newCat)
	fmt.Printf("\nnewCat.String(): %v", newCat.String())
	types.Print(newCat)

	newDog := types.NewDog("Sharik", 70.5, 80)
	fmt.Printf("\nnewDog: %v", newDog)
	//types.Print(dog) //Type does not implement 'Stringer' as some methods are missing: String() string

	fmt.Printf("\nnewDog.GetType(): %v", newDog.GetType())
	types.PrintAnimalType(newDog)
	//fmt.Printf("newCat.GetType(): %v \n", newCat.GetType()) //newCat is not Animal, (a Animal) GetType()
	//types.PrintAnimalType(newCat)
}

//***
//testTrash
//Hello world
//
//defaultBool = false
//sum(1, 2, 123) = 126
//sum(1, 2, 3) = 6
//sum(1, 2, 0) = 3
//str1 = 123
//
//155 == 155 = truestr1 = 123, str2 = 123321
//str1 = 123, str3 = 0xc00004a6f0
//funcs.SumTreeNumbers(defaultInt, 2, 3) is less than 7
//
//maxInt = 127
//maxInt8PlusOne = -128
//Sin(1.0) 0.8414709848078965(4+6i)
//(-2-2i)
//(-5+10i)
//-5
//10

func testTrash() {
	fmt.Printf("\n***\ntestTrash")
	a := 1
	b := 2
	c := 3
	var variable = 123
	var str1 = "123"
	str2 := ""
	var num1 = 155
	num2 := 155
	var defaultInt int
	var defaultBool bool
	var maxInt = MaxInt8

	fmt.Println("\nHello world")
	fmt.Printf("\ndefaultBool = %t", defaultBool)
	fmt.Printf("\nsum(%v, %v, %v) = %v", a, b, variable, funcs.SumTreeNumbers(a, b, variable))
	fmt.Printf("\nsum(%v, %v, %v) = %v", a, b, c, funcs.SumTreeNumbers(a, b, c))
	fmt.Printf("\nsum(%v, %v, %v) = %v", a, b, defaultInt, funcs.SumTreeNumbers(a, b, defaultInt))

	fmt.Printf("\nstr1 = %s\n", str1)
	fmt.Printf("\n%v == %v = %v", num1, num2, num1 == num2)

	str2 = str1
	str2 += "321"
	//str2 += 321 //Invalid operation
	fmt.Printf("str1 = %s, str2 = %s\n", str1, str2)

	str3 := &str1
	//str3 = "321" //'"321"' (type string) cannot be represented by the type *string - &str1 is an address in memory like 0xc00004a280
	fmt.Printf("str1 = %s, str3 = %v\n", str1, str3)

	if funcs.SumTreeNumbers(defaultInt, 2, 3) < 7 {
		fmt.Println("funcs.SumTreeNumbers(defaultInt, 2, 3) is less than 7")
	}

	var maxInt8PlusOne = int8(maxInt + 1)
	fmt.Printf("\nmaxInt = %v", maxInt)
	fmt.Printf("\nmaxInt8PlusOne = %v", maxInt8PlusOne)
	//if true {
	//} // doesn't work = };
	//else {
	//}

	x := Sin(1.0)
	fmt.Printf("\nSin(1.0) %v", x)

	var complex1 complex128 = complex(1, 2) // 1 + 2i
	complex2 := 3 + 4i
	fmt.Println(complex1 + complex2)
	fmt.Println(complex1 - complex2)
	fmt.Println(complex1 * complex2)
	fmt.Println(real(complex1 * complex2))
	fmt.Println(imag(complex1 * complex2))
}
