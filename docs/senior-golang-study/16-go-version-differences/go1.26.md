# Go 1.26

Релиз: февраль 2026.

Главная идея релиза: следующий шаг после 1.25, где experimental идеи начинают становиться default, а tooling сильнее помогает с модернизацией кода.

## Обзор изменений

| Категория | Изменение | Влияние |
|---|---|---|
| Язык | `new(expr)` — создание указателя с инициализацией | Меньше временных переменных в struct literal |
| Язык | Self-referential generic constraints | Более выразительные самотипизированные интерфейсы |
| Tooling | `go fix` перестроен на analysis framework | Массовые автоматические миграции API |
| Tooling | `go mod init` пишет `go 1.25.0` по умолчанию | Улучшение совместимости новых модулей |
| Tooling | `cmd/doc` и `go tool doc` удалены | Стандартная замена — `go doc` |
| Runtime | Green Tea GC включён по умолчанию | Снижение GC latency на heavy нагрузках |
| Runtime | cgo overhead снижен на ~30% | Ускорение сервисов с C-библиотеками |
| Runtime | Randomization heap base address | Security hardening на 64-bit платформах |
| Debugging | Experimental `goroutineleak` pprof profile | Обнаружение утечек горутин в CI и production |
| Stdlib | `runtime/secret` (experimental) | Надёжное стирание секретов из памяти |
| Stdlib | `crypto/hpke` по RFC 9180 | Post-quantum hybrid KEMs out of the box |
| Platform | `windows/arm` 32-bit удалён | Нужна миграция на `windows/arm64` |
| Platform | `linux/riscv64` race detector | Полноценная поддержка race detection на RISC-V |

## Что изменилось

### Язык

Встроенная функция `new` теперь принимает выражение: можно писать `new(expr)`, сразу создавая указатель на значение выражения. Это особенно удобно для optional pointer fields, например в `encoding/json` или protobuf-моделях.

```go
// До 1.26: new() принимал только тип
p := new(int)
*p = 42

// Go 1.26: new(expr) — создаёт указатель сразу с инициализацией
p := new(42)       // *int со значением 42
s := new("hello")  // *string

// Особенно удобно для optional полей в struct literal:
type Config struct {
    Timeout *time.Duration
    Debug   *bool
}

// До 1.26:
d := 5 * time.Second
b := true
cfg := Config{Timeout: &d, Debug: &b}

// Go 1.26:
cfg := Config{
    Timeout: new(5 * time.Second),
    Debug:   new(true),
}
```

Снято ограничение на self-reference generic type в списке type parameters constraint'а. Это делает generic constraints выразительнее и полезнее для самотипизированных интерфейсов.

```go
// До 1.26: нельзя было использовать T в его же constraint
type Ordered[T Ordered[T]] interface { // ошибка в 1.25
    Less(T) bool
}

// Go 1.26: работает
type Comparable[T Comparable[T]] interface {
    CompareTo(T) int
}

type Temperature struct{ celsius float64 }
func (t Temperature) CompareTo(other Temperature) int {
    if t.celsius < other.celsius { return -1 }
    if t.celsius > other.celsius { return 1 }
    return 0
}
// Temperature теперь удовлетворяет Comparable[Temperature]
```

### Tooling и `go` command

`go fix` фактически перезапущен как платформа modernizers. Он теперь строится на том же analysis framework, что и `go vet`, и рассчитан на безопасные автоматические миграции к современным idioms и API.

```bash
# До 1.26: go fix — ручные fixers, почти не развивался
# Go 1.26: построен на analysis framework (как go vet)

# Доступные modernizers:
go fix -fix=modernize ./...     # применить все безопасные modernizers
go fix -fix=stdversion ./...    # обновить go directive до актуальной версии

# Примеры автоматических миграций:
# - заменяет sort.Slice на slices.Sort где возможно
# - заменяет strings.Index на strings.Contains для bool-результата
# - обновляет устаревшие API до современных эквивалентов
```

`go mod init` теперь по умолчанию пишет более низкую версию Go в новый `go.mod`. На toolchain `1.26.x` новый модуль по умолчанию получит `go 1.25.0`, а не `go 1.26.0`.

`cmd/doc` и `go tool doc` удалены; стандартная замена теперь `go doc`.

В `pprof -http` flame graph стал default view.

### Runtime и debugging

Green Tea GC, который в 1.25 был экспериментом, в 1.26 включен по умолчанию. Для GC-heavy нагрузок релиз обещает заметное снижение GC overhead, а на новых amd64 CPU ожидается дополнительный выигрыш.

```go
// Go 1.25: GOEXPERIMENT=greenteagc (экспериментально)
// Go 1.26: включён по умолчанию

// Что изменилось архитектурно:
// - GC работает с более мелкими регионами памяти (не весь heap)
// - Снижена stop-the-world latency
// - Особенно заметно на GC-intensive нагрузках

// Проверить GC stats:
var stats runtime.MemStats
runtime.ReadMemStats(&stats)
fmt.Printf("GC pause (last): %v\n", time.Duration(stats.PauseNs[(stats.NumGC+255)%256]))
fmt.Printf("GC cycles: %d\n", stats.NumGC)

// Для отладки: GODEBUG=gccheckmark=1 для consistency check
// GODEBUG=gctrace=1 для вывода каждого GC цикла
```

Базовый overhead `cgo` вызовов снижен примерно на 30%. Это важно для сервисов с C-библиотеками: SQLite, librdkafka, BoringSSL.

```go
// cgo вызовы стали быстрее ~на 30% в Go 1.26
// Это важно для сервисов с C-библиотеками: SQLite, librdkafka, BoringSSL

// Benchmark до/после апгрейда:
func BenchmarkCGOCall(b *testing.B) {
    for b.Loop() {
        C.some_c_function()
    }
}
// До 1.26: ~50ns/op
// После 1.26: ~35ns/op (примерные числа)
```

На 64-битных платформах включена randomization heap base address как security hardening.

Появился experimental leak profile `goroutineleak` в `runtime/pprof` и endpoint `/debug/pprof/goroutineleak`.

```go
// Новый pprof endpoint в Go 1.26
import "net/http/pprof"  // регистрирует /debug/pprof/goroutineleak

// Или вручную:
import "runtime/pprof"

func dumpGoroutineLeaks(w io.Writer) error {
    p := pprof.Lookup("goroutineleak")
    if p == nil {
        return errors.New("goroutineleak profile not available")
    }
    return p.WriteTo(w, 1)
}

// Показывает горутины, которые:
// - живут дольше порогового времени
// - заблокированы на одном и том же месте
// Не ловит: горутины завершившиеся до снапшота
```

### Standard library

Появился experimental `runtime/secret` для более надежного стирания временных данных, связанных с секретами.

```go
// Проблема: секреты (пароли, ключи) могут остаться в памяти
// после освобождения — GC не гарантирует немедленное обнуление

// Go 1.26 experimental: runtime/secret
import "runtime/secret"

func processPassword(pwd string) {
    s := secret.Make([]byte(pwd))
    defer s.Wipe() // надёжно затирает память при defer

    // работаем с паролем через s.Bytes()
    hash := bcrypt.GenerateFromPassword(s.Bytes(), bcrypt.DefaultCost)
    _ = hash
    // После defer s.Wipe() — память обнулена
}
```

Новый пакет `crypto/hpke` добавляет Hybrid Public Key Encryption по RFC 9180, включая post-quantum hybrid KEMs.

Появился experimental `simd/archsimd` для architecture-specific SIMD на amd64.

`bytes.Buffer.Peek` позволяет посмотреть следующие `n` байт без продвижения указателя чтения.

В crypto-пакетах усиливается тренд на более безопасное использование randomness: часть API теперь игнорирует пользовательский random source и берет криптографически безопасный источник.

### Platform и compatibility

Go 1.26 требует для bootstrap минимум Go 1.24.6.

`windows/arm` 32-bit удален.

`linux/riscv64` получил поддержку race detector.

Go 1.26 — последний релиз с поддержкой macOS 12 Monterey.

## Что это меняет на практике

- `go fix` становится реальным инструментом массовой миграции кодовой базы, а не историческим артефактом;
- если сервис сильно упирается в GC или делает много `cgo` вызовов, апгрейд до 1.26 стоит мерить отдельными benchmark/profiling прогонами;
- leak detection для горутин становится ближе к production use, особенно в CI и на сервисах со сложной конкурентностью;
- security-команды и криптографический код получают новые примитивы и более безопасные defaults.

## Что проверить перед апгрейдом

- нет ли внутренних скриптов или IDE-интеграций, которые все еще вызывают `go tool doc`;
- устраивает ли команду новый default `go` version в `go mod init`;
- есть ли сервисы, которым полезно включить и протестировать `goroutineleak` profile;
- не используется ли кастомный источник randomness там, где новое crypto API теперь его игнорирует.

## Что могут спросить на интервью

- зачем менять `go fix`, если есть линтеры и ручные refactor'ы;
- почему lower default version в `go mod init` полезен для совместимости модулей;
- чем Go 1.26 отличается от 1.25 по состоянию Green Tea GC;
- какие типы goroutine leaks можно найти новым профилем, а какие он не поймает.

## Источники

- [Go 1.26 Release Notes](https://go.dev/doc/go1.26)
- [Go Release History](https://go.dev/doc/devel/release)
