package everything

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

var poolSize = 11
var pool = make(chan int, poolSize)

func TryChanPool() {
	useInit()
	//wg := sync.WaitGroup{}
	//for i := 0; i <= 100; i++ {
	//	wg.Add(1)
	//
	//	go func(number int){
	//		ln_defer wg.Done()
	//		//wt := rand.Intn(5)
	//		//fmt.Printf("%d wait %d \n",number, wt)
	//		//time.Sleep(time.Duration(wt)* time.Second)
	//		//time.Sleep(2 * time.Second)
	//		fmt.Printf("start work %d \n", number)
	//		doWork(number)
	//	}(i)
	//}
	//wg.Wait()
	//fmt.Println("finished")
}

func useInit() {
	wg := sync.WaitGroup{}
	for i := 1; i <= poolSize; i++ {
		fmt.Printf("init %d \n", i)
		pool <- i
	}
	fmt.Printf("pool initialized %d\n", len(pool))

	for i := 0; i <= 100; i++ {
		wg.Add(1)
		go func(number int) {
			defer wg.Done()
			fmt.Printf("start work %d \n", number)
			doWork(number)
		}(i)
		//time.Sleep(1 * time.Second)
	}
	wg.Wait()

	fmt.Printf("finished %d\n", len(pool))
}

func doWork(number int) {
	select {
	case i := <-pool:
		fmt.Printf("lock pool %d for %d\n", i, number)
		wt := rand.Intn(5)
		time.Sleep(time.Duration(wt) * time.Second)
		fmt.Printf("unlock pool %d for %d\n", i, number)
		pool <- i
		//case <-time.After(4 * time.Second):
		//	fmt.Printf("wait for %d \n", number)
		//	doWork(number)
	}
}
