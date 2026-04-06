package goleak

import (
	_ "os"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// Проверка утечек горутин после всех тестов
	goleak.VerifyTestMain(m)
}

func TestSomething(t *testing.T) {
	defer goleak.VerifyNone(t)

	go func() {
		select {} // Горшок, который никогда не завершится
	}()
}
