# Execution Tracer: runtime/trace

`runtime/trace` — инструмент, который pprof не может заменить. Он отвечает не на "что медленно", а на "**когда** и **почему**". Незаменим для диагностики latency spike, GC пауз и проблем с планировщиком.

## Содержание

- [pprof vs trace: когда что](#pprof-vs-trace-когда-что)
- [Сбор трейса](#сбор-трейса)
- [go tool trace: интерфейс](#go-tool-trace-интерфейс)
- [Читаем Timeline](#читаем-timeline)
- [Диагностика с помощью trace](#диагностика-с-помощью-trace)
- [User annotations: Task, Region, Log](#user-annotations-task-region-log)
- [Ограничения](#ограничения)
- [Interview-ready answer](#interview-ready-answer)

---

## pprof vs trace: когда что

```
pprof:
  "Какая функция потребляет 40% CPU?"
  "Кто аллоцирует больше всего памяти?"
  → Статистика за период. Хорошо для "что".

runtime/trace:
  "Почему p95 latency 500ms при среднем 10ms?"
  "Почему все горутины встали на 50ms?"
  "Почему один P не используется?"
  → Полная временная линия событий. Хорошо для "когда" и "почему".
```

| | pprof | runtime/trace |
|---|---|---|
| Метод | Семплирование (100 Hz для CPU) | Запись каждого runtime события |
| Overhead | < 5% | 5–30% (зависит от активности) |
| Гранулярность | ~10ms | микросекунды |
| Размер вывода | KB–MB | MB за секунды |
| GC паузы | Не видны | Полностью видны |
| Scheduler events | Не видны | Полностью видны |
| Подходит для | Хотспоты, аллокации | Latency spike, GC туning, schedule issues |

---

## Сбор трейса

### Из HTTP endpoint

```bash
# 5 секунд трейса
curl -o trace.out "http://localhost:6060/debug/pprof/trace?seconds=5"

# Открыть в браузере
go tool trace trace.out
```

**5 секунд** обычно достаточно. За это время файл может вырасти до 50-200 MB для нагруженного сервиса.

### Из кода (для конкретного участка)

```go
import (
    "os"
    "runtime/trace"
)

func main() {
    f, err := os.Create("trace.out")
    if err != nil {
        log.Fatal(err)
    }
    defer f.Close()

    if err := trace.Start(f); err != nil {
        log.Fatal(err)
    }
    defer trace.Stop()

    runMyWorkload()
}
```

### Из тестов

```bash
go test -trace=trace.out -run=TestMyScenario ./...
go tool trace trace.out
```

---

## go tool trace: интерфейс

```bash
go tool trace trace.out
# Запустится HTTP сервер, откроется браузер
```

Главные ссылки на странице:
- **View trace** — основная визуализация (временная шкала)
- **Goroutine analysis** — агрегированная статистика по горутинам
- **Network blocking profile** — где горутины ждали сетевой I/O
- **Synchronization blocking profile** — где ждали mutex/channel
- **Syscall blocking profile** — где ждали syscall
- **Scheduler latency profile** — сколько ждали scheduler

Самое важное — **View trace**.

---

## Читаем Timeline

```
go tool trace → View trace
```

```
Горизонталь = время (мс/мкс)
Вертикаль   = разные ряды (Procs, Goroutines, Heap, GC)
```

### Строки на таймлайне

```
PROCS:
  Proc 0 ─────[G42]────[G17]───────────────────[G42]────→
  Proc 1 ─────────────────────[G91]────[G91]──────────────→
  Proc 2 ─────[G17]────────────────────────────────────────→ (пустой!)
  Proc 3 ─────[G63]───[G24]────────────────────[G24]────→

GC:           ████ (серый прямоугольник = STW пауза)

Heap:         ─────────/──────────────/────────/───
                       ↑ растёт       ↑ GC собрал
```

### Что искать

**GC STW пауза** (серый прямоугольник во всю ширину):
```
██████████████████
   STW = 4.2ms
```
→ Все горутины остановились. Если часто и долго — смотри GOGC, размер heap, аллокации.

**Один P постоянно пустой:**
```
Proc 0 [G1][G2][G3][G4][G5]...
Proc 1 [G6][G7][G8][G9][G10]...
Proc 2                          ← пустой!
Proc 3 [G11][G12]...
```
→ Work imbalance. Возможно горутины не масштабируются или есть глобальная блокировка.

**Длинный промежуток для одной горутины:**
```
G42: ──────────────────────────────[running 50ms]──
```
→ Горутина выполнялась 50ms без preemption. До Go 1.14 это блокировало P, с 1.14 — async preemption через SIGURG через ~10ms.

**Scheduling latency (горутина долго ждёт P):**
```
G42: ──[runnable 8ms wait]──[running]──
           ↑ ждала свободный P
```
→ Все P заняты. Или мало P (GOMAXPROCS), или много CPU-bound горутин.

---

## Диагностика с помощью trace

### Случай 1: Периодические latency spikes

**Симптом:** p99 200ms, хотя median 5ms. Паттерн регулярный.

**В трейсе:** Ищем паузы, коррелирующие со spikes. Скорее всего увидим:
```
GC ─────────────────────████████████────────────→
                         ↑ STW 15ms  ← вот причина!
```

**Диагностика:** Смотрим частоту GC — каждые N секунд? Размер heap в момент GC?

**Fix:**
```go
// Увеличить GOGC — GC реже, больше пиковая память
debug.SetGCPercent(200)

// Или поставить жёсткий лимит (Go 1.19+)
debug.SetMemoryLimit(2 * 1024 * 1024 * 1024)  // 2 GB

// Или уменьшить аллокации (главный fix)
```

### Случай 2: Горутины не параллелятся

**Симптом:** Много горутин, но CPU usage 25% при 4 cores.

**В трейсе:**
```
Proc 0 [G1]────────────────────────────────
Proc 1         [G1]────────────────────────  ← всегда одна горутина!
Proc 2 ─────────────────────────────────────  ← пустые
Proc 3 ─────────────────────────────────────  ← пустые
```

→ Горутины сериализованы (глобальный mutex или channel с одним получателем).

**В Synchronization blocking profile:** `sync.(*Mutex).Lock` с огромным flat — вот где ждут.

### Случай 3: Scheduler delays

**Симптом:** Горутины есть, P не заняты полностью, но latency высокая.

**В трейсе:** Горутины долго в состоянии "runnable" перед тем как начать выполняться.

**Fix:**
```go
// Проверить GOMAXPROCS — не слишком ли мало?
runtime.GOMAXPROCS(0)  // вернуть текущее значение

// В контейнерах:
import _ "go.uber.org/automaxprocs"
```

### Случай 4: Network I/O паузы

**В трейсе:** Переключиться на "Network blocking profile":
```
goroutine 142: 245ms in net.(*conn).Read
goroutine 87:  189ms in net.(*conn).Read
```

→ Горутины долго ждут ответа от upstream. Это не проблема планировщика — это сам upstream медленный. Смотреть SetDeadline, circuit breaker, retry с timeout.

---

## User annotations: Task, Region, Log

Можно добавить собственные аннотации в трейс — это мощный инструмент для понимания бизнес-логики в таймлайне.

```go
import "runtime/trace"

func handleRequest(ctx context.Context, r *Request) {
    // Task — привязать горутину к логической задаче
    ctx, task := trace.NewTask(ctx, "HandleRequest")
    defer task.End()

    // Region — выделить участок кода на таймлайне
    trace.WithRegion(ctx, "Validate", func() {
        validate(r)
    })

    trace.WithRegion(ctx, "DBQuery", func() {
        queryDB(ctx, r.UserID)
    })

    trace.WithRegion(ctx, "Render", func() {
        render(r)
    })

    // Log — добавить событие с сообщением
    trace.Log(ctx, "user_id", fmt.Sprintf("%d", r.UserID))
}
```

В `go tool trace` → **User-defined tasks** появятся именованные блоки, их можно отфильтровать по имени таска.

### Измерение времени конкретной операции

```go
func processWithTrace(ctx context.Context, items []Item) {
    for _, item := range items {
        trace.WithRegion(ctx, "processItem", func() {
            processItem(item)
        })
    }
}
```

→ В трейсе увидишь каждую итерацию как отдельный регион с длительностью.

---

## Ограничения

1. **Overhead**: 5–30% для нагруженного сервиса — не оставляй включённым в production постоянно
2. **Размер файла**: 5 секунд нагруженного сервиса может создать 200+ MB файл
3. **Только для одного процесса**: нет агрегации по нескольким инстансам
4. **Сложность**: трейс требует привычки для чтения, не так прямолинеен как pprof
5. **Не заменяет pprof**: trace показывает "когда", не "сколько CPU потратила функция X"

### Когда НЕ нужен trace

- "Высокий CPU" → достаточно CPU профиля
- "Много памяти" → достаточно heap профиля
- "Какая функция медленная" → достаточно pprof

### Когда trace незаменим

- Latency spike которого нет в CPU профиле
- GC паузы влияют на p99
- Горутины не масштабируются (одни P пустые)
- Нужно понять порядок событий, а не только суммарное время

---

## Interview-ready answer

**"Чем runtime/trace отличается от pprof и когда его использовать?"**

pprof — статистика: семплирует стеки 100 раз в секунду и говорит "функция X набрала 40% семплов". Он отвечает на вопрос "что медленно".

runtime/trace — полная запись всех runtime событий: создание горутин, планирование на P, GC фазы, network I/O, syscalls. Он отвечает на вопрос "когда именно и почему".

**Типичный сценарий для trace:** p99 latency 200ms, хотя CPU профиль выглядит нормально и p50 = 5ms. Это классика GC STW пауз. Собираю 5-секундный трейс, открываю `go tool trace`, ищу серые прямоугольники "Stop the World". Если пауза 15ms каждые несколько секунд — вот и объяснение p99.

Ещё случай: throughput ниже ожидаемого при 4 CPU — смотрю таймлайн Procs. Если 2 из 4 P постоянно пустые — горутины сериализованы через глобальный mutex.

Ограничение: overhead 5-30%, файл растёт быстро. Для production — короткий снапшот (3-5 секунд) под нагрузкой, не постоянно.
