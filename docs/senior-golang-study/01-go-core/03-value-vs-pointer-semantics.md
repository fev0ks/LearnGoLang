# Value vs Pointer Semantics

Влияет на API design, race conditions, GC pressure и читаемость. Нужно понимать не только когда что выбрать, но и какие конкретные баги возникают при неправильном выборе.

## Содержание

- [Базовая идея](#базовая-идея)
- [Что копируется, а что разделяется](#что-копируется-а-что-разделяется)
- [Когда value semantics](#когда-value-semantics)
- [Когда pointer semantics](#когда-pointer-semantics)
- [Классические production-баги](#классические-production-баги)
- [Практические правила](#практические-правила)
- [Interview-ready answer](#interview-ready-answer)

## Базовая идея

В Go всё передаётся по значению — но "значение" может быть маленьким header, который **ссылается** на общие данные:

```go
// struct: полная копия
type Point struct{ X, Y int }
a := Point{1, 2}
b := a           // независимая копия
b.X = 99
fmt.Println(a.X) // 1 — a не изменился

// slice: копия header, но shared underlying array
s1 := []int{1, 2, 3}
s2 := s1           // копируется header: {ptr, len, cap}
s2[0] = 99
fmt.Println(s1[0]) // 99 — s1 и s2 смотрят на один массив!

// map: копируется runtime descriptor (по сути pointer)
m1 := map[string]int{"a": 1}
m2 := m1
m2["a"] = 99
fmt.Println(m1["a"]) // 99 — одна и та же map
```

## Что копируется, а что разделяется

| Тип | При присваивании/передаче | Shared state? |
|-----|--------------------------|---------------|
| `int`, `float64`, `bool` | полная копия | нет |
| `struct` | полная копия всех полей | только если поля-ссылки |
| `[N]T` (array) | полная копия | нет |
| `[]T` (slice) | копия header {ptr, len, cap} | да, underlying array |
| `map[K]V` | копия descriptor (≈ pointer) | да, та же map |
| `chan T` | копия descriptor (≈ pointer) | да |
| `*T` (pointer) | копия адреса | да, тот же объект |
| `interface{}` | копия {type, data} pair | data может быть shared |
| `func` | копия closure header | captures shared |

**Struct с ссылочными полями:**
```go
type Config struct {
    Tags    []string          // slice header → shared underlying array
    Options map[string]string // descriptor → shared map
    Name    string            // string immutable, safe to copy
}

c1 := Config{Tags: []string{"a", "b"}}
c2 := c1          // c1.Tags и c2.Tags смотрят на один slice!
c2.Tags[0] = "z"
fmt.Println(c1.Tags[0]) // "z" — неожиданно!

// Защита: deep copy при необходимости независимости
c3 := Config{
    Tags:    append([]string{}, c1.Tags...),
    Options: maps.Clone(c1.Options), // Go 1.21+
    Name:    c1.Name,
}
```

## Когда value semantics

Value semantics — когда копия это правильное поведение:

```go
// ✓ Маленькие immutable value objects
type Money struct {
    Amount   int64
    Currency string
}
func (m Money) Add(other Money) Money {
    return Money{m.Amount + other.Amount, m.Currency}
}

// ✓ Снэпшоты конфигурации
type ServerConfig struct {
    Timeout time.Duration
    MaxConn int
}

// ✓ Координаты, точки, прямоугольники
type Rect struct{ X, Y, W, H float64 }
func (r Rect) Area() float64 { return r.W * r.H }

// ✓ Передача по значению показывает: функция не меняет аргумент
func processConfig(cfg ServerConfig) { ... } // сигнатура явно говорит "не изменю"
```

## Когда pointer semantics

```go
// ✓ Мутирующие методы
type Counter struct{ n int }
func (c *Counter) Inc() { c.n++ }       // обязательно pointer receiver
func (c *Counter) Value() int { return c.n } // для консистентности тоже pointer

// ✓ Структуры с sync-примитивами — НИКОГДА не копировать
type SafeMap struct {
    mu   sync.RWMutex
    data map[string]int
}

// ✓ Различение "нет значения" (nil) от zero value
type UserSettings struct {
    Theme *string // nil = не установлено, &"dark" = установлено в "dark"
}

// ✓ Большие структуры (избегаем дорогого копирования)
type LargePayload struct {
    Data [1 << 20]byte // 1 MB — не копируем
}
func process(p *LargePayload) { ... }

// ✓ Стабильная identity (несколько частей системы держат ссылку на один объект)
type Node struct {
    Val  int
    Next *Node
}
```

## Классические production-баги

### Баг 1: копирование struct с mutex

```go
type Service struct {
    mu    sync.Mutex
    cache map[string]int
}

func (s *Service) Set(k string, v int) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.cache[k] = v
}

// ПЛОХО: копирование Service
func newWorker(svc Service) { // копируем struct — mu скопирован в начальном состоянии
    svc.Set("x", 1)           // работает с копией mu, не с оригинальным
}

// ХОРОШО: передавать pointer
func newWorker(svc *Service) {
    svc.Set("x", 1)
}

// go vet -copylocks ловит эту ошибку:
// "newWorker passes lock by value: Service contains sync.Mutex"
```

### Баг 2: неожиданное разделение slice

```go
// append может создать новый underlying array (при превышении cap)
// или разделять старый (если cap есть запас)

func addToList(base []int) []int {
    return append(base, 99)
}

orig := make([]int, 3, 6) // len=3, cap=6 — есть запас!
orig[0] = 1

extended := addToList(orig)
extended[0] = 42

fmt.Println(orig[0]) // 42 — orig[0] изменился! shared underlying array

// Защита: если нужна независимость
extended := append([]int{}, orig...) // явная копия
extended = append(extended, 99)
extended[0] = 42
fmt.Println(orig[0]) // 1 — теперь независимо
```

### Баг 3: хранение указателя на элемент slice перед append

```go
items := []int{1, 2, 3}
ptr := &items[0] // указатель на первый элемент

items = append(items, 4, 5, 6, 7) // превышаем cap → новый underlying array

*ptr = 99              // изменяем OLD array (уже не items!)
fmt.Println(items[0])  // 1 — неожиданно, ptr стал dangling (к старому массиву)
```

### Баг 4: inconsistent receiver type на одном типе

```go
type Buffer struct {
    data []byte
    pos  int
}

// ПЛОХО: смешанные receivers
func (b Buffer) Len() int      { return len(b.data) - b.pos } // value
func (b *Buffer) Read(p []byte) (int, error) { ... }           // pointer

// Это работает, но Buffer не удовлетворяет io.Reader через value:
var r io.Reader = Buffer{} // ОШИБКА: Read требует pointer receiver
var r io.Reader = &Buffer{} // OK

// ХОРОШО: все методы с pointer receiver, если хоть один мутирует
func (b *Buffer) Len() int      { return len(b.data) - b.pos }
func (b *Buffer) Read(p []byte) (int, error) { ... }
```

## Практические правила

1. **Если тип содержит `sync.Mutex`, `sync.RWMutex`, `sync.WaitGroup`, `sync.Once`, atomic поля — только pointer semantics**, никогда не копировать. `go vet` поможет (`-copylocks`).

2. **Консистентный receiver**: если хотя бы один метод с pointer receiver — делай все методы pointer receiver.

3. **Value для immutable value objects** (Money, Point, Color, ID) — явно показывает, что нет shared mutable state.

4. **Pointer для mutable service state** (DB pools, caches, handlers) — один объект с lifecycle.

5. **При сомнении о slice aliasing**: если нужна независимость — всегда явно копировать через `append([]T{}, original...)` или `copy`.

## Interview-ready answer

**"Когда использовать pointer receiver, а когда value receiver?"**

Pointer receiver обязателен когда: метод мутирует struct, struct содержит sync-примитив (mutex нельзя копировать), struct большая и дорогая для копирования, нужна стабильная identity (несколько держателей одного объекта). Value receiver когда: метод не мутирует, struct маленькая и логически immutable (Money, Point, Config).

**"Почему slice выглядит как value, но ведет себя как reference?"**

При передаче slice копируется header: {pointer, len, cap}. Два slice указывают на один underlying array. Поэтому `s2[0] = 99` меняет оба. `append` создает новый массив только при превышении cap — если cap есть запас, оба slice начнут расходиться после append, но старые элементы будут shared.

**"Почему копирование struct с mutex опасно?"**

`sync.Mutex` содержит internal state — после копирования обе копии mutex независимы. Lock на копии не блокирует оригинал. `go vet` с `-copylocks` ловит это: "passes lock by value".
