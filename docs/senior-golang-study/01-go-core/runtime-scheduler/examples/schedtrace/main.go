// Синтетическое демо для наблюдения за планировщиком Go через GODEBUG=schedtrace.
//
// Программа поднимает три вида нагрузки, чтобы schedtrace показал разные явления:
//   - CPU-bound горутины  → заполняют LRQ каждого P, видны work stealing и preemption;
//   - I/O-bound горутины   → спят на тикере (Gwaiting), потребляют память, но не CPU;
//   - (опц.) spin-горутина → tight loop без точек уступки — демо async preemption (1.14+).
//
// Запуск (нужен GODEBUG=schedtrace=1000 — печать состояния планировщика раз в 1000 мс):
//
//	GODEBUG=schedtrace=1000 go run .
//	GODEBUG=schedtrace=1000,scheddetail=1 go run .   # подробно по каждому P/M/G
//	GOMAXPROCS=1 GODEBUG=schedtrace=1000 go run .     # один P — видно чередование
//	GODEBUG=schedtrace=1000 go run . -spin            # добавить tight loop
//	go run . -cpu=16 -io=5000 -dur=15s
//
// Что искать в строке `SCHED 1000ms: gomaxprocs=.. idleprocs=.. threads=.. spinningthreads=.. runqueue=.. [..]`:
//   - runqueue=N        — горутин в ГЛОБАЛЬНОЙ очереди;
//   - [3 1 0 2]         — длины LRQ каждого P (дисбаланс → сейчас будет work stealing);
//   - idleprocs         — простаивающие P (под нагрузкой должно быть 0);
//   - threads           — всего OS-потоков (включая sysmon);
//   - spinningthreads   — потоки, активно ищущие работу.
package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
	"time"
)

func main() {
	cpu := flag.Int("cpu", 8, "число CPU-bound горутин (грузят P, видны в LRQ)")
	io := flag.Int("io", 1000, "число I/O-bound горутин (спят → Gwaiting, не на CPU)")
	spin := flag.Bool("spin", false, "запустить tight-loop горутину (демо async preemption)")
	dur := flag.Duration("dur", 10*time.Second, "сколько работать перед выходом")
	flag.Parse()

	fmt.Printf("GOMAXPROCS=%d  NumCPU=%d  cpu=%d  io=%d  spin=%v  dur=%s\n",
		runtime.GOMAXPROCS(0), runtime.NumCPU(), *cpu, *io, *spin, *dur)
	fmt.Println("Запусти с GODEBUG=schedtrace=1000, чтобы видеть строки SCHED ... ниже.")
	fmt.Println()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// CPU-bound: бесконечно считают порциями. Между порциями есть вызов функции
	// (точка уступки) и проверка stop. Заполняют очереди P → видно work stealing.
	for i := 0; i < *cpu; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				burnCPU() // порция работы (~миллионы итераций)
			}
		}()
	}

	// I/O-bound: просыпаются по тикеру, чуть работают, снова паркуются (Gwaiting).
	// Тысячи таких горутин почти не грузят CPU — заметно по schedtrace (LRQ короткие).
	for i := 0; i < *io; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(50 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					// проснулись, мини-работа и обратно в сон
				}
			}
		}()
	}

	// Spin: tight loop БЕЗ вызовов функций и блокировок. До Go 1.14 занял бы P навсегда;
	// с 1.14 sysmon вытесняет его через SIGURG. НЕ в WaitGroup — завершится с процессом.
	if *spin {
		go func() {
			x := 0
			for {
				x++ // нет точек уступки — только async preemption спасает остальных
			}
		}()
	}

	// Раз в секунду печатаем число «обычных» горутин (без системных).
	go func() {
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				fmt.Printf("[app] NumGoroutine=%d\n", runtime.NumGoroutine())
			}
		}
	}()

	time.Sleep(*dur)
	close(stop)
	wg.Wait()
	fmt.Println("done")
}

// burnCPU — порция чистой CPU-работы. Вынесена в функцию, чтобы был safe point
// на входе (кооперативная точка уступки), и компилятор не выкинул цикл.
//
//go:noinline
func burnCPU() {
	var x float64
	for j := 0; j < 5_000_000; j++ {
		x += float64(j) * 1.0000001
	}
	sink = x
}

// sink не даёт компилятору выбросить вычисление как мёртвый код.
var sink float64
