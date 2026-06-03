# Netpoller: сетевой I/O без blocking threads

Netpoller — слой в Go runtime, который позволяет тысячам горутин ждать сетевых событий, не блокируя OS threads. Понимание netpoller объясняет, почему Go эффективен для высоконагруженных сетевых сервисов.

> **Что такое fd (file descriptor)?** Это просто **целое число** — «ручка», которую ядро выдаёт процессу на открытый ресурс: сокет, файл, pipe и т.п. В Unix «всё есть файл», поэтому соединение, файл и канал — всё это fd (например, `3`, `4`, `5`…). Когда ты пишешь `conn.Read`, внутри это `read(fd, …)` — операция над числом-дескриптором. Само ядро по этому числу находит свою структуру (буферы, состояние TCP и пр.). `socket()`/`accept()`/`open()` возвращают новый fd; `close()` его освобождает. fd уникальны **в пределах процесса** и выдаются по возрастанию (0/1/2 заняты под stdin/stdout/stderr).

## Содержание

- [Проблема: blocking I/O vs threads](#проблема-blocking-io-vs-threads)
- [Решение: epoll/kqueue/IOCP](#решение-epollkqueueiocp)
- [Архитектура netpoller в Go](#архитектура-netpoller-в-go)
- [Куда приходят данные, пока горутина спит](#куда-приходят-данные-пока-горутина-спит)
- [Жизненный цикл сетевого соединения](#жизненный-цикл-сетевого-соединения)
- [pollDesc: связь fd и горутины](#polldesc-связь-fd-и-горутины)
- [Когда netpoll() вызывается](#когда-netpoll-вызывается)
- [Deadlines: SetDeadline через таймеры](#deadlines-setdeadline-через-таймеры)
- [DNS resolver: Go vs platform](#dns-resolver-go-vs-platform)
- [Типичные паттерны и ошибки](#типичные-паттерны-и-ошибки)
- [Диагностика](#диагностика)
- [Interview-ready answer](#interview-ready-answer)

---

## Проблема: blocking I/O vs threads

**Наивный подход: один поток на соединение**

```
10 000 соединений
= 10 000 OS threads
= ~80 GB стека (8 MB × 10 000)
= огромный context switch overhead
```

**Классический async подход: event loop**

```
1 поток + event loop (Node.js, nginx worker)
= O(1) memory
= но: всё в одном потоке, CPU-bound блокирует всех
```

**Go подход: goroutine per connection + netpoller**

```
10 000 соединений
= 10 000 горутин × 2–8 KB стека = 20–80 MB
= 4–8 OS threads (GOMAXPROCS)
= код пишется синхронно, выполняется асинхронно
```

Горутины пишут `conn.Read()` как блокирующий вызов, но под капотом горутина **паркуется** и OS thread освобождается для других горутин.

---

## Решение: epoll/kqueue/IOCP

Go использует платформенный механизм асинхронного уведомления о готовности I/O:

| Платформа | Механизм | Сложность |
|---|---|---|
| Linux | `epoll` | O(1) на событие, O(log n) на добавление |
| macOS/BSD | `kqueue` | аналогично epoll |
| Windows | `IOCP` (I/O Completion Ports) | completion model, не readiness |
| Solaris | `event ports` | |

**epoll (Linux) — как работает:**

```c
// Создать epoll instance
epfd = epoll_create1(0);

// Зарегистрировать fd для мониторинга
epoll_ctl(epfd, EPOLL_CTL_ADD, sockfd, &event);

// Ждать событий (блокирующий вызов на отдельном потоке)
n = epoll_wait(epfd, events, MAX_EVENTS, timeout_ms);
// events[0..n-1] содержат готовые fd
```

**Ключевое преимущество epoll**: возвращает только **готовые** fd, не перебирает все. При 10 000 соединений и 100 активных — epoll_wait вернёт 100, а не перебирает 10 000.

---

## Архитектура netpoller в Go

```
net.Conn.Read(buf)
    ↓
internal/poll.FD.Read()
    ↓
poll.FD.readLock() → syscall.Read() (O_NONBLOCK)
    ↓ если EAGAIN (данных нет)
pollDesc.waitRead()
    ↓
gopark() — горутина паркуется (Gwaiting), M освобождается
    ↓
[данные появились в ядре]
netpoll(delay) → возвращает список готовых горутин
    ↓
горутина → Grunnable → LRQ любого P
    ↓
горутина продолжает, syscall.Read() возвращает данные
```

**Ключевые компоненты:**

```
runtime.pollDesc    — один на каждый fd, хранит заблокированные горутины
runtime.netpollGenericInit() — инициализация epoll fd при старте
runtime.netpoll(delay) — вызов epoll_wait, возврат готовых горутин
runtime.netpollready() — разбудить горутину для конкретного fd
```

---

## Куда приходят данные, пока горутина спит

Частый вопрос: если горутина, ждущая `conn.Read`, **снята с потока и не исполняется**, то куда вообще приходят данные? Ответ: **приём данных делает ядро, а не горутина** — асинхронно, независимо от того, исполняется ли сейчас твоя горутина (и даже твой процесс).

Когда `read()` вернул `EAGAIN` (данных нет), горутина паркуется, а её fd зарегистрирован в epoll. Дальше всё происходит **в ядре**:

```
1. Пакет приходит на сетевую карту (NIC — Network Interface Card)
   → NIC по DMA (Direct Memory Access — запись в RAM напрямую,
     минуя CPU) кладёт пакет в буферы ядра → аппаратное прерывание

2. Ядро (в контексте прерывания/softirq, БЕЗ участия твоего кода):
   → сетевой стек: драйвер → IP → TCP (проверка, сборка, отправка ACK)
   → копирует payload в RECEIVE-БУФЕР СОКЕТА (память ЯДРА, привязана к fd)
   → помечает fd как "readable" и кладёт его в ready-list epoll-инстанса

3. Данные лежат в socket buffer ядра, fd помечен готовым.
   Горутина всё ещё СПИТ — для приёма она не нужна.

4. Go-рантайм вызывает netpoll() = epoll_wait()
   → ядро отдаёт готовые fd → рантайм по fd находит pollDesc
   → припаркованную горутину → Grunnable

5. Горутина просыпается, ПОВТОРЯЕТ read():
   → копирует байты из socket buffer ЯДРА → в твой buf (user-space)
```

Горутина нужна **только на шаге 5** — скопировать уже пришедшие данные. Само ожидание и приём её не занимают: данные хранит ядро (в socket buffer), готовность запоминает epoll (тоже в ядре).

### Два разных буфера (частая путаница)

| Буфер | Где живёт | Кто пишет | Кто читает |
|---|---|---|---|
| **socket receive buffer** | память **ядра**, на каждый fd (размер ~ `SO_RCVBUF`) | ядро при приёме пакетов | `read()` при вызове |
| **user buffer** (`buf` в `conn.Read(buf)`) | память **процесса** (стек/heap горутины) | `read()` копирует сюда | твой код |

`read()` — это операция «перелей из буфера ядра в мой буфер». Пусто в буфере ядра → `EAGAIN` → паркуемся; есть данные → копируем и возвращаем `n`.

> Аналогия: ты заказал посылку и ушёл. Курьер (ядро) принимает её и кладёт в почтовый ящик (socket buffer) — стоять у двери не нужно. Ты (горутина) подходишь к ящику (`read`), только когда удобно, и забираешь доставленное. epoll — это «лампочка на ящике: есть ли что забрать».

Именно поэтому асинхронный I/O масштабируется: при **блокирующем** чтении поток сидел бы внутри `read()` в ядре, ожидая данные — один поток на соединение. В Go ожиданием и приёмом занимается ядро, а горутина спит «бесплатно» (запись в pollDesc + ~2 KB стека) и просыпается лишь скопировать готовое.

---

## Жизненный цикл сетевого соединения

### Создание сервера

```go
ln, err := net.Listen("tcp", ":8080")
// 1. socket(AF_INET6, SOCK_STREAM, 0)    — создать fd
// 2. setsockopt(fd, SO_REUSEADDR, ...)   — опции
// 3. bind(fd, :8080)
// 4. listen(fd, backlog)
// 5. setNonblock(fd)                     — O_NONBLOCK
// 6. epoll_ctl(epfd, ADD, fd, EPOLLIN)   — зарегистрировать в epoll
```

### Accept нового соединения

```go
conn, err := ln.Accept()
// 1. Горутина вызывает accept()
// 2. Если нет соединений → EAGAIN → parkGoRoutine
// 3. epoll уведомляет: новое соединение готово
// 4. Горутина разбужена → accept() снова → получает conn fd
// 5. conn fd выставляется в O_NONBLOCK
// 6. epoll_ctl(epfd, ADD, connfd, EPOLLIN|EPOLLOUT)
```

### Read данных

```go
n, err := conn.Read(buf)
// 1. syscall.Read(fd, buf) — O_NONBLOCK
// 2. Если EAGAIN → pollDesc.waitRead()
//    → gopark(netpollblockcommit)
//    → горутина Gwaiting, M берёт другую горутину
// 3. Данные пришли: epoll_wait → EPOLLIN на connfd
//    → netpollready(pd, 'r') → горутина → Grunnable
// 4. Горутина продолжает, Read возвращает данные

// Write аналогично через EPOLLOUT
```

### Close соединения

```go
conn.Close()
// 1. epoll_ctl(epfd, DEL, fd, 0) — убрать из epoll
// 2. close(fd)
// 3. pollDesc помечается как closed
// 4. Если горутина паркована на этом fd — она разбуждается с ошибкой ErrNetClosing
```

---

## pollDesc: связь fd и горутины

Каждый сетевой fd имеет связанный `pollDesc`:

```go
// runtime/netpoll.go (упрощённо)
type pollDesc struct {
    link *pollDesc  // free list

    fd      uintptr // файловый дескриптор
    closing bool

    rg  atomic.Uintptr  // goroutine waiting for read, or flags
    wg  atomic.Uintptr  // goroutine waiting for write, or flags
    rd  int64           // read deadline (unix nano)
    wd  int64           // write deadline (unix nano)
    rt  timer           // read deadline timer
    wt  timer           // write deadline timer
}
```

Горутина, ждущая `Read`, хранится в `rg`. Горутина, ждущая `Write`, в `wg`. По одной горутине на каждое направление на каждый fd.

**Что это значит на практике:** нельзя делать concurrent `Read` из нескольких горутин на один `conn` — второй `Read` заменит `rg` первого, и первая горутина потеряет уведомление. Все официальные клиенты (HTTP, etc.) это учитывают.

---

## Когда netpoll() вызывается

`netpoll(delay)` вызывается из нескольких мест в scheduler:

```
1. findRunnable() — каждый раз когда P ищет новую горутину
   если LRQ, global queue, work steal все пусты → netpoll(0) (non-blocking)

2. sysmon — периодически
   netpoll(delay) с timeout → проверить готовые fd

3. startTheWorld (после GC STW) — разбудить всех waiting goroutines

4. Явный вызов runtime.Gosched() → может триггернуть netpoll
```

**Важно**: netpoll не крутится в выделенном потоке. Он вызывается как часть scheduler loop:

```
findRunnable():
  1. runnext
  2. LRQ
  3. global queue (1 раз в 61 тик)
  4. netpoll(0)   ← проверить готовые сетевые события
  5. work steal
  6. netpoll(-1)  ← если нечего делать — ждать с timeout
```

### Зачем netpoll вызывают из ДВУХ мест (findRunnable и sysmon)

Это частый источник непонимания. Путь через `findRunnable` работает, **только когда хотя бы один P простаивает** и заходит искать новую горутину. А если **все P заняты** — крутят CPU-bound горутины и не заглядывают за новой работой?

Тогда netpoll из планировщика **никто не вызовет**. В это время по сети пришли данные, fd готовы, горутины, ждавшие `conn.Read`, уже могут продолжить — но **сидят незамеченными**: их некому перевести в `Grunnable`, потому что ни один P не «оглядывается».

Вот для этого и нужен **`sysmon`** — он работает **независимо, без P**, и периодически проверяет, давно ли вызывался netpoll (по таймстампу `sched.lastpoll`). Если давно — вызывает netpoll сам, забирает готовые I/O-горутины и кладёт в очередь.

```
Все P простаивают  → netpoll зовётся из findRunnable          ✓ (основной путь)
Все P заняты CPU    → findRunnable никто не зовёт
                    → netpoll сам по себе не сработал бы
                    → sysmon периодически зовёт netpoll        ✓ (страховка)
```

Итог: **`findRunnable` проверяет готовность сетевых fd (вызывает netpoll), когда планировщик и так ищет работу; `sysmon` — страховка на случай, когда все P заняты вычислениями и искать работу некому.** Поэтому сетевые горутины не «висят» даже под полной CPU-нагрузкой.

> Уточнение по словам: netpoll (`epoll_wait`) **не «опрашивает сеть»** — данные ядро уже приняло в socket buffer. netpoll лишь спрашивает у ядра, **какие fd уже помечены готовыми** (проверяет ready-list epoll — те самые «флажки на почтовых ящиках»).

---

## Deadlines: SetDeadline через таймеры

`conn.SetDeadline(t)` не делает syscall. Это чисто Go runtime механизм через timer heap:

```go
conn.SetDeadline(time.Now().Add(5 * time.Second))
// Устанавливает pollDesc.rd = deadline unix nano
// runtime timer: через 5s вызвать netpollDeadline(pd, 'r')
// netpollDeadline → pollDesc пометить как expired
//                → разбудить горутину с ошибкой timeout
```

**Что происходит при истечении дедлайна:**

```go
n, err := conn.Read(buf)
// err = &net.OpError{Err: poll.ErrDeadlineExceeded}
// errors.Is(err, os.ErrDeadlineExceeded) → true
```

**SetDeadline vs SetReadDeadline:**

```go
// SetDeadline — для обоих направлений
conn.SetDeadline(time.Now().Add(30 * time.Second))

// SetReadDeadline — только для Read
conn.SetReadDeadline(time.Now().Add(10 * time.Second))

// SetWriteDeadline — только для Write
conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

// Сбросить дедлайн (без ограничения)
conn.SetDeadline(time.Time{})
```

**Паттерн для idle connections (keep-alive servers):**

```go
func handleConn(conn net.Conn) {
    defer conn.Close()
    buf := make([]byte, 4096)

    for {
        // Сдвигать дедлайн на каждой итерации
        conn.SetDeadline(time.Now().Add(30 * time.Second))

        n, err := conn.Read(buf)
        if err != nil {
            if errors.Is(err, os.ErrDeadlineExceeded) {
                // Клиент молчал 30s — закрыть
                return
            }
            return
        }
        handleRequest(buf[:n])
    }
}
```

---

## DNS resolver: Go vs platform

DNS — частая неочевидная точка, где Go может использовать либо netpoller, либо blocking syscalls.

```go
// По умолчанию Go выбирает сам (зависит от платформы и конфигурации):
addr, err := net.LookupHost("example.com")
```

**Pure Go resolver** (default на Linux при CGO_ENABLED=0):
- Читает `/etc/resolv.conf`, `/etc/hosts`
- Открывает UDP сокет, ставит в O_NONBLOCK
- DNS запрос через netpoller → горутина паркуется, M свободен
- Хорошо масштабируется

**CGo resolver** (default на Linux при CGO_ENABLED=1, всегда на macOS):
- Вызывает `getaddrinfo()` из libc через CGo
- Blocking CGo call → M заблокирован на время DNS запроса
- При 1000 concurrent DNS lookup → 1000 OS threads

```bash
# Принудить Go resolver
export GODEBUG=netdns=go

# Принудить CGo resolver  
export GODEBUG=netdns=cgo

# Посмотреть какой используется
export GODEBUG=netdns=go+1  # +1 добавляет логирование
```

**Для production**: если много concurrent DNS запросов — использовать `GODEBUG=netdns=go` или DNS caching прокси (CoreDNS с caching).

---

## Типичные паттерны и ошибки

### Правильный timeout на каждый запрос

```go
// Плохо: нет timeout → горутина висит вечно при зависшем клиенте
func handle(conn net.Conn) {
    buf := make([]byte, 4096)
    conn.Read(buf)  // висим если клиент не пишет
}

// Хорошо: дедлайн на операцию
func handle(conn net.Conn) {
    conn.SetReadDeadline(time.Now().Add(10 * time.Second))
    buf := make([]byte, 4096)
    n, err := conn.Read(buf)
    if errors.Is(err, os.ErrDeadlineExceeded) {
        // клиент молчал 10s → закрыть
    }
}
```

### Concurrent Read/Write на одном conn

```go
// Можно: Read и Write из разных горутин (pollDesc.rg и wg независимы)
go func() { conn.Read(buf) }()
go func() { conn.Write(data) }()

// Нельзя: concurrent Read из двух горутин
go func() { conn.Read(buf1) }()  // записывает pollDesc.rg
go func() { conn.Read(buf2) }()  // перезапишет pollDesc.rg → первый теряет уведомление
```

### Накопление горутин при отсутствии дедлайнов

```go
// При 10k соединений без дедлайна — 10k горутин в Gwaiting
// Они занимают память (~2KB минимум каждая) но не CPU
// Проверить: runtime.NumGoroutine() в метриках
// Смотреть: /debug/pprof/goroutine?debug=2
```

### http.Server и автоматические дедлайны

```go
server := &http.Server{
    Addr:         ":8080",
    Handler:      mux,
    ReadTimeout:  5 * time.Second,   // time to read request headers + body
    WriteTimeout: 10 * time.Second,  // time to write response
    IdleTimeout:  120 * time.Second, // keep-alive idle time
}
// http.Server устанавливает conn.SetDeadline автоматически
// Без этих значений — потенциальные goroutine leaks при slow clients
```

---

## Диагностика

```bash
# Goroutine dump — посмотреть где горутины заблокированы
curl http://localhost:6060/debug/pprof/goroutine?debug=2 | head -100

# Типичный вид заблокированной в netpoller горутины:
# goroutine 42 [IO wait]:
# internal/poll.runtime_pollWait(0xc000134000, 0x72)
#     /usr/local/go/src/runtime/netpoll.go:351 +0x85
# internal/poll.(*pollDesc).waitRead(...)
# net.(*conn).Read(0xc000124120, ...)

# Количество горутин в метриках
runtime.NumGoroutine()  // общее число
// Экспортировать в Prometheus:
// go_goroutines gauge

# netstat для диагностики соединений
ss -s           # сводка по TCP состояниям
ss -tn | wc -l  # количество TCP соединений
ss -tn state ESTABLISHED | wc -l

# TIME_WAIT — нормально при большом трафике
ss -tn state TIME-WAIT | wc -l

# CLOSE_WAIT — всегда баг (приложение не закрыло соединение)
ss -tn state CLOSE-WAIT | wc -l
```

```go
// Кастомный net.Listener для метрик
type instrumentedListener struct {
    net.Listener
    accepts prometheus.Counter
    active  prometheus.Gauge
}

func (l *instrumentedListener) Accept() (net.Conn, error) {
    conn, err := l.Listener.Accept()
    if err == nil {
        l.accepts.Inc()
        l.active.Inc()
    }
    return &instrumentedConn{conn, l.active}, err
}
```

---

## Interview-ready answer

**"Как Go обрабатывает 100k concurrent TCP соединений без 100k threads?"**

Go использует **netpoller** — abstraction поверх `epoll` (Linux) / `kqueue` (macOS). Все сетевые fd открываются в O_NONBLOCK режиме и регистрируются в epoll instance при создании.

Когда горутина вызывает `conn.Read()` и данных нет — syscall возвращает EAGAIN. Горутина **паркуется** (`gopark` → состояние Gwaiting), OS thread освобождается и берёт другую горутину из очереди.

Когда данные приходят — epoll уведомляет Go runtime через `netpoll()`. Это не выделенный поток: `netpoll(0)` вызывается из `findRunnable()` — каждый раз когда P ищет работу. Готовые горутины переходят в Grunnable и попадают в run queue.

Именно поэтому 100k соединений работают с 4–8 OS threads: горутины в состоянии Gwaiting потребляют только память (~2KB стека), а не CPU и thread.

**`SetDeadline`** работает через runtime timer heap, без дополнительных syscalls. При истечении таймера горутина разбуждается с `os.ErrDeadlineExceeded`.

**Важная деталь:** только сетевой I/O идёт через netpoller. Файловый I/O на Linux — blocking syscall, M блокируется, P отдаётся через scheduler handoff механизм.
