package funcs

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

func Sum(a, b int) (int, string) {
	return a + b, "sum"
}

func IncTwo(a, b int) (c, d int, s string) {
	c = a + 1
	d = b + 1
	s = "incremented by 1"
	return
}

// CopyFile Функция, копирующая файл
func CopyFile(dstName, srcName string) (written int64, err error) {
	src, err := os.Open(srcName) // Открытие файла-источника
	if err != nil {              // Проверка
		return // Если неудача, возврат с ошибкой
	}
	// Если пришли сюда, то файл-источник был успешно открыт
	defer src.Close() // Отложенный вызов: src.Close() будет вызван по завершении CopyFile

	dst, err := os.Create(dstName) // Открытие файла-приёмника
	if err != nil {                // Проверка и возврат при ошибке
		return
	}
	defer dst.Close() // Отложенный вызов: dst.Close() будет вызван по завершении CopyFile

	return io.Copy(dst, src) // Копирование данных и возврат из функции
	// После всех операций будут вызваны: сначала dst.Close(), затем src.Close()
}

//func sum(a int, b int) int {
//	return a + b
//}

func SumTreeNumbers(a int, b int, c int) int {
	return a + b + c
}

func ReadFileName(srcName string) (result string, err error) {
	file, err := os.Open(srcName)
	if err != nil {
		// Генерация новой ошибки с уточняющим текстом
		return "", fmt.Errorf("Ошибка при чтении файла %s: %g\n", srcName, err)
	}
	// Дальнейшее исполнение функции, если ошибки не было
	return file.Name(), nil // Возврат результата и пустой ошибки, если выполнение успешно
}

func ThrowPanic(a, b int) {
	processErrorVar := processError
	defer processErrorVar()
	fmt.Fprintf(os.Stdout, "%d / %d = %d\n", a, b, a/b)
}

func ThrowParsePanic(a, b string) {
	processErrorVar := processError
	defer processErrorVar()
	value1, err := strconv.ParseInt(a, 10, 1)
	if err != nil {
		panic(err)
	}
	value2, err := strconv.ParseInt(b, 10, 32)
	if err != nil {
		panic(err)
	}
	ThrowPanic(int(value1), int(value2))
}

func processError() {
	err := recover()
	if v, ok := err.(error); ok { // Обработка паники, соответствующей интерфейсу error
		fmt.Fprintf(os.Stderr, "Error %v \"%s\"\n", err, v.Error())
	} else if err != nil {
		panic(err) // Обработка неожиданных ошибок - повторный вызов паники.
	}
}
