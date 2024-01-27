package everything

import (
	"fmt"
	"sync"
)

var (
	mu = &sync.Mutex{}
)

func TryThreads() {
	list := make(map[string]string, 0)
	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				mu.Lock()
				list[fmt.Sprintf("%d - %d", n, j)] = fmt.Sprintf("%d - %d", n, j)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	list2 := make(map[string]string, 0)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				mu.Lock()
				list2[fmt.Sprintf("%d - %d", n, j)] = fmt.Sprintf("%d - %d", n, j)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	//for _, value := range list {
	//	fmt.Println(value)
	//}
	fmt.Println(len(list), "-", len(list2))
}
