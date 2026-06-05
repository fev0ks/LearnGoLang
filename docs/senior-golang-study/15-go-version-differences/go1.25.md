# Go 1.25

Релиз: август 2025.

Главная идея релиза: акцент на runtime-поведение в контейнерах, диагностику и более сильный набор инструментов для concurrent/testing задач.

## Краткая сводка изменений

| Категория | Изменение | Влияние |
|-----------|-----------|---------|
| Runtime | Container-aware `GOMAXPROCS` через cgroup CPU bandwidth | Сервисы в Kubernetes больше не переиспользуют лишние OS-треды |
| Runtime | Динамическое обновление `GOMAXPROCS` при изменении лимитов | Не нужен рестарт процесса при изменении cgroup limits |
| Runtime | Experimental Green Tea GC (`GOEXPERIMENT=greenteagc`) | Потенциально ниже GC latency при большом heap |
| Observability | `runtime/trace.FlightRecorder` | Лёгкий кольцевой буфер trace без постоянного overhead |
| Testing | `testing/synctest` GA | Детерминированное тестирование concurrent кода с виртуальным временем |
| Stdlib | Experimental `encoding/json/v2` | Строже по умолчанию, быстрее decoding |
| Compiler | Исправлен nil check bug (Go 1.21-1.24) | Код, который "случайно работал", может начать паниковать |
| Tooling | `go build -asan` включает leak detection | Автоматическое обнаружение утечек памяти при выходе |
| Tooling | `go doc -http` поднимает локальный doc server | Документация прямо из CLI без godoc |
| Tooling | `go.mod` поддерживает `ignore` directive | Проще исключить legacy-директории из `./...` |

## Что изменилось

### Язык

- Языковых изменений, влияющих на поведение программ, в Go 1.25 нет.
- Это хороший пример релиза, где главная ценность не в синтаксисе, а в runtime, tooling и stdlib.

### Tooling и `go` command

- `go build -asan` теперь по умолчанию включает leak detection на выходе процесса.
- В дистрибутиве стало меньше prebuilt tool binaries: редкие инструменты будут собираться `go tool` по мере необходимости.
- В `go.mod` появился `ignore` directive для директорий, которые `go` command должен пропускать при `./...` и похожих pattern matches.
- Появился `go doc -http`, который поднимает локальный documentation server.
- `go version -m -json` упрощает машинный разбор embedded build info в бинарях.

### Runtime и observability

**Container-aware GOMAXPROCS**

До Go 1.25 runtime устанавливал `GOMAXPROCS` равным числу CPU хоста, игнорируя cgroup CPU limits. Контейнер с квотой в 500m CPU запускался с `GOMAXPROCS=32` на 32-ядерном хосте, что приводило к избыточному числу OS-тредов, CPU throttling и неэффективной работе GC и scheduler.

```go
// До 1.25: GOMAXPROCS = число CPU хоста, игнорирует cgroup limits
// Контейнер с limits: "500m" CPU запускался с GOMAXPROCS=32 (хост имеет 32 cores)
// → GC и scheduler работали неэффективно

// Go 1.25: runtime читает cgroup CPU bandwidth limit автоматически
// Контейнер с limits: "500m" → GOMAXPROCS ≈ 1 (0.5 CPU → округление вверх)
// Контейнер с limits: "2000m" → GOMAXPROCS = 2
// Обновляется динамически при изменении лимитов

// Проверить текущее значение:
fmt.Println("GOMAXPROCS:", runtime.GOMAXPROCS(0))

// Старый workaround (uber-go/automaxprocs) теперь менее нужен:
// import _ "go.uber.org/automaxprocs"
```

Runtime также умеет периодически обновлять `GOMAXPROCS`, если лимиты или доступные CPU изменились во время жизни процесса. Это важно для Go-сервисов в Kubernetes: поведение по умолчанию стало ближе к реальным CPU limits контейнера.

**FlightRecorder**

Полный `runtime/trace` имеет overhead 5-15% и пишет всё подряд. `FlightRecorder` — лёгкая кольцевая запись: держит последние N секунд в памяти и позволяет выгрузить их при инциденте.

```go
import "runtime/trace"

func setupFlightRecorder() *trace.FlightRecorder {
    fr := trace.NewFlightRecorder()
    fr.SetPeriod(10 * time.Second) // хранить последние 10 секунд
    fr.Start()
    return fr
}

// При инциденте: сохранить последние N секунд
func captureOnIncident(fr *trace.FlightRecorder, w io.Writer) error {
    return fr.WriteTo(w)
}

// В HTTP handler для on-demand capture:
http.HandleFunc("/debug/trace/snapshot", func(w http.ResponseWriter, r *http.Request) {
    if err := fr.WriteTo(w); err != nil {
        http.Error(w, err.Error(), 500)
    }
})
```

Такой подход позволяет всегда иметь свежий trace-буфер без постоянного overhead, и снимать его точечно — например, при spike latency или при первом сигнале об ошибке.

Появился также experimental Green Tea GC через `GOEXPERIMENT=greenteagc`. Изменился текст unhandled panic при recover+repanic, а на Linux runtime теперь умеет помечать anonymous mappings более информативными именами.

### Compiler и поведение кода

В Go 1.21-1.24 существовал compiler bug: nil check мог откладываться позже, чем должен. Код, который разыменовывал указатель до проверки на `nil`, мог не паниковать в момент разыменования.

```go
// Go 1.21-1.24: компилятор мог откладывать nil check
func process(p *Payload) string {
    result := p.Value  // в 1.21-1.24 мог не паниковать здесь...
    if p == nil {
        return ""
    }
    return result     // ...а здесь или вообще не паниковать
}

// Go 1.25: корректное поведение — паника сразу на p.Value если p == nil
// Если код "случайно работал" → после апгрейда можно получить новые паники
```

Это не regression релиза, а устранение некорректного поведения. Код, который раньше "случайно работал", в Go 1.25 может начать корректно падать с nil pointer panic.

### Standard library

**testing/synctest (GA)**

`testing/synctest` стал general availability. Пакет даёт изолированный bubble с виртуализированным временем: горутины внутри bubble блокируются на `time.Sleep` или channel без реального ожидания, а `synctest.Wait()` продвигает виртуальные часы вперёд.

```go
// Проблема: тестирование time-based concurrent кода обычно медленное или flaky
func TestWithTimeout_Flaky(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    // sleep делает тест медленным и потенциально flaky в CI
    time.Sleep(200 * time.Millisecond)
    assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

// Go 1.25: testing/synctest — изолированный bubble с виртуальным временем
func TestWithTimeout_Fast(t *testing.T) {
    synctest.Run(func() {
        ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
        defer cancel()
        // Виртуальное время — реального sleep нет, тест мгновенный
        synctest.Wait() // продвинуть время пока все горутины не заблокируются
        if ctx.Err() != context.DeadlineExceeded {
            t.Fatal("expected deadline exceeded")
        }
    })
}
```

Для команд с большим количеством flaky или медленных concurrent-тестов это значительное улучшение.

**encoding/json/v2 (experimental)**

Появился experimental `encoding/json/v2` и низкоуровневый `encoding/json/jsontext`. При `GOEXPERIMENT=jsonv2` стандартный `encoding/json` использует новую реализацию, где decoding заметно быстрее во многих сценариях.

```go
// json/v2: строже по умолчанию
// В v1: unknown fields молча игнорируются
// В v2: unknown fields → ошибка (или явно DisallowUnknownMembers: false)

// Включить эксперимент: GOEXPERIMENT=jsonv2
// Тогда import "encoding/json" использует v2 реализацию
```

Ключевые отличия v2: ключи JSON сравниваются case-sensitive по умолчанию, неизвестные поля в JSON приводят к ошибке декодирования (в v1 молча игнорировались), производительность decoding выше за счёт новой внутренней реализации.

## Что это меняет на практике

- в Kubernetes можно реже тянуться к внешним библиотекам для автоматической настройки `GOMAXPROCS`;
- для редких production-инцидентов trace можно собирать точечно, а не держать тяжелый continuous tracing;
- команды, у которых много flaky/сложных concurrent tests, получают сильный новый инструмент через `testing/synctest`;
- перед апгрейдом на 1.25 нужно прогнать тесты на скрытые nil dereference, которые раньше маскировались compiler bug.

## Что проверить перед апгрейдом

- нет ли сервисов, где логика неявно полагалась на старый default `GOMAXPROCS`;
- не ломаются ли интеграции, которые ожидают старое panic output;
- нет ли "случайно работающего" кода с обращением к результату до проверки `err` или до проверки на `nil`;
- есть ли смысл экспериментально погонять сервисы с `GOEXPERIMENT=greenteagc` или `GOEXPERIMENT=jsonv2`.

## Что могут спросить на интервью

- как container-aware `GOMAXPROCS` влияет на CPU throttling и throughput в Kubernetes;
- чем `FlightRecorder` лучше постоянной записи полного runtime trace;
- чем `testing/synctest` полезнее обычных sleep-based тестов;
- почему исправление compiler bug может проявиться как новый panic после апгрейда.

## Источники

- [Go 1.25 Release Notes](https://go.dev/doc/go1.25)
- [Go Release History](https://go.dev/doc/devel/release)
