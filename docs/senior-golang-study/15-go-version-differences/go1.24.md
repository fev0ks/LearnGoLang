# Go 1.24

Релиз: февраль 2025.

Главная идея релиза: заметный шаг в сторону более зрелого toolchain и runtime, без массовых ломающих изменений в языке.

## Сводная таблица изменений

| Категория | Изменение | Влияние |
|-----------|-----------|---------|
| Язык | Generic type aliases | Меньше копипасты при обёртке generic-API |
| Tooling | `tool` directive в `go.mod` | Замена `tools.go` паттерна |
| Tooling | `go build -json`, `GOCACHEPROG` | Лучшая интеграция с CI и кэш-прокси |
| Runtime | Swiss Tables для встроенных `map` | ~2-3% снижение CPU overhead |
| Stdlib | `os.Root` / `os.OpenRoot` | Безопасный sandbox-доступ к файлам |
| Stdlib | `testing.B.Loop` | Надёжные бенчмарки без лишнего шума |
| Stdlib | `runtime.AddCleanup` | Гибкая замена `SetFinalizer` |
| Stdlib | FIPS 140-3 | Соответствие требованиям безопасности |

## Что изменилось

### Язык

Go 1.24 полностью поддерживает generic type aliases. Это убирает часть ограничений при проектировании API и оберток поверх generic-типов. На собеседовании важно уметь объяснить разницу между `type T = X` и `type T X`, особенно в generic-коде.

```go
// До 1.24 — нельзя было создать generic alias
// type Ptr[T any] = *T  // ошибка компиляции

// Go 1.24+
type Ptr[T any] = *T
type Slice[T any] = []T
type Map[K comparable, V any] = map[K]V

// Полезно для обёрток над generic API без копирования типов
type Result[T any] = either.Either[error, T]
```

`type Ptr[T any] = *T` — это alias: `Ptr[int]` и `*int` один и тот же тип. `type Ptr[T any] *T` — это определение нового типа, несовместимого с `*int`. Разница критична при проектировании публичных API.

### Tooling и `go` command

В `go.mod` появился `tool` directive. Это заменяет старый паттерн с `tools.go` и blank imports для фиксации зависимостей инструментов. Появились `go get -tool`, meta-pattern `tool`, а также кеширование `go run` и нового поведения `go tool`. `go build` и `go install` получили `-json`, что упрощает интеграцию со своими build UI, CI и анализаторами. `GOCACHEPROG` вышел из режима эксперимента и позволяет вынести бинарный/test cache в отдельный процесс по JSON-протоколу.

**До 1.24 — паттерн `tools.go`:**

```go
// tools.go
//go:build tools
package tools

import (
    _ "github.com/golang/mock/mockgen"
    _ "golang.org/x/tools/cmd/stringer"
    _ "github.com/golangci/golangci-lint/cmd/golangci-lint"
)
```

**Go 1.24 — `tool` directive в `go.mod`:**

```
module myapp

go 1.24

require (
    github.com/golang/mock v1.6.0
    golang.org/x/tools v0.19.0
)

tool (
    github.com/golang/mock/mockgen
    golang.org/x/tools/cmd/stringer
)
```

```bash
# Запуск инструмента через go tool:
go tool mockgen -source=service.go -destination=mock_service.go

# Добавление инструмента:
go get -tool github.com/golangci/golangci-lint/cmd/golangci-lint
```

Преимущество: инструменты зафиксированы в `go.mod`, не нужен отдельный файл с build tag, `go tool` знает о них без дополнительной обёртки.

### Runtime и производительность

Runtime получил несколько оптимизаций с усредненным снижением CPU overhead примерно на 2-3%. Встроенные `map` перешли на реализацию, основанную на Swiss Tables. Улучшились аллокации маленьких объектов и внутренняя реализация runtime mutex. Для performance-sensitive систем это означает, что после апгрейда стоит перепроверить профили, а не только тесты корректности.

Swiss Tables — хэш-таблица с открытой адресацией и SIMD-ускоренным поиском по метаданным. Улучшение проявляется в workload'ах с интенсивным чтением/записью карт — это значительная часть типичных Go-сервисов.

### Standard library

#### `os.Root` и `os.OpenRoot`

`os.Root` и `os.OpenRoot` добавили directory-scoped filesystem access. Это полезно для safer file access: операции не должны выйти за пределы корня даже через symlink escape.

```go
// Проблема: path traversal через symlink или ../..
func readUserFile(base, name string) ([]byte, error) {
    path := filepath.Join(base, name) // небезопасно: name может быть "../../etc/passwd"
    return os.ReadFile(path)
}

// Go 1.24: os.Root — filesystem sandbox
func readUserFileSafe(base, name string) ([]byte, error) {
    root, err := os.OpenRoot(base)
    if err != nil {
        return nil, err
    }
    defer root.Close()
    // name не может выйти за пределы base, даже через symlink
    f, err := root.Open(name)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    return io.ReadAll(f)
}
```

#### `testing.B.Loop`

`testing.B.Loop` делает benchmark-код проще и надежнее, чем ручной цикл по `b.N`.

```go
// До 1.24 — ручной цикл по b.N, компилятор мог оптимизировать тело
func BenchmarkOld(b *testing.B) {
    for i := 0; i < b.N; i++ {
        result = doWork() // компилятор может убрать если result не используется
    }
}

// Go 1.24 — b.Loop() надёжнее
func BenchmarkNew(b *testing.B) {
    for b.Loop() {
        result = doWork()
    }
}
```

`b.Loop()` гарантирует, что тело цикла не будет выброшено компилятором как dead code, и корректно учитывает setup-фазу вне цикла при подсчёте итераций.

#### `runtime.AddCleanup`

`runtime.AddCleanup` дает более безопасную и гибкую альтернативу `runtime.SetFinalizer`.

```go
// SetFinalizer — проблемы: привязан к GC циклу, нельзя несколько на один объект
type Resource struct{ handle int }

func NewResourceOld() *Resource {
    r := &Resource{handle: openHandle()}
    runtime.SetFinalizer(r, func(r *Resource) { closeHandle(r.handle) })
    return r
}

// AddCleanup — Go 1.24: более гибкий
func NewResource() *Resource {
    r := &Resource{handle: openHandle()}
    // cleanup получает handle (int), не *Resource — нет цикличной ссылки
    runtime.AddCleanup(r, closeHandle, r.handle)
    return r
}
// Можно вызвать несколько раз на один объект
// Cleanup не вызывается пока объект достижим
```

Ключевые отличия от `SetFinalizer`: можно зарегистрировать несколько cleanup на один объект; cleanup получает произвольный аргумент, а не сам объект — это устраняет риск accidental resurrection объекта через финализатор.

#### FIPS 140-3

В релизе также появился набор механизмов для FIPS 140-3 compliance. Это позволяет использовать Go в средах с обязательной сертификацией криптографических модулей — актуально для финансовых и государственных систем.

## Что это меняет на практике

- зависимости вроде `stringer`, `mockgen`, `golangci-lint`, `wire` теперь можно хранить в `go.mod` без `tools.go`;
- команды CI могут читать структурированный build output вместо парсинга текста;
- benchmark-ы проще писать так, чтобы компилятор не оптимизировал тело теста слишком агрессивно;
- `os.Root` полезен в коде, где есть риск path traversal или небезопасной работы с архивами и пользовательскими путями.

## Что спросить себя перед апгрейдом

- есть ли в репозитории legacy-паттерн `tools.go`, который пора убрать;
- есть ли собственные CI-интеграции, которым выгоден `go build -json`;
- есть ли код, который опирается на старые профили поведения `map` или unsafe-допущения;
- нужен ли проекту более безопасный доступ к файловой системе через `os.Root`.

## Что могут спросить на интервью

- почему `tool` directive лучше, чем `tools.go`;
- почему `os.Root` безопаснее обычного `os.Open` по пользовательским путям;
- когда `runtime.AddCleanup` лучше finalizer'ов, а когда лучше вообще без обоих механизмов;
- почему после апгрейда runtime нужно смотреть perf-профили, даже если API языка почти не поменялось.

## Источники

- [Go 1.24 Release Notes](https://go.dev/doc/go1.24)
- [Go Release History](https://go.dev/doc/devel/release)
