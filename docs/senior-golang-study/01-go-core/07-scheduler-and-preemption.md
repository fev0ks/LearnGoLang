# Go Scheduler: GMP модель и preemption

Scheduler определяет, как горутины распределяются по OS threads и CPU. Senior-уровень — это не просто "есть горутины и они дешевые", а умение объяснить GMP механику, work stealing, preemption, поведение при syscall и почему GOMAXPROCS важен в контейнерах.

## Содержание

- [GMP модель](#gmp-модель)
- [Горутина: стек и состояния](#горутина-стек-и-состояния)
- [Очереди работы](#очереди-работы)
- [Work stealing](#work-stealing)
- [Preemption](#preemption)
- [Syscall: handoff механизм](#syscall-handoff-механизм)
- [Netpoller интеграция](#netpoller-интеграция)
- [GOMAXPROCS в контейнерах](#gomaxprocs-в-контейнерах)
- [Типовые ошибки](#типовые-ошибки)
- [Диагностика](#диагностика)
- [Interview-ready answer](#interview-ready-answer)

## GMP модель

Три сущности runtime scheduler:

```
G — Goroutine      стек + program counter + состояние; дешевая (~2 KB начальный стек)
M — Machine        OS thread; выполняет Go код; нужен P для запуска горутин
P — Processor      виртуальный процессор; держит локальную очередь горутин (LRQ)
```

Схема взаимодействия:

```
 P[0]       P[1]       P[2]       P[3]
  |          |          |          |
 M[0]       M[1]       M[2]      M[3]
  |          |          |          |
 G[x]       G[y]       G[z]      G[w]   ← выполняются прямо сейчас

         Global Run Queue
         [G5, G6, G7 ...]

         Idle M threads pool
```

Ключевые правила:
- число P = `GOMAXPROCS` (по умолчанию число логических CPU хоста);
- горутина выполняется только на M, у которого есть P;
- M может существовать без P — например, во время blocking syscall;
- M может быть больше P: при syscall M теряет P и появляется новый M.

## Горутина: стек и состояния

**Начальный размер стека: 2 KB** (с Go 1.4; до этого было 8 KB).

Стек горутины растет автоматически через **contiguous stack copy** (Go 1.3+):
- при переполнении runtime аллоцирует стек в 2× больше;
- копирует текущий стек;
- обновляет все указатели внутри;
- старый стек освобождается.

Максимум стека по умолчанию: **1 GB** на 64-bit системах.  
Для сравнения: OS thread stack — 1–8 MB и не растет автоматически.

```go
// Сравнение стоимости горутины и OS thread:
// goroutine:  ~2 KB стек + ~450 bytes struct = ~2.5 KB
// OS thread:  ~2–8 MB стек = 1000x дороже
// Именно поэтому 100k горутин нормально, 100k threads — нет
```

**Состояния горутины:**

| Состояние   | Что происходит |
|-------------|----------------|
| `Grunnable` | готова к выполнению, ждет в очереди P |
| `Grunning`  | выполняется на M |
| `Gwaiting`  | заблокирована (channel, mutex, timer, select) |
| `Gsyscall`  | в системном вызове; M и G существуют без P |
| `Gdead`     | завершена, struct можно переиспользовать |

## Очереди работы

Каждый P имеет:

- **`runnext`** — один слот с наивысшим приоритетом для "только что unblocked" горутины (например, goroutine, которую мы только что разблокировали через channel send);
- **LRQ (Local Run Queue)** — кольцевой буфер до **256 горутин**; P берет горутины с головы (FIFO).

Глобальная очередь `sched.runq`:
- неограниченная; пополняется когда LRQ полна или горутина создается без привязки к P;
- каждые **~61 такт** P обязательно проверяет глобальную очередь (anti-starvation).

```
P берет из:
1. runnext       (наивысший приоритет)
2. LRQ           (голова очереди)
3. global queue  (раз в 61 такт, даже если LRQ не пуста)
4. netpoll       (ready network I/O горутины)
5. steal from P  (если все выше пусто)
```

## Work stealing

Когда у P заканчивается работа (runnext, LRQ и global queue пусты):

1. Выбирает **случайный другой P**.
2. Крадет **половину** его LRQ — берет с **хвоста** (tail stealing).
3. Если никто не может отдать — паркует M.

```
До steal:   P[1].LRQ = [G1, G2, G3, G4]
После steal: P[0] получает [G3, G4]; P[1] остается с [G1, G2]
```

Почему с хвоста: P[1] берет горутины с головы (G1, G2), P[0] берет с хвоста (G4, G3). Это минимизирует конфликт на одни и те же элементы очереди.

## Preemption

### До Go 1.14: кооперативная preemption

Горутина могла быть вытеснена только в **safe points** — точках, где компилятор вставил проверку `morestack` (при входе в каждую функцию).

**Проблема**: tight CPU loop без function calls мог занимать P вечно:

```go
// До Go 1.14: этот код блокировал scheduler на весь время работы
go func() {
    for {
        i++ // нет вызовов функций, нет safe point
    }
}()
```

### Go 1.14+: async preemption через SIGURG

**sysmon** — отдельный OS thread, работает каждые 10–20 мс:
- если горутина на одном P выполняется > **10 мс**;
- sysmon отправляет **SIGURG** OS thread, на котором она работает;
- signal handler выставляет флаг preemption в stack guard;
- при ближайшей async-safe инструкции горутина вытесняется и уходит в runnable queue.

```go
// После Go 1.14: scheduler вытеснит горутину через ~10ms, даже без function calls
go func() {
    for {
        i++
    }
}()
```

Исключение: функции с `//go:nosplit` не могут быть async-preempted.

```go
//go:nosplit
func criticalSection() {
    // не будет preem-ted, но нельзя вызывать функции, которые растут стек
}
```

## Syscall: handoff механизм

Когда горутина делает **blocking syscall** (например, file I/O):

```
До syscall:
  P[2] → M[2] → G[z] (running)

Syscall начинается:
  M[2] уходит в syscall (без P)
  P[2] → idle pool или подхватывается M[3]

Syscall завершается:
  G[z] пытается взять любой свободный P
  если P нет → G[z] в global run queue (Grunnable)
```

Это называется **handoff**: P не ждет завершения syscall, а продолжает работу с другими горутинами.

Для **non-blocking syscall** (сетевой I/O через netpoller): горутина паркуется, M освобождает P и берет другую горутину — OS thread не блокируется.

## Netpoller интеграция

Сетевой I/O в Go работает через `epoll` (Linux) / `kqueue` (macOS):

```
conn.Read(buf):
  1. горутина регистрирует fd в netpoller
  2. паркуется (Gwaiting)
  3. M освобождается, берет другую горутину из LRQ

данные готовы (epoll event):
  4. sysmon или другой P вызывает netpoll(nonblocking)
  5. blocked G переходит в Grunnable → добавляется в LRQ
  6. при следующем шедулировании G продолжает выполнение
```

Именно поэтому 100k concurrent TCP connections в Go работают с 4–8 OS threads (при GOMAXPROCS=4–8). Goroutines паркуются, а не блокируют threads.

## GOMAXPROCS в контейнерах

По умолчанию `GOMAXPROCS = runtime.NumCPU()` — число логических CPU **хоста**, а не контейнера.

**Проблема**: контейнер с `cpu.limit = 0.5` на 64-ядерном хосте:
- Go создает 64 P и 64+ OS threads;
- CPU scheduler хоста throttle-ит их до 50%;
- высокий context switch overhead и starvation.

```go
// Вариант 1: библиотека automaxprocs
import _ "go.uber.org/automaxprocs"
// читает cgroup cpu.max при старте, устанавливает GOMAXPROCS правильно

// Вариант 2: вручную через переменную окружения
// GOMAXPROCS=2 ./myapp

// Вариант 3: из кода (редко нужен)
import "runtime"
runtime.GOMAXPROCS(runtime.NumCPU())
```

```yaml
# docker-compose: явное ограничение
resources:
  limits:
    cpus: '2.0'
# с automaxprocs: GOMAXPROCS будет установлен в 2
```

## Типовые ошибки

**Неограниченный fan-out:**
```go
// Плохо: создает горутину на каждый request
for _, item := range items {
    go process(item) // 10k requests = 10k goroutines одновременно
}

// Хорошо: bounded worker pool
sem := make(chan struct{}, 100)
for _, item := range items {
    sem <- struct{}{}
    go func(item Item) {
        defer func() { <-sem }()
        process(item)
    }(item)
}
```

**Goroutine leak:**
```go
// Плохо: горутина зависает навсегда
go func() {
    result := <-ch // если никто не пошлет в ch — утечка
    process(result)
}()

// Хорошо: всегда передавать context
go func(ctx context.Context) {
    select {
    case result := <-ch:
        process(result)
    case <-ctx.Done():
        return
    }
}(ctx)
```

**Завышенный GOMAXPROCS в контейнере** без automaxprocs — scheduler contention и CPU throttling.

## Диагностика

```bash
# Scheduler events трейс: выводит состояние каждые 1000ms
GODEBUG=schedtrace=1000 ./myapp

# Пример вывода:
# SCHED 1000ms: gomaxprocs=4 idleprocs=0 threads=6 spinningthreads=1
#               idlethreads=1 runqueue=2 [3 1 0 2]
#
# gomaxprocs=4    — число P
# threads=6       — OS threads (включая sysmon, netpoller)
# runqueue=2      — горутин в global queue
# [3 1 0 2]       — LRQ каждого P
```

```go
// Количество горутин прямо сейчас
n := runtime.NumGoroutine()

// goroutine pprof профиль — покажет stacktrace каждой горутины
// GET http://localhost:6060/debug/pprof/goroutine?debug=2

// Детальная трассировка scheduler (запись событий в файл)
import "runtime/trace"

f, _ := os.Create("trace.out")
trace.Start(f)
// ... работа сервиса ...
trace.Stop()
// go tool trace trace.out  — открывает в браузере
```

## Interview-ready answer

**"Объясни GMP модель Go scheduler"**

В Go scheduler три сущности: **G** (горутина — начальный стек 2 KB, намного дешевле OS thread), **M** (OS thread — выполняет Go код) и **P** (виртуальный процессор — держит локальную очередь горутин, LRQ до 256 штук). Количество P = GOMAXPROCS.

Когда P заканчивает работу, он применяет **work stealing**: крадет половину LRQ у другого P с хвоста. Когда горутина делает blocking syscall, M отцепляется от P — это **handoff**: P подхватывается другим M, и работа продолжается без блокировки. Сетевые I/O не блокируют M вообще: горутина паркуется в netpoller, M сразу берет другую горутину — поэтому 100k соединений работают без 100k threads.

**Preemption**: до Go 1.14 вытеснение было только в функциях (кооперативное). С 1.14 sysmon посылает SIGURG через 10ms, горутина вытесняется асинхронно — даже из tight loop.

**GOMAXPROCS в контейнерах**: по умолчанию читает CPU хоста, а не cpu.limit из cgroup. Нужен `automaxprocs` или явная установка, иначе scheduler создает лишние OS threads и получает CPU throttling.
