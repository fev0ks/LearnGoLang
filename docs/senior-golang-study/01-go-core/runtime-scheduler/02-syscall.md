# Syscall: механика и scheduler handoff

Системный вызов — переход из user space в kernel space. В Go это нетривиально: scheduler должен не заблокировать P пока M ждёт ответа от OS-ядра. Понимание этого механизма объясняет почему Go масштабируется до тысяч concurrent I/O операций без тысяч OS threads.

## Содержание

- [Blocking vs non-blocking: OS уровень](#blocking-vs-non-blocking-os-уровень)
- [Примеры на Go: что блокирует M, а что нет](#примеры-на-go-что-блокирует-m-а-что-нет)
- [Go syscall path: entersyscall → exitsyscall](#go-syscall-path-entersyscall--exitsyscall)
- [P handoff: три сценария](#p-handoff-три-сценария)
- [sysmon retake: когда забирают P](#sysmon-retake-когда-забирают-p)
- [exitsyscall: быстрый и медленный пути](#exitsyscall-быстрый-и-медленный-пути)
- [syscall.Syscall vs syscall.RawSyscall](#syscallsyscall-vs-syscallrawsyscall)
- [CGo: почему дороже обычного syscall](#cgo-почему-дороже-обычного-syscall)
- [runtime.LockOSThread](#runtimelockosthread)
- [Thread exhaustion: много blocking syscalls](#thread-exhaustion-много-blocking-syscalls)
- [Interview-ready answer](#interview-ready-answer)

---

## Blocking vs non-blocking: OS уровень

С точки зрения OS все системные вызовы делятся на:

**Blocking syscall** — OS thread заблокирован до завершения:
```
read(fd, buf, n)     — ждёт данных
accept(sockfd, ...)  — ждёт соединения
open(path, ...)      — может ждать на NFS
futex(WAIT, ...)     — ждёт mutex
```

**Non-blocking syscall** — возвращает немедленно (с EAGAIN если данных нет):
```
read(fd, ...)  с O_NONBLOCK — вернёт EAGAIN если нет данных
epoll_wait(epfd, ..., timeout=0) — не ждёт, только polling
getpid(), gettime() — всегда мгновенные
```

Go использует **оба** вида по-разному:
- Сетевые сокеты выставляются в O_NONBLOCK → паркует горутину через netpoller
- Файловый I/O, blocking system calls → уходят через blocking syscall путь

---

## Примеры на Go: что блокирует M, а что нет

Один и тот же по виду код (`Read`) ведёт себя по-разному в зависимости от того, что за дескриптор.

### 🔴 Блокируют M (идут через blocking syscall path)

M заходит в OS-ядро и ждёт там; P через handoff уходит другому M. Тысячи таких параллельно → тысячи потоков (см. [Thread exhaustion](#thread-exhaustion-много-blocking-syscalls)).

```go
// 1. Чтение ОБЫЧНОГО файла. На Linux у regular-файлов нет O_NONBLOCK —
//    read() блокирует поток в OS-ядре до конца чтения (диск/страничный кэш).
data, _ := os.ReadFile("/etc/hosts")
f, _ := os.Open("big.log"); f.Read(buf) // то же самое

// 2. Запуск внешнего процесса — ждём его через wait4(), M блокируется.
out, _ := exec.Command("sleep", "1").Output()

// 3. CGo-вызов в блокирующую C-функцию (напр. getaddrinfo в cgo-резолвере DNS).
//    M «теряет» scheduler context на всё время C-кода.
addrs, _ := net.LookupHost("example.com") // при CGO_ENABLED=1 — через cgo
```

### 🟢 НЕ блокируют M (через netpoller)

Сокет в `O_NONBLOCK`: `read()` сразу вернёт `EAGAIN`, горутина паркуется в netpoller (`Gwaiting`), а M **берёт другую работу**. Поэтому 100k соединений живут на горстке потоков.

```go
// Сетевой I/O — НЕ держит поток: парковка через netpoller.
conn, _ := net.Dial("tcp", "example.com:80")
buf := make([]byte, 4096)
conn.Read(buf)   // нет данных → EAGAIN → горутина паркуется, M свободен
ln, _ := net.Listen("tcp", ":8080")
ln.Accept()      // нет соединений → парковка, M свободен
```

### ⚪ Вообще не syscall (vDSO / user-space)

```go
_ = time.Now()   // clock_gettime через vDSO — без перехода в OS-ядро, не syscall
```

**Что такое vDSO.** *vDSO* (virtual Dynamic Shared Object) — это маленькая «виртуальная» разделяемая библиотека, которую **OS-ядро отображает в адресное пространство каждого процесса** при запуске. В неё OS-ядро кладёт код и данные для нескольких «лёгких» вызовов, которым на самом деле не нужны привилегии OS-ядра — прежде всего часы (`clock_gettime`, `gettimeofday`).

Зачем это нужно: обычный syscall дорог из-за **mode switch** (переход user → kernel: сохранить регистры, поднять привилегии, сменить стек, вернуться обратно — сотни нс). Но «сколько сейчас времени» — это просто **чтение значения**, которое OS-ядро и так регулярно обновляет в общей странице. Привилегии для этого не требуются. Поэтому:

- OS-ядро держит текущее время (и данные источника тактов) в **странице, доступной процессу на чтение**;
- код `clock_gettime` из vDSO выполняется **прямо в user-space** — читает эту страницу и считает результат;
- **перехода в OS-ядро (mode switch) нет вообще** → это не syscall, а почти как обычный вызов функции (десятки нс).

```
обычный syscall:  user → [mode switch] → kernel-код → [mode switch] → user   (~сотни нс)
vDSO (time.Now):  user → код из vDSO читает страницу с часами → user         (~десятки нс)
                          └─ в OS-ядро НЕ заходим
```

Именно поэтому `time.Now()` не оборачивается в `entersyscall`/`exitsyscall` и не влияет на планировщик — для него это не системный вызов.

> Нюанс: в редких конфигурациях (источник тактов не поддерживает vDSO, некоторые гипервизоры) `clock_gettime` **откатывается на настоящий syscall** — тогда это обычный неблокирующий syscall с заходом в OS-ядро. Но на обычном Linux/amd64/arm64 — vDSO.

### Как это увидеть

Запусти по 2000 горутин на каждый случай и сравни число потоков (`Threads` в `/proc/self/status` на Linux, или `pprof.Lookup("threadcreate").Count()`):

```go
// файловый I/O в 2000 горутин → пик потоков высокий (каждый blocking read держит M)
for i := 0; i < 2000; i++ {
    go func() { _, _ = os.ReadFile("/etc/hosts") }()
}

// сетевой I/O в 2000 горутин → потоков всё равно ~GOMAXPROCS+чуть (все паркуются в netpoller)
```

| Операция Go | Под капотом | M блокируется? |
|---|---|---|
| `os.ReadFile`, `f.Read` (обычный файл) | blocking `read()` | **да** |
| `exec.Command(...).Run()` | `wait4()` | **да** |
| cgo-вызов / cgo DNS | C-функция | **да** |
| `conn.Read/Write`, `ln.Accept` | `read()` O_NONBLOCK + netpoller | нет (park) |
| `time.Sleep`, таймеры | per-P timer heap (см. [04-timers](./04-timers.md)) | нет (park) |
| `time.Now()` | `clock_gettime` через vDSO | нет (не syscall) |

---

## Go syscall path: entersyscall → exitsyscall

Каждый blocking syscall в Go оборачивается в `entersyscall` / `exitsyscall`:

```
goroutine calls read(fd, ...)
        ↓
runtime.entersyscall()
  • сохранить SP, PC горутины (для GC stack scan)
    SP = Stack Pointer (вершина стека), PC = Program Counter (адрес инструкции)
  • выставить G.status = Gsyscall
  • отвязать P от M (P → idle или подхватывается другим M)
        ↓
SYSCALL (OS-ядро выполняет read)
  • M заблокирован в OS-ядре
  • P уже не привязан к этому M
  • GC может сканировать стек ЭТОЙ горутины (она в Gsyscall, Go-код не
    исполняет → стек заморожен; SP/PC сохранены). Syscall = safe point,
    GC не ждёт такую горутину. P.status = Psyscall.
        ↓
runtime.exitsyscall()
  • попытаться взять P обратно (fast path)
  • если не вышло → runnable queue (slow path)
```

Ключевое: в момент нахождения в syscall M и G **существуют без P**. P может быть подхвачен другим M из idle pool, который продолжит выполнять другие горутины.

### Как M и P находят друг друга

Связь M и P **взаимная**: когда они связаны, ссылаются друг на друга (`m.p = P`, `p.m = M`). А находят друг друга через **два общих пула простаивающих** под глобальным локом `sched.lock`:

```
sched.pidle — список свободных P
sched.midle — список свободных (припаркованных) M
```

Инициатива бывает с обеих сторон — кто «бесхозный», тот и ищет:

| Кто свободен | Что делает | Функция |
|---|---|---|
| **поток (M)** освободился и ищет, на чём бежать (вернулся из syscall, spinning, только стартовал) | берёт **любой** свободный P из `sched.pidle` | `acquirep` / `pidleget` |
| **P** освободился и его надо кому-то отдать (sysmon отобрал у syscall, появилась работа) | берёт idle-M из `sched.midle` (или создаёт новый) и отдаёт ему P | `startm` / `handoffp` |

**Почему оба не запутаются, кто кого нашёл:** всё под одним `sched.lock`, а «взять» = **атомарно вынуть из пула**. Вынул P из `pidle` — его там больше нет, второй претендент увидит пусто. Инвариант: сущность либо в своём пуле (свободна), либо связана ровно с одним партнёром; переходы только под локом. Поэтому двойной привязки не бывает — операции сериализуются. (Аналогия: биржа с диспетчером — свободные водители-M и машины-P в двух списках, диспетчер под замком разрешает по одной сделке.)

> Счётчики `nmspinning`/`needspinning` тут про **эффективность** (не будить лишних M на каждый чих — против thundering herd), а защита от гонки — именно `sched.lock` + удаление из пула.

---

## P handoff: три сценария

### Сценарий 1: короткий syscall (< ~20 мкс)

```
M[2] → P[2] → G[z]
          ↓ entersyscall
M[2] (syscall, no P)    P[2] (idle, status=Psyscall)
          ↓ exitsyscall (быстро)
M[2] забирает P[2] обратно — fast path
```

Если syscall завершился быстро и P ещё никто не забрал — M берёт его обратно. Нет overhead перепланирования.

### Сценарий 2: syscall затягивается (sysmon retake)

```
M[2] → P[2] → G[z]
          ↓ entersyscall
M[2] (syscall, no P)    P[2] (Psyscall, idle)
          ↓ через ~20мкс sysmon замечает P в Psyscall
sysmon: P[2].status = Pidle, P[2] передаётся idle M или создаётся новый M
          ↓ syscall завершился
M[2] (для исполнения Go нужен P) пытается взять любой свободный P
          → свободных нет → G[z] в global run queue, а сам M[2] паркуется в idle-пул
G[z] продолжит выполнение на другом M+P
```

### Сценарий 3: entersyscallblock (известно заранее что блокирующий)

Некоторые вызовы (например, `os.File.Read` на обычных файлах) вызывают `entersyscallblock` — P отдаётся **немедленно** без ожидания sysmon:

```go
// runtime/proc.go (упрощённо)
func entersyscallblock() {
    gp := getg()
    gp.status = _Gsyscall
    pp := gp.m.p
    pp.status = _Pidle
    handoffp(pp)  // сразу отдать P другому M
}
```

---

## sysmon retake: когда забирают P

`sysmon` — выделенный OS thread (не привязан к P), работает циклически каждые 10–20 мс.

Условие retake P:
```
P.status == Psyscall
AND время в syscall > retake порог (~20 мкс)
AND есть свободные горутины (runq не пуст или global queue не пуст)
```

```
// runtime/proc.go (упрощённо)
func retake(now int64) uint32 {
    for i := 0; i < gomaxprocs; i++ {
        pp := allp[i]
        if pp.status == _Psyscall {
            // Если P простаивает достаточно долго — забрать
            if runqempty(pp) && ... {
                continue  // не торопимся если горутин нет
            }
            if ... elapsed > forcePreemptNS {
                handoffp(pp)  // забрать P
            }
        }
    }
}
```

Если горутин нет — P не забирают даже при долгом syscall (нет смысла создавать новый M зря).

---

## exitsyscall: быстрый и медленный пути

### Fast path (exitsyscallfast)

```go
func exitsyscallfast(oldval uint32) bool {
    // Попробовать взять свой старый P
    if gp.m.oldp.ptr() != nil {
        if cas(&pp.status, _Psyscall, _Prunning) {
            // Взял старый P — продолжаем без перепланирования
            return true
        }
    }
    // Взять любой idle P
    if p := pidleget(0); p != nil {
        acquirep(p)
        return true
    }
    return false
}
```

### Slow path

Если ни один P недоступен:

```go
func exitsyscall0(gp *g) {
    // G → Grunnable
    // Положить в global run queue
    globrunqput(gp)
    // Припарковать M (он будет ждать нового P)
    stopm()
}
```

G продолжит выполнение, когда любой P освободится и возьмёт её из global queue.

### Кто будит M из syscall: OS-ядро, а не Go

Важная деталь: всё выше (`exitsyscall`) — это то, что M делает **после** возврата из OS-ядра. А **разбудить** его из блокирующего syscall Go **не может** — это делает OS-ядро.

M — настоящий поток ОС, и в блокирующем `read()` он **спит внутри OS-ядра** (в Linux — состояние `S`/`D`). Когда вызов может завершиться (пришли данные, истёк таймаут), **OS-ядро** делает поток runnable, OS-планировщик ставит его на CPU, и поток возвращается в user-space → в `exitsyscall`. Go тут не участвует и **не контролирует, когда** это произойдёт.

Не путать два разных пробуждения M:

| M спит... | Кто будит | Механизм |
|---|---|---|
| **в syscall** (в OS-ядре) | **OS-ядро / OS-планировщик** | возврат из syscall |
| **припаркованным** в idle-пуле (`sched.midle`) | **рантайм Go** | `notewakeup` (futex) из `startm`, когда есть P + работа |

То есть из syscall M выходит сам, как только OS-ядро вернёт управление; а уже припаркованный (после неудачного `exitsyscall`) M в следующий раз будит Go через futex.

> **Что такое futex?** *futex* = **fast userspace mutex** — примитив синхронизации в Linux, на котором строятся мьютексы/семафоры. Идея в названии: **быстрый путь целиком в user-space** (атомарный CAS по числу в разделяемой памяти — без syscall, когда нет конкуренции), а в ядро идём **только если надо реально заблокироваться**: `FUTEX_WAIT(addr, val)` усыпляет поток в ядре, `FUTEX_WAKE(addr, n)` будит. Go так **паркует и будит свои потоки (M)**: простаивающий M спит в idle-пуле через `futexsleep` (`FUTEX_WAIT`, CPU не ест), а планировщик будит его `futexwakeup` (`FUTEX_WAKE`), когда есть P + работа. В отличие от M в блокирующем `read()`, которого будит само OS-ядро. (На macOS/BSD вместо futex — семафоры `pthread`/Mach, идея та же.)

### Почему `ctx` не отменяет застрявший syscall

Прямое следствие: **отмена `context` не освобождает M, висящий в блокирующем syscall.** Отмена `ctx` кооперативна — она лишь закрывает `ctx.Done()`, а горутина должна **сама** его проверять (`select`). Но горутина внутри syscall Go-код не исполняет — она в OS-ядре, ни на каком `select` не стоит, и закрытие канала её «не слышит». M остаётся заблокированным, пока **OS-ядро** не вернётся.

| I/O | `ctx`/deadline отменяет? | Почему |
|---|---|---|
| **сеть** (`conn.Read`, http) | **да** | горутина не в OS-ядре, а **в netpoller**; рантайм будит её по дедлайну / при `conn.Close()` |
| **файл** (regular file) | **нет** | M реально висит в OS-ядре на `read()` — прервать нельзя |
| **exec** (`CommandContext`) | да, **косвенно** | отмена **убивает процесс** сигналом → `wait4` возвращается |
| **cgo** в блокирующую C-функцию | **нет** | вне scheduler-контроля, ждём C |

Отменить блокирующий вызов можно, только **заставив OS-ядро вернуться**: закрыть fd (сокеты/pipe), убить процесс (exec), выставить deadline (сеть через netpoller). Для чтения обычного файла такого рычага нет.

> Опасное следствие: даже при «отмене» на Go-уровне (обёртка-горутина вернётся по `ctx.Done()`) сам syscall и его M **остаются заблокированы** в OS-ядре — прекращается лишь ожидание результата. Это **утечка горутины + удержанный поток** до возврата OS-ядра. Поэтому файловый/cgo I/O без ограничения параллелизма опасен, и `ctx` тут не помощник.

### Таймаут на чтение файла

**На обычный файл дедлайн не ставится.** `SetReadDeadline` для regular-файла вернёт ошибку, потому что дедлайны умеет только netpoller, а файл идёт по блокирующему пути:

```go
f, _ := os.Open("/path/to/file")
err := f.SetReadDeadline(time.Now().Add(time.Second))
// err == os.ErrNoDeadline: "file type does not support deadline"
```

**Дедлайн работает для pollable-дескрипторов** (pipe/FIFO, сокеты, tty) — они в O_NONBLOCK и обслуживаются netpoller'ом:

```go
r, w, _ := os.Pipe()             // pipe — pollable
defer r.Close(); defer w.Close()
r.SetReadDeadline(time.Now().Add(time.Second))

buf := make([]byte, 64)
_, err := r.Read(buf)            // никто не пишет в w → через 1с:
// errors.Is(err, os.ErrDeadlineExceeded) == true   ← netpoller разбудил горутину по таймеру
```

**Для regular-файла остаётся только «мягкий» таймаут** — перестать ждать через goroutine + `select`. Но он **не отменяет** чтение (см. выше):

```go
type result struct {
    data []byte
    err  error
}
ch := make(chan result, 1) // буфер 1: чтобы горутина не залипла на send, когда мы уже ушли

go func() {
    data, err := os.ReadFile(path) // ВСЁ РАВНО висит, пока OS-ядро не вернётся
    ch <- result{data, err}
}()

select {
case r := <-ch:
    _ = r // успели
case <-time.After(time.Second):
    // «таймаут»: мы перестали ждать ответ.
    // НО read, его горутина и поток (M) ещё заблокированы в OS-ядре — это не отмена.
}
```

Итог: жёсткого таймаута на чтение обычного файла в Go **нет** (ОС не даёт прервать blocking `read()` regular-файла). Что делать на практике: ограничивать параллелизм файлового I/O (bounded worker pool + `SetMaxThreads`), где можно — использовать сеть/pipe (там дедлайны работают), а goroutine+select применять как мягкий таймаут *ответа приложения*, помня, что поток он не освобождает.

---

## syscall.Syscall vs syscall.RawSyscall

```go
// Пакет syscall предоставляет два варианта:

// Syscall — оборачивает в entersyscall/exitsyscall
// Используется для потенциально blocking вызовов
n, _, err = syscall.Syscall(syscall.SYS_READ, fd, uintptr(p), uintptr(len(p)))

// RawSyscall — без entersyscall/exitsyscall
// ТОЛЬКО для заведомо non-blocking вызовов (getpid, gettimeofday и т.п.)
pid, _, _ = syscall.RawSyscall(syscall.SYS_GETPID, 0, 0, 0)
```

**Почему важно различие:**

`syscall.RawSyscall` с blocking вызовом:
- M заблокирован в OS-ядре
- P не отдан scheduler-у
- никакие другие горутины не могут выполняться на этом P
- при достаточном количестве таких вызовов — весь scheduler может встать

`syscall.Syscall` корректно обрабатывает оба случая через `entersyscall`/`exitsyscall`.

---

## CGo: почему дороже обычного syscall

CGo вызовы идут через отдельный путь, **без** scheduler awareness:

```
Go goroutine → cgocall() → C function
                    ↓
        entersyscall() вызывается явно
        но M "теряет" scheduler context на всё время C call
        C код может вызвать blocking libc функции
```

**Цена CGo:**
1. Thread switch overhead (goroutine → C)
2. Потеря P на всё время C вызова
3. libc malloc/free не знают про Go GC → нельзя хранить Go pointers в C
4. Трудоёмкое управление временем жизни объектов

```go
// CGo call проходит через несколько уровней:
// Go goroutine → cgocall → asmcgocall → C function
// При возврате: exitsyscall → восстановление P

// Измерить цену CGo:
// go test -bench=. -cpuprofile=cpu.prof
// Функции cgocall/cgocallback в профиле — CGo overhead
```

**Правило**: CGo на hot path — красный флаг. 1 CGo вызов ≈ несколько сотен нс overhead против нескольких нс для чистого Go.

---

## runtime.LockOSThread

Иногда нужно, чтобы горутина всегда выполнялась на одном OS thread:

```go
func initOpenGL() {
    // OpenGL контекст привязан к OS thread
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    // Теперь эта горутина всегда на одном M
    // M не вернётся в idle pool пока не вызван UnlockOSThread
    gl.Init()
    gl.CreateWindow(...)
}
```

**Когда нужно:**
- библиотеки с thread-local state (OpenGL, некоторые C библиотеки)
- GUI frameworks
- JNI в мобильных приложениях

**Что происходит:**
- M помечается как `lockedg = gp`
- scheduler не будет переносить горутину на другой M
- этот M не уйдёт в idle pool пока не `UnlockOSThread`

**Внимание:** `LockOSThread` без `UnlockOSThread` — утечка OS thread.

---

## Thread exhaustion: много blocking syscalls

Go создаёт **новые M** когда все M заняты blocking syscall. Нет жёсткого лимита по умолчанию.

```go
// Это создаст 10000 OS threads (по одному на каждый blocking syscall):
for i := 0; i < 10000; i++ {
    go func() {
        ioutil.ReadFile("/path/to/file")  // blocking file I/O
    }()
}
```

**Лимит OS threads:**

```go
// По умолчанию нет лимита (кроме RLIMIT_NPROC OS)
// Явная установка:
runtime.SetMaxThreads(1000)  // паника если превышено
// По умолчанию: 10000 (до Go 1.21), os.MaxInt (после)
```

**Почему файловый I/O создаёт threads:**

Линукс async file I/O (io_uring) в Go не используется (на момент написания). Обычный `read()` — blocking syscall → M блокируется → нужен новый M.

Для CPU-bound файловых операций (сжатие, хэширование больших файлов) — используй worker pool:

```go
// Bounded worker pool для file I/O
sem := make(chan struct{}, runtime.GOMAXPROCS(0)*4)
for _, path := range paths {
    path := path
    sem <- struct{}{}
    go func() {
        defer func() { <-sem }()
        processFile(path)
    }()
}
```

---

## Interview-ready answer

**1. «Что происходит, когда горутина делает blocking syscall?»**

- На входе выполняется `entersyscall`: сохраняются SP/PC горутины, статус → `Gsyscall`, P **отвязывается от M** (сразу через `entersyscallblock`, если вызов заведомо блокирующий, либо через ~20 мкс — sysmon retake). P подхватывает idle-M или создаётся новый — другие горутины продолжают работать. M висит в OS-ядре с этой горутиной, без P. По завершении syscall выполняется `exitsyscall`, и **именно M** (вернувшийся из OS-ядра поток — потоку нужен P, чтобы исполнять Go) пытается **взять P обратно**: если свободен — fast path, продолжает сразу; если нет — G уходит в global run queue, а **сам M паркуется**. Поэтому один blocking syscall не останавливает весь планировщик: P не ждёт M.

**2. «Какие операции блокируют поток (M), а какие нет?»**

- **Блокируют M** (идут через blocking syscall path, держат поток в OS-ядре): чтение **обычного файла** (`os.ReadFile`/`f.Read` — у regular-файлов нет O_NONBLOCK), запуск процесса (`exec` → `wait4`), **cgo**-вызовы. **Не блокируют M**: сетевой I/O (`conn.Read`/`Accept`) — сокет в O_NONBLOCK, горутина паркуется в netpoller, M свободен; таймеры/`time.Sleep` — per-P heap. **Вообще не syscall**: `time.Now()` — `clock_gettime` через vDSO, в OS-ядро не заходит.

**3. «Чем vDSO отличается от обычного syscall?»**

- Обычный syscall дорог из-за **mode switch** (переход user→kernel и обратно). **vDSO** — страница с кодом/данными, которую OS-ядро отображает в адресное пространство процесса; «лёгкие» вызовы вроде `clock_gettime` выполняются **в user-space без захода в OS-ядро** (читают обновляемую OS-ядром страницу с часами). Поэтому `time.Now()` — это десятки нс, не syscall, и планировщика не касается.

**4. «Как M и P снова находят друг друга после syscall?»**

- Связь взаимная (`m.p ↔ p.m`), а матчинг — через общие пулы под `sched.lock`: свободные P в `sched.pidle`, свободные M в `sched.midle`. Инициатива у «бесхозного»: освободившийся **M** берёт P (`acquirep`), а освободившийся **P** отдают idle-M (`startm`/`handoffp`). Гонки нет — «взять» = атомарно вынуть из пула под локом, поэтому двойной привязки не бывает.

**5. «Почему тысячи блокирующих syscall'ов опасны, а сетевых соединений — нет?»**

- Каждый blocking syscall держит свой M в OS-ядре, и Go под нагрузкой **поднимает новые потоки** (до `SetMaxThreads`, дефолт был 10000) → файловый I/O в цикле без ограничения параллелизма может упереться в лимит (thread exhaustion). Сетевой I/O этой проблемы не имеет: он идёт через netpoller, M не блокируется вообще, и 100k соединений живут на горстке потоков. Фикс для файлового/блокирующего — bounded worker pool.

**6. «Не мешают ли GC горутины, висящие в syscall?»**

- Нет. Горутина в `Gsyscall` не исполняет Go-код → её стек заморожен, а SP/PC сохранены в `entersyscall`. Значит **syscall = safe point**: GC может просканировать её стек сам, не дожидаясь пробуждения. Поэтому тысячи горутин в блокирующих вызовах не тормозят сборку.
