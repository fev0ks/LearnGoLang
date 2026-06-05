# Linux Signals и процессы

Сигналы — основной механизм управления жизненным циклом процессов в Linux. Понимание сигналов обязательно для правильного graceful shutdown в контейнерах.

## Содержание

- [Процессы: fork, exec, exit](#процессы-fork-exec-exit)
- [Таблица сигналов](#таблица-сигналов)
- [PID 1: особый статус](#pid-1-особый-статус)
- [Zombie и orphan процессы](#zombie-и-orphan-процессы)
- [Docker stop sequence](#docker-stop-sequence)
- [Kubernetes terminationGracePeriodSeconds](#kubernetes-terminationgraceperiodseconds)
- [Go: обработка сигналов](#go-обработка-сигналов)
- [SIGTERM vs SIGKILL: почему нельзя всегда использовать SIGKILL](#sigterm-vs-sigkill-почему-нельзя-всегда-использовать-sigkill)
- [SIGHUP: reload без перезапуска](#sighup-reload-без-перезапуска)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

---

## Процессы: fork, exec, exit

Все процессы в Linux образуют дерево. Корень — PID 1 (init / systemd).

**Создание процесса**:
```text
fork()   → создаёт копию родительского процесса (copy-on-write)
exec()   → заменяет образ процесса новой программой
waitpid()→ родитель ждёт завершения ребёнка и получает exit code
```

Типичная последовательность (shell запускает команду):
```text
bash (PID 100)
  │── fork() → bash (PID 200)  ← копия
  │             └── exec("ls") → ls (PID 200)  ← заменил себя
  └── waitpid(200) → ждёт ls
```

**Наследование**: дочерний процесс наследует:
- открытые file descriptors (stdin/stdout/stderr и другие)
- environment variables
- текущую директорию
- signal handlers (кроме некоторых случаев после exec)
- namespaces (если не создаются новые через `clone()`)

---

## Таблица сигналов

| Сигнал | Номер | Default action | Catchable/Blockable | Назначение |
|---|---|---|---|---|
| `SIGHUP`  | 1  | Terminate | Да | Потеря terminal; конвенционально — reload config |
| `SIGINT`  | 2  | Terminate | Да | Ctrl+C в терминале |
| `SIGQUIT` | 3  | Core dump | Да | Ctrl+\\ — завершить с core dump |
| `SIGKILL` | 9  | Kill | **Нет** | Немедленное уничтожение, нельзя поймать |
| `SIGUSR1` | 10 | Terminate | Да | Пользовательский сигнал 1 (приложение определяет) |
| `SIGUSR2` | 12 | Terminate | Да | Пользовательский сигнал 2 |
| `SIGTERM` | 15 | Terminate | Да | Graceful shutdown запрос |
| `SIGCHLD` | 17 | Ignore | Да | Дочерний процесс изменил состояние |
| `SIGSTOP` | 19 | Stop | **Нет** | Приостановить процесс (нельзя поймать) |
| `SIGCONT` | 18 | Continue | Да | Продолжить приостановленный процесс |

Полный список: `kill -l` или `man 7 signal`.

**Посылка сигналов**:
```bash
kill -15 <pid>      # SIGTERM (default)
kill -9 <pid>       # SIGKILL
kill -HUP <pid>     # SIGHUP
kill -s SIGUSR1 <pid>

# Всем процессам группы (отрицательный PID = process group)
kill -15 -<pgid>

# Из контейнера через Docker
docker kill --signal=SIGTERM my-container
docker stop my-container  # SIGTERM, потом SIGKILL через 10s
```

---

## PID 1: особый статус

PID 1 (init) — первый процесс в системе. Он имеет несколько особенностей, отличающих его от обычных процессов.

### Нет default signal handlers

У обычных процессов ядро устанавливает default handler для каждого сигнала (например, SIGTERM → terminate). У PID 1 **default handlers не установлены**. Это значит:

- **SIGTERM → ничего** (если PID 1 явно не обработал).
- **SIGINT → ничего**.
- Только SIGKILL и SIGSTOP работают всегда (их нельзя заблокировать ни для кого).

Это защита от случайного убийства init: если кто-то пошлёт SIGTERM PID 1 — система не упадёт.

**В контейнере это ловушка**: если Go-бинарь запущен как PID 1 (exec form ENTRYPOINT) и не вызывает `signal.Notify(ch, syscall.SIGTERM)` — `docker stop` отправит SIGTERM, Go процесс его **молча проигнорирует**, через 10 секунд придёт SIGKILL.

```go
// Без этого — SIGTERM игнорируется при PID 1
sigs := make(chan os.Signal, 1)
signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
// или современный способ:
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
```

### Reaping orphan processes

Когда процесс завершается, он остаётся в таблице процессов как **zombie** до тех пор, пока родитель не вызовет `wait()`. Если родитель умирает раньше ребёнка — ребёнок становится **orphan** и переприписывается к PID 1.

PID 1 обязан вызывать `wait()` для всех orphan-процессов, иначе таблица процессов заполняется зомби.

Go-приложение как PID 1 **не делает это автоматически**. Если оно запускает child-процессы через `os/exec` и один из них умирает раньше времени — возникает zombie. Решение — `tini`:

```dockerfile
# tini как PID 1, Go-бинарь как дочерний процесс
ENTRYPOINT ["/tini", "--", "/app/server"]
```

Для **чистых Go-сервисов без `os/exec`** — zombie проблема не возникает. tini нужен только при fork.

---

## Zombie и orphan процессы

```text
Zombie процесс:
  Child завершился → запись в таблице процессов сохранена
  Родитель ещё не вызвал wait() → PID занят, ресурсы освобождены
  В ps/top: статус Z (zombie)
  Ресурсов не потребляет, но занимает PID slot

Orphan процесс:
  Родитель завершился первым
  Ядро переприписывает child к PID 1
  PID 1 должен вызвать wait() → убрать zombie когда child завершится
```

```bash
# Посмотреть zombie процессы
ps aux | grep Z

# Посмотреть дерево процессов
pstree -p

# Kто родитель процесса
cat /proc/<pid>/status | grep PPid
```

---

## Docker stop sequence

```text
docker stop my-container
    │
    ├─ 1. Отправляет SIGTERM к PID 1 контейнера
    │
    ├─ 2. Ждёт --stop-timeout (default: 10 секунд)
    │     (можно изменить: docker stop -t 30 my-container)
    │
    └─ 3. Если контейнер ещё жив → SIGKILL
```

Важно: SIGTERM идёт **только к PID 1**. Остальные процессы в контейнере получат сигналы от PID 1 (если он их проксирует) или будут убиты вместе с namespace при смерти PID 1.

Настройка timeout в Dockerfile:
```dockerfile
STOPSIGNAL SIGTERM  # можно изменить сигнал (например, на SIGINT)
```

```bash
# Настройка при запуске
docker run --stop-timeout=30 --stop-signal=SIGTERM my-service
```

---

## Kubernetes terminationGracePeriodSeconds

В Kubernetes последовательность при завершении Pod'а:

```text
1. Pod помечается Terminating
2. Endpoint удаляется из Service (kube-proxy обновляет iptables)
   ↑ это асинхронно — занимает несколько секунд!

3. preStop hook выполняется (если настроен)
   lifecycle:
     preStop:
       exec:
         command: ["sleep", "5"]  # дать время на drain

4. SIGTERM отправляется в PID 1 контейнера

5. Ждёт terminationGracePeriodSeconds (default: 30s)
   Параллельно с шагами 2-4.

6. Если Pod ещё жив → SIGKILL
```

Проблема race condition: между шагом 1 и шагом 2 новые запросы могут ещё приходить на Pod (kube-proxy не успел обновить). Если Go-сервис начал shutdown сразу при SIGTERM — он не обработает эти запросы.

Решение:
```go
<-ctx.Done()  // получили SIGTERM

// Дать kube-proxy время убрать endpoint
time.Sleep(5 * time.Second)  // или через preStop hook

// Теперь безопасно закрывать
srv.Shutdown(shutdownCtx)
```

Или через `preStop` hook с `sleep 5` — это предпочтительнее, не загрязняет код инфраструктурной логикой.

---

## Go: обработка сигналов

### signal.NotifyContext (Go 1.16+, предпочтительный способ)

```go
func main() {
    // ctx отменяется при получении SIGTERM или SIGINT
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGTERM,
        syscall.SIGINT,
    )
    defer stop()  // освободить ресурсы, сбросить signal handling

    srv := startServer(ctx)

    // Ждём сигнала
    <-ctx.Done()
    slog.Info("shutdown signal received", "signal", ctx.Err())

    // Grace period для in-flight запросов
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        slog.Error("shutdown error", "err", err)
    }
    slog.Info("shutdown complete")
}
```

### signal.Notify (низкоуровневый способ)

```go
sigs := make(chan os.Signal, 1)
// Буфер 1 обязателен: если goroutine не читает канал сразу, сигнал не теряется

signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
// Важно: без вызова Notify — сигналы обрабатываются default handler'ом (terminate)

go func() {
    for sig := range sigs {
        switch sig {
        case syscall.SIGTERM, syscall.SIGINT:
            gracefulShutdown()
            os.Exit(0)
        case syscall.SIGHUP:
            reloadConfig()
        }
    }
}()
```

### Сигналы в Go runtime

Go runtime сам использует некоторые сигналы внутренне:
- `SIGURG` (real-time): используется для async preemption горутин (Go 1.14+).
- `SIGSEGV`, `SIGBUS`, `SIGFPE`: перехватываются runtime для panic recovery.
- `SIGPIPE`: при записи в закрытый pipe — runtime преобразует в ошибку.

Не регистрируй обработчик на `SIGURG` — это сломает горутинный планировщик.

---

## SIGTERM vs SIGKILL: почему нельзя всегда использовать SIGKILL

SIGKILL немедленно убивает процесс. Нет cleanup, нет flush buffers, нет commit транзакций.

Последствия SIGKILL для типичного Go-сервиса:
- **In-flight HTTP запросы** прерываются → клиенты получают connection reset.
- **Открытые DB транзакции** откатываются (если DB это поддерживает), но lock может держаться секунды.
- **Write-ahead log** (PostgreSQL, MySQL) незаконченные транзакции откатываются при рестарте.
- **Kafka producer** с batch buffering теряет неотправленные сообщения.
- **File writes** без fsync теряют данные при незаконченных write.

SIGTERM даёт шанс:
- завершить in-flight запросы;
- flush буферы (Kafka producer, logger);
- закрыть DB connections корректно;
- освободить distributed locks (Redis, etc.).

Всегда используй SIGTERM с разумным timeout (10–30s). SIGKILL — только как последний шаг.

---

## SIGHUP: reload без перезапуска

По исторической конвенции SIGHUP означает "перечитай конфигурацию". Используется в nginx, sshd, многих daemon-процессах.

```bash
# nginx: reload config без downtime
kill -HUP $(pgrep -f "nginx: master")
# или
nginx -s reload
```

В Go-сервисах можно реализовать hot config reload:

```go
sigs := make(chan os.Signal, 1)
signal.Notify(sigs, syscall.SIGHUP)

go func() {
    for range sigs {
        slog.Info("SIGHUP received, reloading config")
        if err := cfg.Reload(); err != nil {
            slog.Error("config reload failed", "err", err)
            continue
        }
        slog.Info("config reloaded successfully")
    }
}()
```

В Kubernetes hot reload через SIGHUP менее актуален — лучше пересоздать Pod с новым ConfigMap. Но для длинноживущих daemon процессов вне k8s — полезно.

---

## Типичные ошибки

- **Shell form ENTRYPOINT**: `CMD /app/server` → sh становится PID 1, SIGTERM не доходит до Go.
- **Не вызван `signal.Notify`**: при PID 1 SIGTERM молча игнорируется — процесс висит до SIGKILL.
- **Буфер канала = 0**: `make(chan os.Signal)` — если горутина не читает сразу, сигнал теряется. Всегда `make(chan os.Signal, 1)`.
- **Нет graceful timeout**: shutdown может висеть бесконечно при зависшем in-flight запросе. Всегда `context.WithTimeout` для `Shutdown()`.
- **Игнорировать `ctx.Done()`** в downstream вызовах: запрос отменён (клиент отключился), но handler продолжает работу, тратит ресурсы. Пробрасывай context.
- **`os.Exit()` вместо graceful shutdown**: `os.Exit()` не запускает `defer`, не завершает goroutines. Только после полного shutdown.
- **Подписка на `SIGKILL` или `SIGSTOP`**: `signal.Notify(ch, syscall.SIGKILL)` — молча игнорируется (эти сигналы нельзя поймать).

---

## Interview-ready answer

SIGTERM — запрос на завершение, можно поймать и обработать gracefully. SIGKILL — немедленное уничтожение ядром, нельзя поймать. `docker stop` посылает SIGTERM, ждёт timeout (10s), затем SIGKILL. PID 1 особый: нет default signal handlers, SIGTERM игнорируется если явно не обработан. В Go: `signal.NotifyContext` или `signal.Notify(ch, syscall.SIGTERM)` — обязательно для корректного shutdown при запуске как PID 1. Zombie — завершившийся процесс, чей родитель не вызвал `wait()`. Orphan — процесс, чей родитель умер; переприписывается к PID 1, который должен их reap. В Kubernetes между SIGTERM и остановкой трафика есть race condition — нужен sleep 5s (preStop hook) перед началом shutdown.
