# File Descriptors и I/O модели

Всё в Linux — файл. Сокеты, трубы, устройства, timer'ы — всё это file descriptors. I/O модели (blocking, select, epoll) напрямую определяют, как Go обрабатывает тысячи соединений.

## Содержание

- [File descriptor: что это и как устроено](#file-descriptor-что-это-и-как-устроено)
- [Таблицы: per-process → open file → inode](#таблицы-per-process--open-file--inode)
- [Типы file descriptors](#типы-file-descriptors)
- [Лимиты: ulimit и system-wide](#лимиты-ulimit-и-system-wide)
- [I/O модели: от blocking до epoll](#io-модели-от-blocking-до-epoll)
- [epoll: механика](#epoll-механика)
- [Level-triggered vs Edge-triggered](#level-triggered-vs-edge-triggered)
- [Go netpoller: как Go строится на epoll](#go-netpoller-как-go-строится-на-epoll)
- [io_uring: следующее поколение](#io_uring-следующее-поколение)
- [Практика: диагностика fd](#практика-диагностика-fd)
- [Interview-ready answer](#interview-ready-answer)

---

## File descriptor: что это и как устроено

File descriptor (fd) — небольшое неотрицательное целое число, которое процесс использует как ссылку на открытый ресурс. Ядро держит реальный объект (файл, сокет, pipe и т.д.), процесс работает только с fd.

Стандартные fd:
```text
0  → stdin
1  → stdout
2  → stderr
3+ → открытые файлы, сокеты, pipe...
```

```bash
# Посмотреть открытые fd процесса
ls -la /proc/$(pgrep -f my-service)/fd/
# lrwxrwxrwx 0 -> /dev/null
# lrwxrwxrwx 1 -> /dev/null
# lrwxrwxrwx 3 -> socket:[12345678]
# lrwxrwxrwx 4 -> socket:[12345679]
# ...
```

---

## Таблицы: per-process → open file → inode

Три уровня таблиц в ядре:

```text
Process A                 Kernel
┌────────────┐         ┌─────────────────────┐    ┌──────────────┐
│ fd table   │         │ Open File Table     │    │ Inode Table  │
│ fd 3 ──────┼────────►│ entry (offset, flags│───►│ /var/log/app │
│ fd 4 ──────┼──┐      │ refcount: 1)        │    │ (permissions,│
└────────────┘  │      ├─────────────────────┤    │ timestamps)  │
                │      │ entry (offset, flags│    └──────────────┘
Process B       │      │ refcount: 1)        │
┌────────────┐  └─────►│                     │
│ fd table   │         └─────────────────────┘
│ fd 5 ──────┼──────────────────────────────►│ (то же inode!)
└────────────┘
```

Важные следствия:
- `fork()` дублирует fd-таблицу → дочерний процесс наследует все fd родителя. Если не закрыть — fd «утечёт».
- Два процесса могут открыть один файл → два entry в open file table → независимые offset'ы.
- `dup2(old, new)` — дублирует fd, оба указывают на один entry open file table → **shared offset**.
- Файл удалён на диске, но fd открыт → inode и данные живут, пока fd не закрыт (вот почему `lsof` показывает `(deleted)`).

---

## Типы file descriptors

```bash
# Что может быть за fd — смотрим на символические ссылки в /proc/<pid>/fd/

# Обычный файл
lrwx 3 -> /var/log/app.log

# Сокет (детали через ss или /proc/net/tcp)
lrwx 4 -> socket:[4026532415]

# Pipe (анонимный)
lrwx 5 -> pipe:[4026532416]

# Именованный pipe (FIFO)
lrwx 6 -> /tmp/my.fifo

# epoll instance
lrwx 7 -> anon_inode:[eventpoll]

# timerfd
lrwx 8 -> anon_inode:[timerfd]

# signalfd (получать сигналы через fd вместо handler)
lrwx 9 -> anon_inode:[signalfd]

# eventfd (уведомление между потоками)
lrwx 10 -> anon_inode:[eventfd]
```

Go runtime создаёт `epoll` fd при старте (netpoller), `timerfd` для runtime timer, `eventfd` для wakeup.

---

## Лимиты: ulimit и system-wide

### Per-process лимиты

```bash
# Посмотреть текущие лимиты
ulimit -a

# Soft limit на число открытых fd
ulimit -n          # обычно 1024

# Изменить в текущей сессии (только до hard limit)
ulimit -n 65536

# Посмотреть limits конкретного процесса
cat /proc/$(pgrep my-service)/limits
# Max open files  65536  65536  files
```

**Soft vs Hard limit**:
- Soft limit — реальное ограничение для процесса.
- Hard limit — потолок, до которого процесс сам может поднять soft limit.
- Только root может поднять hard limit.

Производственное правило: Go HTTP сервер с 10k соединений + connection pools к DB/Redis + файлы логов. При `ulimit -n 1024` — `too many open files` при нагрузке.

```bash
# Поднять лимит системно (сохраняется после перезагрузки)
cat /etc/security/limits.conf
# myapp soft nofile 65536
# myapp hard nofile 65536
# * soft nofile 65536

# Для systemd сервисов
cat /etc/systemd/system/myapp.service
# [Service]
# LimitNOFILE=65536
```

В Docker:
```bash
docker run --ulimit nofile=65536:65536 my-service
```
В Kubernetes — через securityContext или на уровне ноды (`/etc/security/limits.conf` на хосте).

### System-wide лимит

```bash
# Максимум открытых файлов в системе
cat /proc/sys/fs/file-max        # обычно 9223372036854775807 на modern Linux

# Текущее использование: открыто / свободно / максимум
cat /proc/sys/fs/file-nr
# 12928   0   9223372036854775807

# Изменить на лету
sysctl -w fs.file-max=2097152
```

---

## I/O модели: от blocking до epoll

### Blocking I/O

Поток вызывает `read(fd)` → ждёт данных → ОС блокирует поток → данные пришли → поток разблокируется.

```text
Thread 1: [read fd3]──────────────────────────►[data ready][continue]
Thread 2: [read fd4]──────────────[data ready][continue]
Thread 3: ...
```

Для 10k соединений нужно 10k потоков. Каждый поток = ~2 MB stack → 20 GB RAM только на стеки. Context switch между потоками дорогой. **Не масштабируется**.

### Non-blocking I/O + busy polling

```c
fcntl(fd, F_SETFL, O_NONBLOCK);
while (true) {
    int n = read(fd, buf, sizeof(buf));
    if (n == -1 && errno == EAGAIN) {
        // данных нет, попробуем позже
        continue;
    }
    // обработать данные
}
```

`EAGAIN` — данных ещё нет. Цикл крутится и жжёт CPU. **Не используется** напрямую, только как основа для мультиплексинга.

### select (POSIX)

```c
fd_set readfds;
FD_ZERO(&readfds);
FD_SET(fd1, &readfds);
FD_SET(fd2, &readfds);

// Блокируемся до готовности любого fd или timeout
select(maxfd + 1, &readfds, NULL, NULL, &timeout);

// Перебираем все fd чтобы найти готовые
for (int i = 0; i <= maxfd; i++) {
    if (FD_ISSET(i, &readfds)) { /* готов */ }
}
```

Проблемы `select`:
- Максимум **1024 fd** (FD_SETSIZE).
- При каждом вызове нужно заново строить fd_set.
- Сканирование всех fd O(n) даже если готов один.
- Передача fd_set в ядро и обратно при каждом вызове — overhead.

### poll

```c
struct pollfd fds[N];
fds[0] = {.fd = fd1, .events = POLLIN};
fds[1] = {.fd = fd2, .events = POLLIN};

poll(fds, N, timeout_ms);

for (int i = 0; i < N; i++) {
    if (fds[i].revents & POLLIN) { /* готов */ }
}
```

`poll` снял ограничение в 1024 fd (массив вместо битового поля). Но O(n) сканирование осталось. При каждом вызове — копирование массива в ядро. **Не масштабируется** на тысячи fd.

### epoll (Linux-специфично, но на практике везде)

```c
// Создать epoll instance (возвращает fd)
int epfd = epoll_create1(0);

// Зарегистрировать fd для мониторинга (O(log n))
struct epoll_event ev = {.events = EPOLLIN, .data.fd = sockfd};
epoll_ctl(epfd, EPOLL_CTL_ADD, sockfd, &ev);

// Ждать событий (возвращает только готовые fd)
struct epoll_event events[MAX_EVENTS];
int nready = epoll_wait(epfd, events, MAX_EVENTS, timeout_ms);

// Обрабатываем только готовые — O(готовых), не O(всех)
for (int i = 0; i < nready; i++) {
    handle(events[i].data.fd);
}
```

**Преимущества epoll**:
- `epoll_ctl` — регистрация один раз. Ядро держит структуры.
- `epoll_wait` — возвращает **только готовые** fd. O(количество готовых), не O(всех).
- Нет копирования fd-списка при каждом вызове.
- Работает с сотнями тысяч fd.

---

## epoll: механика

Внутри ядра epoll instance содержит:
- **Red-black tree** всех зарегистрированных fd (для быстрого EPOLL_CTL_ADD/DEL/MOD).
- **Linked list** готовых fd (epoll_wait возвращает их).

Когда сетевая карта получает пакет:
```text
NIC → interrupt → kernel network stack → socket receive buffer
   → callback на epoll entry → добавить в ready list
   → если поток спит в epoll_wait → разбудить
```

---

## Level-triggered vs Edge-triggered

**Level-triggered (LT)** — по умолчанию:
- epoll_wait возвращает fd, пока буфер содержит данные.
- Если не прочитали всё — при следующем epoll_wait снова придёт уведомление.
- Проще в использовании, но больше syscall'ов если читать по частям.

**Edge-triggered (ET)** — флаг `EPOLLET`:
- epoll_wait уведомляет только при **изменении** состояния (новые данные пришли).
- Если данные есть, но не прочитаны — повторного уведомления не будет (пока не придут новые данные).
- Требует читать в цикле до `EAGAIN` за один вызов.
- Меньше syscall'ов, выше throughput, но сложнее в реализации.

Go netpoller использует **edge-triggered** режим.

---

## Go netpoller: как Go строится на epoll

Это ключевое объяснение "как Go обрабатывает 10k соединений одним потоком":

```text
Уровни:
  Горутина:   net.Conn.Read() — блокирует горутину
  Go runtime: если данных нет → парк горутину, вернуть M (OS thread) планировщику
  OS thread:  работает над другими горутинами
  Netpoller:  отдельный тред, вызывает epoll_wait
              когда fd готов → разбудить горутину (G→runnable)
```

```go
// Пользовательский код выглядит как обычный blocking I/O:
conn, _ := net.Accept(l)
go func() {
    buf := make([]byte, 4096)
    n, err := conn.Read(buf)  // ← горутина паркуется здесь (не OS thread!)
    // когда данные пришли — горутина возобновляется
}()
```

Под капотом `conn.Read`:
1. Пробует `read()` syscall.
2. Если `EAGAIN` (данных нет) → регистрирует fd в netpoller (epoll_ctl ADD).
3. Горутина паркуется (`gopark`), M (OS thread) свободен.
4. Когда epoll сигнализирует о готовности → горутина помечается runnable.
5. Scheduler снова запускает горутину → `read()` теперь успешен.

**Результат**: тысячи горутин, ждущих I/O, не занимают OS-потоки. OS-потоки работают только когда есть реальная работа. Один `GOMAXPROCS=N` поток обслуживает тысячи I/O-горутин.

Netpoller инициализируется в `runtime.main()`:
```go
// runtime/netpoll_epoll.go
func netpollinit() {
    epfd = epollcreate1(_EPOLL_CLOEXEC)
    // также создаёт pipe для wakeup netpoller goroutine
}
```

---

## io_uring: следующее поколение

Добавлен в Linux 5.1 (2019). Полностью асинхронный интерфейс через кольцевые буферы в shared memory:

```text
User space             Kernel space
┌──────────────┐      ┌──────────────────┐
│ SQ (submit)  │─────►│ Submission Queue │  ← пишем запросы (read/write/accept/...)
│ ring buffer  │      │ Worker threads   │
├──────────────┤      ├──────────────────┤
│ CQ (complete)│◄─────│ Completion Queue │  ← читаем результаты
│ ring buffer  │      └──────────────────┘
└──────────────┘
```

Преимущества:
- **Zero syscalls** в горячем пути: submit + poll completion без `read()`/`write()`.
- **Zero-copy** I/O: буферы регистрируются заранее, ядро использует напрямую.
- Поддерживает: файлы, сокеты, splice, send file, accept...
- Отлично для **high-throughput disk I/O** (не только сети).

В Go: нет поддержки в stdlib (2024–2025). Есть экспериментальные библиотеки (`github.com/godzie44/go-uring`). В Go 1.23+ добавили некоторую экспериментальную поддержку.

---

## Практика: диагностика fd

```bash
# Сколько fd открыто у процесса
ls /proc/<pid>/fd | wc -l

# Топ процессов по числу открытых fd
lsof | awk '{print $2}' | sort | uniq -c | sort -rn | head -20

# Все сокеты процесса
lsof -p <pid> | grep socket

# Утечка fd: нарастает со временем?
watch -n 1 "ls /proc/<pid>/fd | wc -l"

# Ошибка "too many open files" — найти виновника
strace -e trace=open,openat,socket -p <pid> 2>&1 | head -50

# Системное использование fd
cat /proc/sys/fs/file-nr
```

**fd leak в Go**: частая причина — незакрытые `*os.File`, `net.Conn`, `http.Response.Body`. Проверяй через `defer resp.Body.Close()`. Горутина выходит из scope — файл НЕ закрывается автоматически (в отличие от GC для памяти — finalizer ненадёжен для fd).

---

## Interview-ready answer

File descriptor — целое число, ссылка на ресурс в ядре (файл, сокет, pipe). Три таблицы: fd-таблица (per-process) → open file table (kernel, shared при fork/dup) → inode. Лимит `ulimit -n` (soft/hard): при 10k соединений нужно ≥ 10k fd плюс запас. I/O модели: blocking (один поток на соединение, не масштабируется), select/poll (O(n) сканирование, select ≤ 1024 fd), epoll (O(готовых), red-black tree для регистрации, только готовые fd возвращаются). Go netpoller: горутина паркуется при EAGAIN, M (OS thread) свободен, epoll уведомляет при готовности fd — горутина снова runnable. Поэтому Go обрабатывает 100k соединений с небольшим числом OS-потоков. Edge-triggered epoll: уведомляет только при изменении состояния (новые данные), требует читать до EAGAIN.
