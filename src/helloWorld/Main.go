package main

import (
	"fmt"
	"helloWorld/funcs"
	"helloWorld/logging"
	_ "helloWorld/pkg/logging"
	aliasForLogging "helloWorld/pkg/logging"
	"helloWorld/types"
	. "math"
	"reflect"
	"time"
)

func main() {
	//funcs.ThrowPanic(12, 0)

	//testVariables()
	//testLogger()
	//testArray()
	//testLoops()
	//testTrash()
	testTypes()
	//testFunc()
	//multithreading.StartThreads(5)
	//multithreading.Chan()
	//multithreading.WaitGroup()
	//testSwitch(2)
	//testSwitch(1)
	//testSwitch(0)
	//testSwitch(99)

}

func testVariables() {
	var v1 int = 100
	fmt.Printf("\nv1 = %v ", v1)

	var v2 string = "Hello!"
	fmt.Printf("\nv2 = %v ", v2)

	var v3 = [10]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	fmt.Printf("\nv3 = %v ", v3)

	var v4 = []int{1000, 2000, 12334}
	fmt.Printf("\nv4 = %v ", v4)

	var v5 = struct{ f int }{50}
	fmt.Printf("\nv5 = %v ", v5.f)

	var v6 *int = &v1
	fmt.Printf("\nv6 = %v ", v6)

	var v7 = map[string]int{"one": 1, "two": 2, "three": 3}
	fmt.Printf("\nv7[one] = %v ", v7["one"])
	fmt.Printf("\nv7[zero] = %v ", v7["zero"])

	var v8 = func(a int) int { return a + 1 }
	fmt.Printf("\nv8(10) = %v ", v8(10))
}

func testFunc() {
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
	fmt.Printf("\nimSumFunc(3,5) = %v", imSumFunc(3, 5))
	funcs.ThrowParsePanic("123", "12.4")
	defer func() {
		if err != nil {
			fmt.Printf("\nerr is not nil PANIC!!!! %v", err)
			panic(err)
		}
	}()
}

func testArray() {
	var myNumbers [5]int
	fmt.Print("updated myNumber: ")
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

	for range slice {
		fmt.Printf("\nim in slice...")
	}

}

func testSwitch(value int) {
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

func testLoops() {

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

func testLogger() {
	//var newLogger = new (logging.Logger{new (logging.MyInterface), "123", true})
	//logging.Info = logging.Debug //so sad...
	var nilLogger *logging.Logger = nil
	var copyOfNilLogger logging.MyInterface = nilLogger

	if nilLogger == nil {
		fmt.Printf("nilLogger = %v\n", nilLogger)
	}
	//nilLogger.SetDebug(true) // invalid memory address or nil pointer dereference

	if copyOfNilLogger != nil {
		fmt.Printf("copyOfNilLogger copy of nil as Interface is not nil but nil = %v\n", copyOfNilLogger)
	}

	if reflect.ValueOf(copyOfNilLogger).IsNil() {
		fmt.Printf("reflect.ValueOf(copyOfNilLogger).IsNil copyOfNilLogger = %v\n", copyOfNilLogger)
	}

	var defaultLogger logging.Logger
	fmt.Printf("defaultLogger = %v\n", defaultLogger)
	defaultLogger.Log(logging.Info, "defaultLogger")

	fmt.Println()
	var bigNullLogger logging.BigLogger
	fmt.Printf("bigNullLogger = %v\n", bigNullLogger)
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

func testTypes() {
	cat := types.Animal{AnimalType: "cat", Name: "Snow", Weight: 7.5, Height: 40}
	fmt.Printf("cat: %v \n", cat)
	fmt.Printf("cat.String(): %v \n", cat.String())

	types.Print(cat)

	var nullCat types.Animal
	fmt.Printf("nullCat: %v \n", nullCat)

	dog := types.Dog{Animal: types.Animal{AnimalType: "dog", Name: "Sharik", Weight: 70.5, Height: 80}}
	fmt.Printf("dog: %v \n", dog)
	//types.Print(dog) //Type does not implement 'Stringer' as some methods are missing: String() string

	fmt.Printf("dog.GetType(): %v \n", dog.GetType())
	types.PrintAnimalType(dog)
	fmt.Printf("cat.GetType(): %v \n", cat.GetType())
	types.PrintAnimalType(cat)

	newCat := types.NewCat("Snow", 7.5, 40)
	fmt.Printf("newCat: %v \n", newCat)
	fmt.Printf("newCat.String(): %v \n", newCat.String())
	types.Print(newCat)

	newDog := types.NewDog("Sharik", 70.5, 80)
	fmt.Printf("newDog: %v \n", newDog)
	//types.Print(dog) //Type does not implement 'Stringer' as some methods are missing: String() string

	fmt.Printf("newDog.GetType(): %v \n", newDog.GetType())
	types.PrintAnimalType(newDog)
	//fmt.Printf("newCat.GetType(): %v \n", newCat.GetType()) //newCat is not Animal, (a Animal) GetType()
	//types.PrintAnimalType(newCat)
}

func testTrash() {
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

	fmt.Println("Hello world")
	fmt.Printf("defaultBool = %t\n", defaultBool)
	fmt.Printf("sum(%v, %v, %v) = %v\n", a, b, variable, funcs.SumTreeNumbers(a, b, variable))
	fmt.Printf("sum(%v, %v, %v) = %v\n", a, b, c, funcs.SumTreeNumbers(a, b, c))
	fmt.Printf("sum(%v, %v, %v) = %v\n", a, b, defaultInt, funcs.SumTreeNumbers(a, b, defaultInt))

	fmt.Printf("str1 = %s\n", str1)
	fmt.Printf("%v == %v = %v\n", num1, num2, num1 == num2)

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
	fmt.Printf("maxInt = %v\n", maxInt)
	fmt.Printf("maxInt8PlusOne = %v\n", maxInt8PlusOne)
	//if true {
	//} // doesn't work = };
	//else {
	//}

	x := Sin(1.0)
	fmt.Printf("Sin(1.0) %v \n", x)

	var complex1 complex128 = complex(1, 2) // 1 + 2i
	complex2 := 3 + 4i
	fmt.Println(complex1 + complex2)
	fmt.Println(complex1 - complex2)
	fmt.Println(complex1 * complex2)
	fmt.Println(real(complex1 * complex2))
	fmt.Println(imag(complex1 * complex2))
}
