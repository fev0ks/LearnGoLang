# Generics

Generics появились в Go 1.18. Решают конкретную задачу: писать типобезопасный code reuse без дублирования и без потери типовой информации. Не серебряная пуля — у generics есть цена и ограничения.

## Содержание

- [Type parameters и constraints — синтаксис](#type-parameters-и-constraints--синтаксис)
- [Когда generics vs `interface{}` vs кодогенерация](#когда-generics-vs-interface-vs-кодогенерация)
- [Производительность: когда generics медленнее interface](#производительность-когда-generics-медленнее-interface)
- [Generic data structures](#generic-data-structures)
- [Пакеты `slices`, `maps`, `cmp` из stdlib](#пакеты-slices-maps-cmp-из-stdlib)
- [Подводные камни](#подводные-камни)
- [Пример: типобезопасный Result тип](#пример-типобезопасный-result-тип)
- [Interview-ready answer](#interview-ready-answer)

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
func BinarySearch[T cmp.Ordered](s []T, target T) int { ... }  // cmp.Ordered (Go 1.21+)
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

## Производительность: когда generics медленнее interface

### Короткая версия — что запомнить

Если ничего больше не отложится, запомни три случая:

| Что делает дженерик | Скорость против `interface{}` | Почему |
|---|---|---|
| считает числа / value-типы (`+`, `<`, поля структуры) | **быстрее** ✅ | нет боксинга, тип известен на компиляции |
| работает с указателями (`*User`, `*Order`) | **так же** ≈ | под капотом одинаковый layout |
| вызывает методы через constraint (`T.String()`) | **может быть медленнее** ⚠️ | вызов идёт через таблицу, без инлайна |

Одной фразой: **дженерики любят числа, равнодушны к указателям и не любят вызовы методов.** Всё остальное в этом разделе — объяснение, *почему* так.

### Почему так — аналогия со «шпаргалкой»

Есть два честных способа скомпилировать дженерик-функцию:

- **Нанять специалиста под каждый тип** (так делают C++ и Rust): на `T = int` — своя копия кода, на `T = string` — своя. Быстро (каждый знает свой тип наизусть), но копий много → бинарь пухнет, компиляция дольше.
- **Один универсальный работник на всё** (так делает Java): он работает с любым типом, но через «обёртку» (боксинг) — поэтому медленно на числах.

Go выбрал середину. Он нанимает **одного работника на каждую «форму» типа** и выдаёт ему **шпаргалку (словарь, dictionary)** с конкретикой: «тип — 8 байт, копировать вот так, методы — вот в этой табличке». Работник общий, а детали подсматривает в шпаргалке.

Отсюда и три случая из таблицы:
- сложить числа работник умеет сам, шпаргалка почти не нужна → **быстро**;
- вызвать метод он сам не может — каждый раз лезет в табличку методов из шпаргалки → **лишний шаг, медленнее**.

### Что такое «форма» (GC shape)

«Форма» = **размер типа + где внутри него лежат указатели**. Почему именно это: общему коду надо лишь уметь скопировать значение и дать сборщику мусора найти в нём указатели — для этого конкретный тип не важен, важны размер и расположение указателей. Поэтому один код шарится между типами одной формы:

```
Stack[*User]   \
Stack[*Order]   >-- одна копия кода (все указатели — одна форма)
Stack[*Item]   /

Stack[int32]   \
Stack[float32]  >-- одна копия кода (оба 4 байта, без указателей)

Stack[int64]  --- отдельная копия (8 байт)
```

### Что лежит в шпаргалке (словаре)

Словарь — это скрытый аргумент, который компилятор сам подставляет в каждый вызов дженерик-функции:

```
SumGeneric[Int](xs)  →  реально вызывается  SumGeneric(dict_Int, xs)
                                                         └── скрытая шпаргалка для T=Int
```

Внутри: реальный тип `T` (для `new(T)`, `reflect`, zero-value), **таблицы методов (itab)** — если constraint требует методов, данные для сравнения/хеширования. Главное следствие — то самое из таблицы: **вызов метода идёт через табличку из словаря, его нельзя заинлайнить.**

### Цифры (бенчмарки `go1.26`, arm64)

**Случай «вызов метода» — где дженерик проседает.** `Sum` через метод `Add` по 1000 элементам: дженерик с constraint-методом vs та же логика под конкретный `Int`:

```
BenchmarkGenericMethod-16    808 ns/op   0 B/op   0 allocs/op   // T.Add() через табличку из словаря
BenchmarkConcreteMethod-16   258 ns/op   0 B/op   0 allocs/op   // Int.Add() напрямую, инлайнится
```

~**3.1× медленнее**, и заметь — **аллокаций ноль у обоих**. То есть дело не в боксинге, а именно в «лишнем шаге» через словарь: метод не инлайнится. Если такой вызов на горячем пути — иногда дешевле написать конкретную версию руками.

**Случай «числа» — где дженерик выигрывает.** Тот же `Sum`, но без методов — просто `+` по 1000 элементам, дженерик `[]int` vs интерфейсный `[]any`:

```
BenchmarkGenericInt-16    265 ns/op    0 B/op   0 allocs/op   // Sum[int]([]int)
BenchmarkIfaceInt-16      369 ns/op    0 B/op   0 allocs/op   // Sum([]any) с x.(int) на элемент
```

~**1.4× быстрее** — нет `x.(int)` на каждый элемент. И это ещё не вся выгода: чтобы вообще передать `[]int` как `[]any`, каждый `int` пришлось бы **боксировать** (по аллокации на элемент), а дженерик работает с `[]int` напрямую — здесь `[]any` собран заранее, чтобы сравнение было честным (см. [боксинг в 03-interfaces](./03-interfaces-method-sets-and-nil.md#когда-укладка-значения-в-интерфейс-аллоцирует-боксинг)).

### Для контекста: как это делают другие языки

| Подход | Кто | Идея | Цена |
|---|---|---|---|
| Специалист на каждый тип (мономорфизация) | C++, Rust | копия кода под каждый `T` | максимум скорости, но толстый бинарь + долгая сборка |
| Один работник + боксинг (type erasure) | Java | всё через `Object`/указатель | компактно, но медленно на числах |
| Один работник + шпаргалка (**Go**) | Go 1.18+ | копия на «форму» + словарь с конкретикой | компромисс: быстро на числах, есть косвенность на методах |

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

// Constraint для ordered types (числа и строки — всё, что поддерживает < > <= >=)
type Ordered interface {
    ~int | ~int8 | ... | ~float64 | ~string
}
```

> **`cmp.Ordered` (Go 1.21, stdlib) vs `constraints.Ordered`** (`golang.org/x/exp/constraints`, внешний). Раньше единственным был `constraints.Ordered` из экспериментального пакета; с Go 1.21 тот же constraint вошёл в stdlib как `cmp.Ordered` — теперь используют его, без внешней зависимости. `slices.Sort`/`slices.BinarySearch`/`slices.Min` опираются именно на `cmp.Ordered`. Не путать с `comparable` (про `==`/`!=`, есть у структур/указателей) — `Ordered` про **порядок** `<`, только числа и строки.

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

### Generic type aliases (Go 1.24+)

До Go 1.24 алиас типа не мог иметь параметров. С 1.24 — может:

```go
type Set[T comparable] = map[T]struct{}   // параметризованный алиас (Go 1.24+)
```

Удобно для коротких имён над дженерик-типами. До 1.24 пришлось бы заводить полноценный `type Set[T comparable] map[T]struct{}` (новый тип, а не алиас).

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

**1. Зачем generics, если есть `interface{}`?**

- `interface{}` теряет тип в compile-time — ошибки ловятся в runtime через type assertion, а value-типы боксируются (аллокации). Generics сохраняют типы (`[]T` остаётся `[]T`), убирают боксинг и type-assertion boilerplate. В бенчмарке выше generic-сумма ~1.4× быстрее интерфейсной (нет `x.(int)` на элемент) и не требует боксировать данные в `[]any`.

**2. Как Go компилирует generics?**

- Гибрид, не мономорфизация (C++/Rust) и не боксинг (Java). **GCShape stenciling**: общий код на «форму» (размер + layout указателей), а не на каждый `T` — все pointer-типы делят реализацию, value-типы одного размера тоже. Тип-специфику (реальный тип, itab'ы методов) передаёт скрытый аргумент — **словарь (dictionary)**. Следствие: вызов метода на `T` идёт через itab из словаря, без инлайна → может быть медленнее конкретного кода (в бенче ~3.1×). Выигрыш дженериков — на операциях без методов над value-типами: нет боксинга и `x.(T)` на элемент.

**3. Почему нельзя generic-метод?**

- Type parameters привязаны к типу или функции, не к методу. Обход — сделать тип дженериком (`Container[T]`, тогда метод берёт `T` из типа) или вынести в функцию. Также нельзя type switch по `T` напрямую — только через `any(v)`.

**4. `comparable` vs `cmp.Ordered`?**

- `comparable` — про `==`/`!=` (числа, строки, указатели, структуры из comparable-полей). `cmp.Ordered` (Go 1.21, stdlib) — про порядок `<`/`>`, только числа и строки. До 1.21 порядковый constraint жил во внешнем `golang.org/x/exp/constraints`.

**5. Когда generics, когда кодогенерация?**

- Generics — когда алгоритм одинаков для всех типов (Map/Filter/Set, контейнеры). Кодогенерация — когда для каждого типа нужна особая оптимизация/поведение (protobuf) или тип задаётся при сборке. `any` — когда тип реально неизвестен в компиляции (JSON в `map[string]any`, разнородные коллекции).
