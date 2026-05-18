# Процессы и потоки

Процесс — это работающая программа с **изолированной** памятью и собственными ресурсами. Поток (thread) — это путь исполнения **внутри** процесса, разделяющий память с другими потоками того же процесса. Это базовые единицы, которыми оперирует OS.

В Go ты обычно работаешь с **goroutines** — это **не** OS threads. Понимание этой разницы — ключ к пониманию того, как Go масштабируется до миллионов "потоков" на одной машине, тогда как OS thread'ов на той же машине бывает максимум десятки тысяч.

## Содержание

- [Простая аналогия](#простая-аналогия)
- [Что такое процесс](#что-такое-процесс)
- [Что такое поток (thread)](#что-такое-поток-thread)
- [Процесс vs поток: разница в одной таблице](#процесс-vs-поток-разница-в-одной-таблице)
- [Создание процесса: fork() и exec()](#создание-процесса-fork-и-exec)
- [Создание потока: clone() в Linux](#создание-потока-clone-в-linux)
- [Kernel mode vs User mode](#kernel-mode-vs-user-mode)
- [Syscalls — переход между mode](#syscalls-переход-между-mode)
- [task_struct в Linux](#task_struct-в-linux)
- [Что видно через /proc](#что-видно-через-proc)
- [Goroutines vs threads vs processes](#goroutines-vs-threads-vs-processes)
- [Когда Go создаёт OS thread](#когда-go-создаёт-os-thread)
- [LockOSThread и привязка к потоку](#lockosthread-и-привязка-к-потоку)
- [Практические выводы](#практические-выводы)

---

## Простая аналогия

Представь офисное здание с несколькими **квартирами** (процессы). У каждой квартиры — свой адрес, своя мебель, свои ключи. Жильцы одной квартиры не могут зайти в другую без специального приглашения.

Внутри квартиры могут жить несколько **людей** (потоки). Они разделяют ту же гостиную, кухню, ванную (память процесса), но у каждого — своя постель, свои личные вещи (стек, регистры). Они работают независимо, но в одном пространстве. Один может случайно сломать общую посуду — это affects всех в квартире.

Сравнение:
- **Изоляция:** между процессами — стены и замки. Внутри процесса между потоками — только договорённости.
- **Связь:** процессы общаются "почтой" (IPC, sockets). Потоки — просто пишут друг другу в общую тетрадь (shared memory).
- **Стоимость:** построить квартиру дорого (создание процесса). Завести нового жильца — относительно дёшево (создание потока).

---

## Что такое процесс

С точки зрения OS, процесс — это **контейнер ресурсов**:

**1. Виртуальное адресное пространство.**
Свои стек, heap, code, data segments (см. [05-virtual-memory-and-paging.md](./05-virtual-memory-and-paging.md)). Адрес `0x1000` в процессе A — совершенно другой физический байт чем в процессе B.

**2. PID (Process ID).**
Уникальный идентификатор. В Linux — целое число до 2^22 (4 миллиона) обычно. Первый процесс — `init` (PID 1, либо systemd, либо в контейнере — твоё приложение).

**3. Таблица file descriptors.**
Список открытых файлов, сокетов, pipe'ов. По умолчанию: 0 = stdin, 1 = stdout, 2 = stderr. Все открытые `os.Open` и `net.Dial` добавляются сюда.

**4. Working directory.**
Текущая директория (`pwd`). Все относительные пути считаются от неё.

**5. Environment variables.**
`$PATH`, `$HOME`, и т.д.

**6. UID/GID.**
От чьего имени работает процесс. Определяет права доступа к файлам и системным операциям.

**7. Signal handlers.**
Что делать при SIGINT, SIGTERM, SIGSEGV. См. [linux/04-signals-and-processes.md](../linux/04-signals-and-processes.md).

**8. Состояние выполнения.**
Running, sleeping, zombie, stopped. Видно в `ps`.

### Когда процесс создаётся

Только через `fork()` (или производные — `vfork`, `clone`). Каждый процесс имеет **родителя** (parent process). Образуется дерево.

```bash
$ pstree -p
systemd(1)
├─sshd(842)
│   └─sshd(2103)
│       └─bash(2105)
│           └─go(2200)
│               └─myapp(2201)
└─dockerd(950)
    └─containerd-shim(1234)
        └─myapp-in-container(1)  ← PID 1 внутри namespace
```

---

## Что такое поток (thread)

Поток — это **независимый путь выполнения** внутри одного процесса.

Несколько потоков в процессе **разделяют:**
- Виртуальное адресное пространство (тот же heap, тот же code)
- File descriptors
- Working directory, environment
- UID/GID
- Signal handlers (но каждый thread может маскировать сигналы по-своему)

Каждый поток имеет **своё:**
- TID (Thread ID)
- Стек (обычно 8 MB на Linux по умолчанию, см. `ulimit -s`)
- Регистры (RAX, RBP, RIP, etc.)
- Состояние выполнения

**Главная разница:** потоки одного процесса видят одну и ту же память. Если thread A пишет `var = 42`, thread B сразу же может это прочитать. Это — основа multi-threaded программирования (и источник race conditions).

### Multithreading vs multiprocess

- **Multithreaded:** один процесс, несколько потоков → общая память, дешёвые "коммуникации" (просто переменные), но **catastrophic crash** одного потока валит весь процесс.
- **Multiprocess:** несколько процессов → изоляция, crash одного не валит других, но "общаются" через IPC (медленнее).

Web-серверы исторически делятся:
- **Apache prefork** — multiprocess. Стабильно, но дорого по памяти.
- **Apache worker / nginx** — multithreaded внутри. Эффективнее.
- **Postgres** — multiprocess (forks per connection). Изоляция от crash.
- **Go** — multithreaded внутри процесса, но **goroutines** мультиплексируются (см. ниже).

---

## Процесс vs поток: разница в одной таблице

| Свойство | Process | Thread |
|---|---|---|
| Address space | Own | Shared with siblings |
| File descriptors | Own | Shared |
| PID/TID | Own PID | Own TID, parent PID |
| Стоимость создания | Большая (~1 мс) | Маленькая (~10-100 мкс) |
| Стоимость context switch | Высокая (TLB flush) | Дешевле (часть состояния та же) |
| Crash effect | Crash isolated | Crash валит весь процесс |
| Коммуникация | IPC (pipes, sockets) | Shared memory + sync |
| OS контейнер ресурсов | Yes | No (использует ресурсы процесса) |
| Memory overhead | MB (свой address space) | KB (только стек) |

---

## Создание процесса: fork() и exec()

В Unix-like системах есть только **один** способ создать процесс — `fork()`. Он создаёт **копию** существующего процесса:

```c
pid_t pid = fork();
if (pid == 0) {
    // Внутри child процесса
    // Всё то же что у parent, но pid = 0 здесь
} else if (pid > 0) {
    // Внутри parent процесса
    // pid = PID нового child
} else {
    // Ошибка
}
```

После fork:
- Child наследует **копию** всей памяти parent'а (благодаря COW — копируется лениво, см. [05-virtual-memory-and-paging.md](./05-virtual-memory-and-paging.md))
- Child наследует копии open file descriptors
- Child имеет свой PID, parent — свой
- Оба продолжают выполняться **с следующей инструкции после fork()** (но в разных PID)

### exec()

Чтобы запустить **другую** программу, после fork делается `exec()` — он **заменяет** содержимое процесса новой программой:

```c
pid_t pid = fork();
if (pid == 0) {
    // Заменить себя на /usr/bin/ls
    execvp("/usr/bin/ls", argv);
    // Если execvp удался — этот код уже НЕ выполнится
}
// Parent ждёт child
waitpid(pid, &status, 0);
```

Это паттерн "fork+exec" — стандартный способ запустить новую программу. Shell делает именно это для каждой команды.

В Go:
```go
cmd := exec.Command("/usr/bin/ls", "-la")
cmd.Run()  // под капотом — fork+exec
```

### Если бы не было fork

В Windows нет fork — используется `CreateProcess()`, который сразу создаёт процесс с другой программой. Это проще архитектурно. Но **fork+exec гибче**: между ними можно настроить пайпы, поменять environment, установить limits, и т.д.

### Тонкости fork

- **Только текущий thread "переходит" в child.** Если в parent было 10 threads, в child останется только тот, который сделал fork(). Остальные просто исчезают вместе с их данными — что часто приводит к проблемам (locked mutex'ы в never-running threads, и т.д.).
- **Для этого fork+exec обычно делают сразу.** Если планируется работать после fork **без** exec — лучше избегать многопоточного fork.
- **Async-safe functions only between fork and exec.** Большинство стандартных функций НЕ безопасно вызывать в child до exec.

---

## Создание потока: clone() в Linux

В Linux нет специального syscall для потоков. Есть **`clone()`** — обобщение fork, позволяющее точно контролировать что разделять с родителем.

```c
clone(flags, stack, ...)
```

Через flags указывается что разделить:
- `CLONE_VM` — общая память
- `CLONE_FILES` — общие file descriptors
- `CLONE_FS` — общая working directory
- `CLONE_SIGHAND` — общие signal handlers
- `CLONE_THREAD` — это thread того же процесса (TID, не PID)
- `CLONE_NEWNS`, `CLONE_NEWPID`, etc. — namespaces для контейнеров

**Fork** = clone без всех CLONE_* флагов (всё дублируется).

**Thread** = clone со всеми CLONE_* флагами (всё разделено).

**Контейнер** = clone с CLONE_NEW* флагами (новые namespaces).

То есть в Linux fork, threads, и контейнеры — варианты одного и того же syscall'а. Очень элегантно.

В библиотеках:
- `pthread_create()` (C) — высокоуровневая обёртка над clone() для threads
- `clone()` напрямую — для специфических задач (контейнеры через runc используют clone)

В Go:
- `go func() { ... }()` — создаёт **goroutine**, не OS thread (см. ниже)
- Runtime сам управляет OS threads под капотом

---

## Kernel mode vs User mode

CPU имеет два (на x86 фактически 4 кольца, но обычно используются 2) режима выполнения:

**User mode (ring 3):**
- Программы пользователя
- НЕ имеет прямого доступа к hardware
- НЕ может выполнять привилегированные инструкции (`HLT`, `CLI`, изменение page tables, etc.)
- Доступ к памяти — только к виртуальному пространству **своего процесса**
- Если попытается сделать что-то запрещённое → CPU exception → kernel убивает процесс

**Kernel mode (ring 0):**
- Ядро OS, драйверы
- Полный доступ ко всему hardware
- Может выполнять любые инструкции
- Доступ ко всей памяти, включая других процессов

```
┌─── Kernel space (ring 0) ────────┐
│   Scheduler, FS, network stack,   │
│   drivers, page tables, etc.       │
├──────────────────────────────────┤
│         Syscall boundary           │  ← переход через специальный механизм
├──────────────────────────────────┤
│   User space (ring 3)              │
│   ┌─────────┐  ┌─────────┐         │
│   │Process A│  │Process B│ ...     │
│   └─────────┘  └─────────┘         │
└──────────────────────────────────┘
```

### Зачем нужны два режима

Защита. Без них любая программа могла бы:
- Прочитать память других процессов (включая пароли)
- Сбить page tables (положить весь компьютер)
- Прямо общаться с hardware (диск, сеть) — обходя security

Kernel — единственный "доверенный" слой, через который всё проходит. Все взаимодействия user-space программ с hardware и с другими процессами — через kernel.

---

## Syscalls — переход между mode

Программа хочет открыть файл, послать сетевой пакет, выделить память — это всё операции, требующие kernel. Способ запросить услугу у ядра — **syscall** (system call).

```
User program:
   read(fd, buf, 4096)
            ↓
   Setup arguments in registers
            ↓
   SYSCALL instruction (x86_64) или INT 0x80 (старый x86)
            ↓
   CPU mode: User → Kernel
            ↓
   Kernel: dispatch к функции sys_read
            ↓
   Read data from file (actual I/O if needed)
            ↓
   Set return value in register
            ↓
   SYSRET instruction
            ↓
   CPU mode: Kernel → User
            ↓
User program: continue
```

### Стоимость syscall

Базовая стоимость на современных Intel/AMD: ~100-500 нс. После Spectre/Meltdown mitigations (KPTI) — увеличилась в 1.5-3 раза.

Для backend это значит: **сотни тысяч syscall'ов в секунду — нормально, миллионы — потолок**. Поэтому:
- Buffered I/O > unbuffered (один большой syscall vs много мелких)
- `bufio.Reader` в Go буферизует чтение из net.Conn
- HTTP keep-alive — много запросов через один TCP connection

### Какие операции — syscall

В Go:
- `os.Open`, `os.Read`, `os.Write` — syscalls
- `net.Dial`, `Read`/`Write` на сокет — syscalls
- `time.Now()` — syscall на старых системах, но в Linux обычно через `vDSO` (без перехода в kernel)
- `make([]byte, ...)` — может вызвать syscall (mmap для большой памяти), но мелкие — нет
- `mutex.Lock()` — обычно НЕ syscall (atomic CAS в user space), только при contention — syscall (futex для sleep)
- `runtime.Gosched()` — не syscall, передача управления внутри Go runtime

### vDSO — syscall без перехода в kernel

Некоторые syscall'ы (time, getpid) Linux экспонирует через специальный shared object `vDSO`, mapped в каждый процесс. Программа "делает syscall", но фактически выполняется код в user-space, который читает данные kernel'а через shared memory.

Это даёт **`time.Now()` за ~25 нс** в Go вместо ~500 нс честного syscall.

---

## task_struct в Linux

В ядре Linux каждый process/thread представлен структурой `task_struct`. Это огромная struct с сотнями полей:

```c
struct task_struct {
    pid_t pid;
    pid_t tgid;          // Thread Group ID — это PID процесса для thread'ов
    struct task_struct *parent;
    struct mm_struct *mm;        // Memory management (page tables)
    struct files_struct *files;  // File descriptors
    struct fs_struct *fs;        // Filesystem info (cwd, etc.)
    struct signal_struct *signal;
    int state;                    // RUNNING, INTERRUPTIBLE, ZOMBIE, etc.
    // ... сотни полей
};
```

**Ключевые поля:**
- `pid` — TID (yes, в kernel это всегда называется PID, но для thread'ов — это TID)
- `tgid` — Thread Group ID — это и есть PID процесса с user point of view
- `mm` — указатель на структуру памяти. **Threads** одного процесса имеют **тот же** `mm`.

### State машина процесса

```
            ┌─ TASK_RUNNING (executing or runnable)
            │
new ────────┼─ TASK_INTERRUPTIBLE (sleeping, can be woken by signal)
            │
            ├─ TASK_UNINTERRUPTIBLE (sleeping in kernel, e.g., disk I/O)
            │
            ├─ TASK_STOPPED (SIGSTOP)
            │
            └─ TASK_ZOMBIE (process exited, waiting for parent to read exit code)
```

В `ps` и `top` видишь эти состояния в столбце `STAT`:
- `R` — running
- `S` — interruptible sleep
- `D` — uninterruptible sleep (часто signals to disk I/O issue)
- `Z` — zombie
- `T` — stopped

### Zombie процессы

Когда процесс завершается (`exit()`), его `task_struct` остаётся в kernel до тех пор пока parent не прочитает exit code через `wait()`. Если parent не делает wait — child становится "zombie".

Zombies не потребляют CPU/memory, но занимают place в process table. Если zombie много (например, бесконечно spawn'ишь процессы и не делаешь wait) — таблица процессов забивается, и новый fork() будет fail'ить.

В Go-сервисах в Docker — твой процесс PID 1, и если он не reap'ит детей правильно, накапливаются zombie. Решение — использовать `tini` как init в контейнере, или Go-библиотеку для reaping.

---

## Что видно через /proc

В Linux `/proc` — виртуальная файловая система с информацией о ядре и процессах. Каждый процесс — папка `/proc/<PID>/`:

```bash
ls /proc/$(pgrep myapp)/
# cmdline        — командная строка
# environ        — environment variables
# status         — текстовое описание состояния процесса
# stat           — те же данные в machine-friendly формате
# maps           — карта виртуального адресного пространства
# fd/            — open file descriptors
# task/          — потоки (TID'ы)
# limits         — ulimits
# cgroup         — в какой cgroup состоит
# mountinfo      — какие файловые системы видит
```

**Очень полезные:**

```bash
# Кто открыл какие файлы
ls -la /proc/PID/fd/

# Карта памяти
cat /proc/PID/maps
# 00400000-00567000 r-xp /usr/bin/myapp     ← code
# 7ffff7a00000-7ffff7c00000 rw-p [heap]      ← heap
# 7ffffffdd000-7ffffffff000 rw-p [stack]    ← stack

# Текущие потоки процесса
ls /proc/PID/task/
# 12345 12346 12347  ← TIDs

# Smapped потоки можно изучать отдельно
cat /proc/PID/task/12346/status
```

В Go:
```go
// runtime.NumGoroutine() показывает goroutines, не OS threads
// Чтобы посмотреть OS threads — /proc/PID/task/

// Через runtime.GOMAXPROCS можно ограничить число P (logical processors)
// runtime.LockOSThread() — закрепить goroutine за OS thread
```

---

## Goroutines vs threads vs processes

```
┌──────────── Process ────────────┐
│                                  │
│  ┌──── OS Thread 1 ─────┐        │
│  │   ┌─ Goroutine A    │        │
│  │   ├─ Goroutine B    │        │
│  │   └─ Goroutine C    │ ← мультиплекс
│  └──────────────────────┘        │
│                                  │
│  ┌──── OS Thread 2 ─────┐        │
│  │   ┌─ Goroutine D    │        │
│  │   └─ Goroutine E    │        │
│  └──────────────────────┘        │
│                                  │
└──────────────────────────────────┘
```

**Goroutine** — это **user-level thread**, управляемый Go runtime. Не OS thread.

| Свойство | Process | OS Thread | Goroutine |
|---|---|---|---|
| Создание | ~1 мс | ~50-100 мкс | ~1 мкс |
| Минимальная память | ~1 MB | ~2-8 MB | ~2 KB (растущий стек) |
| Количество на машине | ~1000 | ~10000 | **миллионы** |
| Управление | OS scheduler | OS scheduler | Go runtime scheduler |
| Контекст switch | 1-10 мкс (с TLB flush) | 1-5 мкс | ~200 нс |

### Как Go runtime использует OS threads

Go runtime использует модель **M:N scheduling**:

- **G** (goroutines) — пользовательские задачи, тысячи или миллионы
- **M** (machines) — OS threads, обычно сколько-то
- **P** (processors) — логические "слоты" исполнения, по умолчанию = `GOMAXPROCS` = NumCPU

Goroutines выполняются на P. Каждый P привязан к одному M. Если goroutine блокируется (syscall, channel), runtime может **переместить** другие goroutines с этого P на другой M, чтобы продолжить работу.

Подробнее: [01-go-core/07-scheduler-and-preemption.md](../../01-go-core/07-scheduler-and-preemption.md) и [07-context-switching-and-scheduling.md](./07-context-switching-and-scheduling.md).

### Почему goroutines дешевле

- **Стек растёт по необходимости** (от 2 KB). У OS thread — фиксированные 8 MB сразу.
- **Switch без kernel** — Go runtime сам переключает goroutines, без syscall. Switch — это `save registers; load registers`, ~200 нс.
- **Lazy creation** — runtime создаёт OS threads только когда нужны.

### Когда Go использует OS threads

- На запуске создаются threads под `GOMAXPROCS` (или больше при блокировках)
- При блокирующем syscall — Go может запустить **новый thread** чтобы продолжить работу остальных goroutines
- При вызове CGO — Go pinning'ует goroutine к thread'у (LockOSThread под капотом)

В `runtime/debug.SetMaxThreads()` можно увидеть лимит — по умолчанию 10000. Если Go-программа создаёт больше — это знак что что-то не так (например, много блокирующих CGO вызовов).

---

## Когда Go создаёт OS thread

Понимание помогает диагностировать "почему мой Go-сервис вдруг использует 100+ OS threads".

**1. По умолчанию — `GOMAXPROCS` threads.**
На 8-core CPU — 8 threads сразу при запуске для выполнения CPU-bound горутин.

**2. Когда goroutine делает блокирующий syscall.**
Например, `os.File.Read()` на медленный диск — syscall блокируется. Go runtime отвязывает thread (M) от P, оставляя его в syscall'е, и берёт другой thread (или создаёт новый) чтобы взять P и продолжить работу остальных goroutines.

Когда syscall завершается, thread может снова получить P, или остаться в "свободном пуле".

**3. CGO calls.**
Каждый вызов CGO блокирует thread (Go runtime не может управлять C-кодом). Если у тебя 100 параллельных CGO calls — у тебя 100 OS threads.

**4. `runtime.LockOSThread()`.**
Если goroutine "залочила" thread (см. ниже) — этот thread не может быть использован для других goroutines.

**5. Network polling.**
Go использует один (или несколько) thread'ов для `epoll`/`kqueue` (net poller). На Linux обычно один.

### Сколько threads "норма"

Для типичного Go-сервиса:
- Stateless API сервис: ~`GOMAXPROCS + 1-3 threads` (CPU + epoll + GC)
- С блокирующими syscalls (тяжёлый I/O): `GOMAXPROCS + N`, где N — peak параллельных блокирующих операций
- С CGO: `GOMAXPROCS + параллельных CGO calls`
- С `runtime.LockOSThread`: + locked goroutines

Если видишь сотни/тысячи threads в Go-приложении — это аномалия. Часто причина — где-то синхронные CGO вызовы.

---

## LockOSThread и привязка к потоку

Иногда нужно чтобы goroutine **всегда** выполнялась на **одном и том же** OS thread'е:

```go
import "runtime"

go func() {
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    // Этот код всегда выполняется на одном thread'е
    // ...
}()
```

### Зачем нужно

**1. Thread-local state в C-библиотеках.**
Многие C-библиотеки хранят данные в thread-local storage (TLS), привязанные к pthread. Если Go-goroutine "прыгает" между threads — TLS будет разный.

**2. OpenGL и GUI frameworks.**
Большинство GUI требуют чтобы все calls шли с "main thread". Goroutine должна быть locked к OS main thread.

**3. Signal handlers.**
Если goroutine хочет обрабатывать signal сама — нужно lock'нуть, иначе signal придёт неизвестному thread'у.

**4. Process-level ops.**
Например, `setuid()` в Linux работает per-thread на новых kernel'ах. Чтобы изменить UID для всего процесса — нужно lock thread (или сделать в каждом thread).

### Стоимость

- Locked thread не может быть использован для других goroutines — это "потерянный" М для Go scheduler
- Если у тебя много locked goroutines — Go runtime создаст много OS threads
- При `Lock` thread "становится постоянным" — не возвращается в пул

### main goroutine

`runtime.main()` (та goroutine которая выполняет `func main()`) — **не** locked к main OS thread по умолчанию. Если нужно (например, для GUI) — вызови `runtime.LockOSThread()` в `init()` файла, который импортируется первым.

---

## Практические выводы

**1. Goroutines ≠ OS threads.**
Не путай в обсуждении. "У меня 10000 goroutines" — нормально. "У меня 10000 threads" — это плохо.

**2. Используй goroutines щедро.**
Создание goroutine — 1 мкс. Если структура задачи естественно concurrent — пиши N goroutines.

**3. Crash в одной goroutine = crash процесса.**
В отличие от процессов, потоки одного процесса не изолированы. `panic` в goroutine без `recover` валит весь процесс. Для критичных частей делай isolated processes (через `exec.Command`).

**4. Контейнер — это процесс.**
В Kubernetes / Docker контейнер — это namespace + cgroup + процесс. Если твой Go-сервис в контейнере PID 1 — он отвечает за reap'инг zombie детей (например, после `exec.Command`).

**5. `ulimit -u` и max processes.**
По умолчанию ~1024-4096 processes/threads на пользователя. Для Go-сервиса с большим CGO load или много external commands — может быть мало. Увеличивай в систему.

**6. Понимай разницу process model и thread model.**
PostgreSQL forks процесс на каждое соединение — изоляция и стабильность. Nginx использует workers + threads — эффективность. Go — горутины + thread pool — оба плюса.

**7. Знай /proc.**
`/proc/PID/status`, `/proc/PID/maps`, `/proc/PID/task/` — твои друзья при debug'е. Можешь увидеть state, память, threads, open files без отладчика.

**8. Минимум syscall'ов.**
Каждый syscall — 100-500 нс + cache flush. Буферизуй I/O, используй keep-alive для TCP, batch'и операции.

---

См. также:
- [linux/04-signals-and-processes.md](../linux/04-signals-and-processes.md) — сигналы, init, zombie/orphan, PID 1
- [01-go-core/07-scheduler-and-preemption.md](../../01-go-core/07-scheduler-and-preemption.md) — Go scheduler внутри
- [07-context-switching-and-scheduling.md](./07-context-switching-and-scheduling.md) — следующий файл про планирование
