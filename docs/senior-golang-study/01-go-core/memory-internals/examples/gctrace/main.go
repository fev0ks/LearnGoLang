// Синтетическое демо для наблюдения за сборщиком мусора через GODEBUG=gctrace=1.
//
// Программа генерирует управляемую аллокационную нагрузку (мусор + опц. живой набор),
// чтобы было видно, как и когда срабатывает GC, и как на это влияют GOGC и GOMEMLIMIT.
//
// Запуск (нужен GODEBUG=gctrace=1 — печать строки на каждый цикл GC):
//
//	GODEBUG=gctrace=1 go run .
//	GODEBUG=gctrace=1 go run . -alloc=512 -rate=1ms     # больше мусора → чаще GC
//	GODEBUG=gctrace=1 go run . -live=200                # держать 200 MB живыми
//	GOGC=50  GODEBUG=gctrace=1 go run .                  # агрессивнее (GC чаще)
//	GOGC=400 GODEBUG=gctrace=1 go run .                  # реже GC, выше RSS
//	GOMEMLIMIT=128MiB GODEBUG=gctrace=1 go run . -live=80
//	GOGC=off GOMEMLIMIT=200MiB GODEBUG=gctrace=1 go run . -live=50
//
// Формат строки gctrace разобран в ../../04-garbage-collector.md (раздел «Диагностика»):
//
//	gc 14 @12.345s 1%: 0.02+2.3+0.14 ms clock, ... 20->22->11 MB, 23 MB goal, ... 8 P
//	   │    │       │                            │            │        └ цель следующего GC
//	   │    │       │                            │            └ heap после sweep (≈ live)
//	   │    │       │                            └ heap до GC → в начале sweep
//	   │    │       └ доля CPU на GC (следи, чтобы не росла)
//	   │    └ время с старта
//	   └ номер цикла
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"
)

func main() {
	allocKB := flag.Int("alloc", 256, "KB короткоживущего мусора за итерацию")
	rate := flag.Duration("rate", time.Millisecond, "пауза между итерациями (меньше → выше allocation rate)")
	liveMB := flag.Int("live", 0, "сколько MB удерживать ЖИВЫМИ (постоянный live heap)")
	dur := flag.Duration("dur", 10*time.Second, "сколько работать перед выходом")
	flag.Parse()

	// Живой набор: удерживаем -live MB ссылками, чтобы было видно «живой heap»
	// в gctrace (в тройке X->Y->Z значение Z ≈ live, т.к. это не мусор).
	live := make([][]byte, 0, *liveMB)
	for i := 0; i < *liveMB; i++ {
		chunk := make([]byte, 1<<20) // 1 MB
		chunk[0] = 1
		live = append(live, chunk)
	}

	fmt.Printf("GOGC=%s  GOMEMLIMIT=%s  alloc=%dKB  rate=%s  live=%dMB  dur=%s\n",
		envOr("GOGC", "100"), envOr("GOMEMLIMIT", "off (по умолчанию)"),
		*allocKB, *rate, *liveMB, *dur)
	fmt.Println("Запусти с GODEBUG=gctrace=1, чтобы видеть строки gc ... ниже.")
	fmt.Println()

	var ms runtime.MemStats
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	deadline := time.Now().Add(*dur)

	for time.Now().Before(deadline) {
		// Генерируем мусор: слайс escape'ит в heap и тут же «забывается» → garbage.
		garbage = make([]byte, *allocKB*1024)
		garbage[0] = 1
		time.Sleep(*rate)

		select {
		case <-tick.C:
			runtime.ReadMemStats(&ms)
			fmt.Printf("[app] NumGC=%d  HeapAlloc=%dMB  HeapSys=%dMB  GCCPUFraction=%.2f%%\n",
				ms.NumGC, ms.HeapAlloc>>20, ms.HeapSys>>20, ms.GCCPUFraction*100)
		default:
		}
	}

	runtime.KeepAlive(live) // не дать GC собрать живой набор раньше времени
	fmt.Println("done")
}

// garbage — глобальная переменная, чтобы компилятор не соптимизировал аллокацию;
// каждое присваивание делает предыдущий слайс мусором.
var garbage []byte

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
