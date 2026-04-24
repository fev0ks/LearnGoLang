# Generics

Generics появились в Go 1.18. Решают конкретную задачу: писать типобезопасный code reuse без дублирования и без потери типовой информации. Не серебряная пуля — у generics есть цена и ограничения.

---

## Type parameters и constraints — синтаксис

### Базовый синтаксис

```go
// T — type parameter, any — constraint (принимает любой тип)
func Min[T any](a, b T) T {
    // ...
}
```

Квадратные скобки после имени функции / типа — объявление type parameters.

### `any` vs `comparable`

```go
// any = interface{} — принимает всё, нельзя использовать ==
func Contains[T any](slice []T, item T) bool {
    for _, v := range slice {
        // v == item — ОШИБКА КОМПИЛЯЦИИ: T не обязательно comparable
    }
    return false
}

// comparable — допускает == и !=
func Contains[T comparable](slice []T, item T) bool {
    for _, v := range slice {
        if v == item {
            return true
        }
    }
    return false
}
```

`comparable` — встроенный constraint, которому удовлетворяют: int, string, bool, pointer, array (не slice/map/func).

### Кастомные constraints через interface

```go
// Constraint — это обычный interface
type Number interface {
    int | int8 | int16 | int32 | int64 |
        uint | uint8 | uint16 | uint32 | uint64 |
        float32 | float64
}

func Sum[T Number](nums []T) T {
    var total T
    for _, n := range nums {
        total += n
    }
    return total
}

Sum([]int{1, 2, 3})     // 6
Sum([]float64{1.1, 2.2}) // 3.3
```

### Тильда `~` — underlying type

```go
type Celsius float64
type Fahrenheit float64

// Без ~ : принимает только float64, не Celsius
type Float interface { float64 }

// С ~ : принимает float64 и любой тип с underlying type float64
type FloatLike interface { ~float64 }

func Double[T FloatLike](v T) T {
    return v * 2
}

Double(Celsius(20.0))    // OK — Celsius underlying type = float64
Double(Fahrenheit(68.0)) // OK
```

### Несколько type parameters

```go
// Map — трансформация слайса
func Map[T, U any](slice []T, fn func(T) U) []U {
    result := make([]U, len(slice))
    for i, v := range slice {
        result[i] = fn(v)
    }
    return result
}

names := Map(users, func(u User) string { return u.Name })
```

### Constraints с методами

```go
type Stringer interface {
    String() string
}

func Print[T Stringer](items []T) {
    for _, item := range items {
        fmt.Println(item.String())
    }
}
```

---

## Когда generics vs `interface{}` vs кодогенерация

### Выбор инструмента

| Критерий | Generics | `interface{}` / `any` | Кодогенерация |
|---|---|---|---|
| Типобезопасность | ✅ compile-time | ❌ runtime panic | ✅ compile-time |
| Производительность | ✅ / ⚠️ (см. ниже) | ❌ boxing overhead | ✅ нет overhead |
| Читаемость кода | ✅ | ✅ | ❌ gen-файлы |
| Сложность инструментария | ✅ нет | ✅ нет | ❌ нужен генератор |
| Отладка | ✅ | ✅ | ⚠️ gen-файлы | 

### Используй generics когда

```go
// 1. Утилитарные функции над коллекциями
func Filter[T any](s []T, fn func(T) bool) []T { ... }
func Keys[K comparable, V any](m map[K]V) []K { ... }

// 2. Типобезопасные контейнеры (Stack, Queue, Set)
type Stack[T any] struct { items []T }
func (s *Stack[T]) Push(v T) { s.items = append(s.items, v) }
func (s *Stack[T]) Pop() (T, bool) { ... }

// 3. Алгоритмы, не зависящие от конкретного типа
func BinarySearch[T constraints.Ordered](s []T, target T) int { ... }
```

### Используй `interface{}` / `any` когда

```go
// Тип неизвестен на этапе компиляции (например, JSON декодирование)
var result any
json.Unmarshal(data, &result)

// Разнородные элементы в одной коллекции
items := []any{1, "hello", true}
```

### Используй кодогенерацию когда

- Нужны специфичные оптимизации для каждого типа
- Сложная логика, которая не выражается через type constraints
- Уже используется в проекте (protobuf, mock-генераторы)

---

## Generic data structures

### Set

```go
type Set[T comparable] struct {
    m map[T]struct{}
}

func NewSet[T comparable](items ...T) *Set[T] {
    s := &Set[T]{m: make(map[T]struct{})}
    for _, item := range items {
        s.Add(item)
    }
    return s
}

func (s *Set[T]) Add(v T)            { s.m[v] = struct{}{} }
func (s *Set[T]) Remove(v T)         { delete(s.m, v) }
func (s *Set[T]) Contains(v T) bool  { _, ok := s.m[v]; return ok }
func (s *Set[T]) Len() int           { return len(s.m) }

func (s *Set[T]) Union(other *Set[T]) *Set[T] {
    result := NewSet[T]()
    for k := range s.m {
        result.Add(k)
    }
    for k := range other.m {
        result.Add(k)
    }
    return result
}

// Использование
ints := NewSet(1, 2, 3, 4)
ints.Contains(3) // true
```

### Stack

```go
type Stack[T any] struct {
    items []T
}

func (s *Stack[T]) Push(v T) {
    s.items = append(s.items, v)
}

func (s *Stack[T]) Pop() (T, bool) {
    if len(s.items) == 0 {
        var zero T
        return zero, false
    }
    top := s.items[len(s.items)-1]
    s.items = s.items[:len(s.items)-1]
    return top, true
}

func (s *Stack[T]) Peek() (T, bool) {
    if len(s.items) == 0 {
        var zero T
        return zero, false
    }
    return s.items[len(s.items)-1], true
}

func (s *Stack[T]) Len() int { return len(s.items) }
```

### Слайс-утилиты: Map, Filter, Reduce

```go
// Map: преобразование каждого элемента
func Map[T, U any](s []T, fn func(T) U) []U {
    result := make([]U, len(s))
    for i, v := range s {
        result[i] = fn(v)
    }
    return result
}

// Filter: отбор по предикату
func Filter[T any](s []T, fn func(T) bool) []T {
    result := make([]T, 0)
    for _, v := range s {
        if fn(v) {
            result = append(result, v)
        }
    }
    return result
}

// Reduce: свёртка
func Reduce[T, U any](s []T, init U, fn func(U, T) U) U {
    acc := init
    for _, v := range s {
        acc = fn(acc, v)
    }
    return acc
}

// Использование
nums := []int{1, 2, 3, 4, 5}
doubled := Map(nums, func(n int) int { return n * 2 })      // [2 4 6 8 10]
evens := Filter(nums, func(n int) bool { return n%2 == 0 }) // [2 4]
sum := Reduce(nums, 0, func(acc, n int) int { return acc + n }) // 15

// Смешанные типы
strs := Map(nums, func(n int) string { return strconv.Itoa(n) }) // ["1" "2" ...]
```

---

## Пакеты `slices`, `maps`, `cmp` из stdlib

Go 1.21 добавил стандартные generic-утилиты.

### `slices`

```go
import "slices"

nums := []int{3, 1, 4, 1, 5, 9}

slices.Sort(nums)                          // [1 1 3 4 5 9] — сортировка на месте
slices.Contains(nums, 4)                   // true
slices.Index(nums, 4)                      // 3 (или -1)
slices.Reverse(nums)                       // разворот на месте
slices.Compact(nums)                       // убирает подряд идущие дубликаты
slices.Clone(nums)                         // поверхностная копия
slices.Equal(nums, []int{1, 1, 3, 4, 5, 9}) // true

// Бинарный поиск (требует отсортированного слайса)
i, found := slices.BinarySearch(nums, 4)

// Пользовательская сортировка
type Person struct { Name string; Age int }
people := []Person{{"Bob", 30}, {"Alice", 25}}
slices.SortFunc(people, func(a, b Person) int {
    return cmp.Compare(a.Age, b.Age) // через cmp.Compare
})

// Поиск min/max
min := slices.Min(nums)
max := slices.Max(nums)
```

### `maps`

```go
import "maps"

m := map[string]int{"a": 1, "b": 2}

maps.Clone(m)                     // копия map
maps.Keys(m)                      // итератор ключей (Go 1.23+ range over func)
maps.Values(m)                    // итератор значений
maps.DeleteFunc(m, func(k string, v int) bool { return v < 2 }) // удалить где v < 2
maps.Equal(m1, m2)                // сравнение
maps.Copy(dst, src)               // копирование src в dst
```

### `cmp`

```go
import "cmp"

cmp.Compare(1, 2)    // -1
cmp.Compare(2, 2)    // 0
cmp.Compare(3, 2)    // 1

cmp.Or("", "fallback")  // "fallback" — первое ненулевое значение
cmp.Or(0, 0, 42)        // 42

// Constraint для ordered types
type Ordered interface {
    ~int | ~int8 | ... | ~float64 | ~string
}
```

---

## Подводные камни

### Нельзя generic methods

```go
type MyType struct{}

// ОШИБКА — методы не могут иметь свои type parameters
func (t MyType) Process[T any](v T) T { ... }

// Решение 1: generic функция вместо метода
func Process[T any](t MyType, v T) T { ... }

// Решение 2: type parameter на уровне типа
type Container[T any] struct{ value T }
func (c Container[T]) Get() T { return c.value } // ОК — T из типа
```

### Type inference — ограничения

```go
func Map[T, U any](s []T, fn func(T) U) []U { ... }

// Go может вывести T из []int, но U из возвращаемого типа функции не всегда
nums := []int{1, 2, 3}
result := Map(nums, func(n int) string { return strconv.Itoa(n) }) // OK — U выводится из fn
result2 := Map[int, string](nums, ...)                              // явно — всегда работает

// В сложных случаях нужна явная аннотация
var zero T  // нужна переменная zero-value типа T — для этого и нужна var
```

### Нельзя использовать type switch с type parameter

```go
func Print[T any](v T) {
    // Так нельзя — T не конкретный тип в switch
    switch v.(type) {
    case int:    // ОШИБКА
    case string: // ОШИБКА
    }
    
    // Обходной путь через any
    switch any(v).(type) {
    case int:    fmt.Println("int")
    case string: fmt.Println("string")
    }
}
```

### Instantiation создаёт новые типы

```go
Stack[int]  // отдельный тип
Stack[string] // другой тип

var s1 Stack[int]
var s2 Stack[string]
// s1 = s2 — ОШИБКА компиляции
```

---

## Производительность: когда generics медленнее interface

### GCShape stenciling — как Go компилирует generics

Go не создаёт отдельный машинный код для каждого `T`. Вместо этого типы группируются по "GC shape" (размер, указатель vs значение):
- все pointer-типы используют **одну** реализацию с `*uint8`
- value-типы одного размера тоже могут шарить реализацию

```
Stack[*User]   \
Stack[*Order]   >-- один бинарный код (pointer stencil)
Stack[*Item]   /

Stack[int32]   \
Stack[float32]  >-- один бинарный код (4-byte value stencil)

Stack[int64]  --- отдельный код (8-byte value)
```

### Где generics медленнее interface

```go
// Для pointer types — generics не дают преимущества перед interface
// Потому что runtime layout одинаковый

// Для value types — generics быстрее:
// interface{} добавляет boxing (heap allocation для value > pointer size)
// generics — нет boxing
```

Benchmark ориентир:
```
BenchmarkInterfaceMin    50ns/op   1 alloc/op   // interface{} — boxing
BenchmarkGenericMin       5ns/op   0 allocs/op  // generics — без boxing
```

### "Devirtualization" не всегда работает

```go
// Компилятор может не devirtualize если:
// 1. Тип определён в другом пакете
// 2. Constraint допускает много типов → компилятор использует itab
type Processor[T Stringer] struct{ v T }
// Если T = pointer, используется vtable dispatch — как обычный interface
```

### Практический вывод

- Для **числовых и value-типов** (`int`, `float64`, structs) — generics быстрее интерфейсов
- Для **pointer-типов** — производительность аналогична interface (нет разницы)
- Если создаёшь **много коротко живущих объектов через interface** — generics могут убрать GC pressure за счёт отсутствия boxing

---

## Пример: типобезопасный Result тип

```go
type Result[T any] struct {
    value T
    err   error
}

func OK[T any](value T) Result[T]    { return Result[T]{value: value} }
func Err[T any](err error) Result[T] { return Result[T]{err: err} }

func (r Result[T]) Unwrap() (T, error) { return r.value, r.err }

func (r Result[T]) IsOK() bool { return r.err == nil }

func (r Result[T]) OrDefault(def T) T {
    if r.err != nil {
        return def
    }
    return r.value
}

// Использование
func divide(a, b float64) Result[float64] {
    if b == 0 {
        return Err[float64](errors.New("division by zero"))
    }
    return OK(a / b)
}

r := divide(10, 2)
fmt.Println(r.OrDefault(0)) // 5.0
```

---

## Interview-ready answer

**Q: Зачем generics в Go, если есть interface{}?**

`interface{}` теряет типовую информацию в compile-time — ошибки обнаруживаются только в runtime через type assertion. Generics сохраняют типы: `[]T` остаётся `[]T`, не `[]any`. Это убирает boxing для value-типов (0 аллокаций), делает код типобезопасным и устраняет необходимость в type-assertion boilerplate.

**Q: Почему нельзя написать generic method?**

Ограничение языка: type parameters у Go привязаны к типу или функции, а не к методу. Обход — сделать тип generic (`Container[T]`) или перевести метод в функцию.

**Q: Когда generics, когда кодогенерация?**

Generics — когда алгоритм одинаков для всех типов (Map, Filter, Set). Кодогенерация — когда для каждого типа нужна особая оптимизация или поведение (protobuf serialization), или когда тип определяется во время сборки.
