# pprof: инструменты и рабочий процесс

`pprof` — стандартный инструмент профилирования Go. Не внешний, не сторонний — встроен в стандартную библиотеку и `go tool`. Понимание того, как его подключить, какие данные он собирает и как читать вывод — базис для всего остального профилирования.

## Содержание

- [Что такое pprof](#что-такое-pprof)
- [Типы профилей](#типы-профилей)
- [Подключение: net/http/pprof](#подключение-nethttppprof)
- [Сбор профилей: три способа](#сбор-профилей-три-способа)
- [go tool pprof: команды](#go-tool-pprof-команды)
- [Веб-интерфейс: -http флаг](#веб-интерфейс--http-флаг)
- [Flat vs Cum: что значат числа](#flat-vs-cum-что-значат-числа)
- [Разница pprof vs runtime/trace](#разница-pprof-vs-runtimetrace)
- [Interview-ready answer](#interview-ready-answer)

---

## Что такое pprof

pprof — **семплирующий профилировщик**. Он не трассирует каждый вызов, а с заданной частотой "замораживает" выполнение и записывает текущие stack traces всех горутин.

```
Каждые 10ms (CPU профиль):
  snapshot goroutine stacks → агрегировать → итог: "функция X набрала N семплов"
```

Это значит:
- **Низкий overhead** — влияние на производительность минимально (< 5% для CPU профиля)
- **Статистика, не точность** — функция с 1000 семплами занимает ~10x больше CPU чем функция с 100 семплами, но не ровно в 10x
- **Чем дольше запись — тем точнее** — 30 секунд надёжнее 5 секунд

---

## Типы профилей

| Профиль | Endpoint | Что показывает | Включён по умолчанию |
|---|---|---|---|
| `cpu` | `/debug/pprof/profile?seconds=N` | где тратится CPU (семплирование 100 Hz) | при запросе |
| `heap` | `/debug/pprof/heap` | объекты в heap (inuse + alloc) | да |
| `allocs` | `/debug/pprof/allocs` | все аллокации с начала работы | да |
| `goroutine` | `/debug/pprof/goroutine` | stack traces всех горутин прямо сейчас | да |
| `block` | `/debug/pprof/block` | где горутины блокировались | нет — нужен `SetBlockProfileRate` |
| `mutex` | `/debug/pprof/mutex` | где mutex.Lock() ждал | нет — нужен `SetMutexProfileFraction` |
| `threadcreate` | `/debug/pprof/threadcreate` | стеки, создавшие OS threads | да |
| `trace` | `/debug/pprof/trace?seconds=N` | runtime/trace (не pprof-профиль) | при запросе |

---

## Подключение: net/http/pprof

### Отдельный порт (рекомендуется для production)

```go
import (
    "net/http"
    _ "net/http/pprof"  // side-effect: регистрирует /debug/pprof/ handlers
)

func main() {
    // Профилирование на отдельном порту — не светить наружу
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()

    // Основной сервер
    startMainServer()
}
```

После этого доступны:
```
http://localhost:6060/debug/pprof/           — index
http://localhost:6060/debug/pprof/goroutine  — дамп горутин
http://localhost:6060/debug/pprof/heap       — heap профиль
http://localhost:6060/debug/pprof/profile?seconds=30  — CPU профиль (30 секунд)
```

### Включение block и mutex профилей

```go
import "runtime"

func init() {
    // Block profile: записывает каждую блокировку (1 = всё, 0 = выключено)
    // Высокий overhead! Используй только при диагностике
    runtime.SetBlockProfileRate(1)

    // Mutex profile: каждый 10-й конфликтный lock (дробь от 1)
    runtime.SetMutexProfileFraction(10)
}
```

### Только для локальной разработки (без HTTP сервера)

```go
import (
    "os"
    "runtime/pprof"
)

func main() {
    f, _ := os.Create("cpu.prof")
    pprof.StartCPUProfile(f)
    defer pprof.StopCPUProfile()

    // ... код программы
}
```

---

## Сбор профилей: три способа

### Способ 1: через URL напрямую в go tool pprof

```bash
# CPU профиль — 30 секунд записи (самый полезный)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Heap (текущее состояние)
go tool pprof http://localhost:6060/debug/pprof/heap

# Все аллокации с начала
go tool pprof http://localhost:6060/debug/pprof/allocs

# Горутины
go tool pprof http://localhost:6060/debug/pprof/goroutine

# Block profile (если включён)
go tool pprof http://localhost:6060/debug/pprof/block

# Mutex profile (если включён)
go tool pprof http://localhost:6060/debug/pprof/mutex
```

### Способ 2: сохранить файл, потом анализировать

```bash
# Скачать профиль
curl -o cpu.prof "http://localhost:6060/debug/pprof/profile?seconds=30"
curl -o heap.prof "http://localhost:6060/debug/pprof/heap"

# Анализировать позже
go tool pprof cpu.prof
go tool pprof heap.prof
```

### Способ 3: при тестировании

```bash
go test -bench=BenchmarkMyFunc -cpuprofile=cpu.prof -memprofile=mem.prof -benchmem ./...

go tool pprof cpu.prof
go tool pprof mem.prof
```

---

## go tool pprof: команды

После запуска `go tool pprof <profile>` открывается интерактивный REPL:

```
(pprof) help     — список всех команд
```

### Основные команды

```
top [N]          — топ N функций по flat (по умолчанию 10)
top -cum [N]     — топ N функций по cum
list <regex>     — source-level breakdown для функций, совпавших с regex
web              — открыть SVG граф вызовов в браузере (нужен graphviz)
weblist <regex>  — source listing в браузере
svg              — сохранить граф в SVG файл
pdf              — сохранить в PDF
png              — сохранить в PNG
tree             — текстовое дерево вызовов
peek <regex>     — показать callers и callees для функции
```

### Пример сессии

```
$ go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
Fetching profile over HTTP from http://localhost:6060/debug/pprof/profile?seconds=30
...
File: myapp
Type: cpu
...
(pprof) top 15
Showing nodes accounting for 18.42s, 92.53% of 19.91s total
...
      flat  flat%   sum%        cum   cum%
     5.23s 26.27% 26.27%      5.31s 26.68%  regexp.(*Regexp).FindString
     3.11s 15.62% 41.89%      3.11s 15.62%  runtime.mallocgc
     2.44s 12.25% 54.14%     18.90s 94.98%  myapp.processRequest
     ...

(pprof) list processRequest
Total: 19.91s
ROUTINE ======================== myapp.processRequest
     2.44s     18.90s (flat, cum) 94.98% of Total
         .          .     45:func processRequest(r *Request) {
         .          .     46:    for _, item := range r.Items {
     2.44s      5.31s     47:        re.FindString(item.Name)  // ← горячая строка
         .     13.59s     48:        processItem(item)
         .          .     49:    }
         .          .     50:}
```

---

## Веб-интерфейс: -http флаг

Самый удобный способ — открыть flamegraph в браузере:

```bash
# Открыть веб-интерфейс с flamegraph (требует graphviz для SVG/dot)
go tool pprof -http=:6061 cpu.prof
# → откроет http://localhost:6061/ в браузере

# Или сразу из живого сервиса
go tool pprof -http=:6061 http://localhost:6060/debug/pprof/profile?seconds=30
```

В браузере доступны вкладки:
- **Top** — таблица как в REPL
- **Graph** — граф вызовов с весами
- **Flame Graph** — flamegraph (самый наглядный)
- **Source** — аннотированный исходный код
- **Peek** — callers/callees для функции
- **Disasm** — ассемблер (редко нужен)

Flamegraph читается так:
```
Каждый прямоугольник — функция
Ширина — пропорциональна времени (CPU семплам или байтам)
Стопка снизу вверх — call stack (нижние вызывают верхние)
Цвет — не несёт смысла (случайный, для контраста)
```

---

## Flat vs Cum: что значат числа

```
      flat  flat%   sum%        cum   cum%
     5.23s 26.27% 26.27%      5.31s 26.68%  regexp.FindString
     2.44s 12.25% 54.14%     18.90s 94.98%  myapp.processRequest
```

**flat** — время, проведённое **в самой функции** (не в её вызовах)  
**cum** (cumulative) — время **включая все вызываемые функции**

```
processRequest: flat=2.44s, cum=18.90s
→ сама функция работала 2.44s
→ но вызванные ею функции вместе взяли 18.90s (она "владеет" почти всем CPU)

regexp.FindString: flat=5.23s, cum=5.31s
→ функция "листовая" — почти всё время тратит сама, вызовы минимальны
```

**Правило для диагностики:**
- Большой **flat** → этот код сам по себе медленный (алгоритм, системный вызов)
- Большой **cum** при малом **flat** → "дирижёр": проблема в чём-то, что она вызывает; смотреть вниз по стеку

---

## Разница pprof vs runtime/trace

| | pprof | runtime/trace |
|---|---|---|
| Метод | семплирование (~ каждые 10ms) | полная запись событий |
| Overhead | < 5% | 5–30% |
| Гранулярность | статистика по функциям | каждое событие с временем |
| Что показывает | где тратится CPU/память | когда и почему горутины пробуждались, STW паузы |
| Размер вывода | килобайты | мегабайты за секунды |
| Когда использовать | "что медленно" | "почему latency spike", GC паузы, scheduling gaps |

Детально: [05-execution-tracer.md](./05-execution-tracer.md)

---

## Interview-ready answer

**"Как бы ты профилировал медленный Go сервис?"**

Сначала определяю гипотезу по метрикам: высокий CPU, рост памяти или высокая latency при нормальном CPU. Это определяет какой профиль собирать.

Если сервис в production, подключаю `net/http/pprof` на отдельном порту (side-effect import). Собираю профиль через `go tool pprof -http=:6061 http://host:6060/debug/pprof/profile?seconds=30`. Открываю flamegraph в браузере.

На flamegraph ищу **широкие плато** — это функции, где тратится непропорционально много времени. Потом `list <func>` — смотрю на конкретные строки кода с семплами.

Для памяти — heap профиль с `inuse_space` (что сейчас держится) или `alloc_space` (где больше всего аллоцирует). Если нужно сравнить "до нагрузки" и "после" — `-diff_base`.

Для goroutine leak — `/debug/pprof/goroutine?debug=2` в браузере, ищу горутины в состоянии "IO wait" или "chan receive" которых неожиданно много.

pprof не покажет причины latency spike или GC паузы — для этого `runtime/trace`.

**Ключевая мысль:** pprof это не дебаггер — он говорит "где", а не "почему". После нахождения hotspot нужно анализировать код вручную.
