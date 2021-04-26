package multithreading

import (
	"fmt"
	"log"
	"os"
	"sync"
)

//***
//WaitGroup
//wg =  {{} [1 4 0]}
//wg =  {{} [1 4 0]}
//wg =  {{} [0 4 0]}
//prepareWord word = 1
//prepareWord word = 4
//prepareWord word = 2
//wg =  {{} [1 4 0]}
//prepareWord word = 3
//wg = {{} [0 0 0]}

func WaitGroup() {
	fmt.Printf("\n***\nWaitGroup\n")
	var args = []string{"1", "2", "3", "4"}
	var wg sync.WaitGroup               // Создание waitgroup. Исходное значение счётчика — 0
	logger := log.New(os.Stdout, "", 0) // log.Logger — потоково-безопасный тип для вывода
	for _, arg := range args {          // Цикл по всем аргументам командной строки
		wg.Add(1) // Увеличение счётчика waitgroup на единицу
		// Запуск go-процедуры для обработки параметра arg
		go func(word string) {
			logger.Println("wg = ", wg)
			// Отложенное уменьшение счётчика waitgroup на единицу.
			// Произойдёт по завершении функции.
			defer wg.Done()
			logger.Println(prepareWord(word)) // Выполнение обработки и вывод результата
		}(arg)
	}
	wg.Wait() // Ожидание, пока счётчик в waitgroup wg не станет равным нулю.
	fmt.Printf("wg = %v\n", wg)
}

func prepareWord(word string) interface{} {
	return fmt.Sprintf("prepareWord word = %s", word)
}
