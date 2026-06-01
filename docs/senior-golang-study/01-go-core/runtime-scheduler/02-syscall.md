# Syscall: механика и scheduler handoff

Системный вызов — переход из user space в kernel space. В Go это нетривиально: scheduler должен не заблокировать P пока M ждёт ответа от ядра. Понимание этого механизма объясняет почему Go масштабируется до тысяч concurrent I/O операций без тысяч OS threads.

## Содержание

- [Blocking vs non-blocking: OS уровень](#blocking-vs-non-blocking-os-уровень)
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

## Go syscall path: entersyscall → exitsyscall

Каждый blocking syscall в Go оборачивается в `entersyscall` / `exitsyscall`:

```
goroutine calls read(fd, ...)
        ↓
runtime.entersyscall()
  • сохранить SP, PC горутины (для GC stack scan)
  • выставить G.status = Gsyscall
  • отвязать P от M (P → idle или подхватывается другим M)
        ↓
SYSCALL (ядро выполняет read)
  • M заблокирован в ядре
  • P уже не привязан к этому M
  • GC может сканировать stack (P.status = Psyscall)
        ↓
runtime.exitsyscall()
  • попытаться взять P обратно (fast path)
  • если не вышло → runnable queue (slow path)
```

Ключевое: в момент нахождения в syscall M и G **существуют без P**. P может быть подхвачен другим M из idle pool, который продолжит выполнять другие горутины.

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
M[2] пытается взять P — нет свободных → G[z] в global run queue
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
- M заблокирован в ядре
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

**"Что происходит когда горутина делает syscall?"**

При blocking syscall горутина вызывает `entersyscall`: P отвязывается от M (либо немедленно через `entersyscallblock`, либо через ~20мкс через sysmon retake). P подхватывается idle M или создаётся новый M — другие горутины продолжают работу. M с горутиной блокируется в ядре в состоянии `Gsyscall`, без P.

По завершении syscall горутина вызывает `exitsyscall` и пытается быстро взять P обратно. Если P свободен — fast path, продолжает сразу. Если нет — горутина идёт в global run queue и продолжит на любом M+P.

Именно поэтому blocking syscalls не останавливают весь scheduler: P не ждёт M, а сразу отдаётся для другой работы. Но при тысячах concurrent blocking syscalls Go будет создавать тысячи OS threads — поэтому файловый I/O в цикле без ограничения параллелизма может исчерпать threads. Для сетевого I/O эта проблема не стоит — он идёт через netpoller и не блокирует M вообще.
