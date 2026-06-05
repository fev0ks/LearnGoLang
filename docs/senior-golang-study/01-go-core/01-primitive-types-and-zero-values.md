# Primitive Types, Sizes And Overflow

Справочник по встроенным типам: zero values, размеры, диапазоны числовых типов, переполнение и конверсии. Главное для интервью — поведение nil-типов, overflow и подводные камни.

## Содержание

- [Таблица типов и zero values](#таблица-типов-и-zero-values)
- [Числовые типы: диапазоны и знак](#числовые-типы-диапазоны-и-знак)
- [Переполнение (overflow)](#переполнение-overflow)
- [Конверсии числовых типов](#конверсии-числовых-типов)
- [Критически важное поведение nil-типов](#критически-важное-поведение-nil-типов)
- [Инициализация через make vs new](#инициализация-через-make-vs-new)
- [Interview-ready answer](#interview-ready-answer)

---

## Таблица типов и zero values

| Тип | Размер | Zero value | Пример |
|-----|--------|-----------|--------|
| `bool` | 1 байт | `false` | `var b bool` |
| `string` | 16 байт ¹ | `""` | `var s string` |
| `int` | 4 / 8 байт ² | `0` | `var n int` |
| `int8` | 1 байт | `0` | `var n int8` |
| `int16` | 2 байта | `0` | `var n int16` |
| `int32` / `rune` | 4 байта | `0` | `var n int32` |
| `int64` | 8 байт | `0` | `var n int64` |
| `uint` | 4 / 8 байт ² | `0` | `var u uint` |
| `uint8` / `byte` | 1 байт | `0` | `var u uint8` |
| `uint16` | 2 байта | `0` | `var u uint16` |
| `uint32` | 4 байта | `0` | `var u uint32` |
| `uint64` | 8 байт | `0` | `var u uint64` |
| `uintptr` | 4 / 8 байт ² | `0` | `var u uintptr` |
| `float32` | 4 байта | `0` | `var f float32` |
| `float64` | 8 байт | `0` | `var f float64` |
| `complex64` | 8 байт | `0+0i` | `var c complex64` |
| `complex128` | 16 байт | `0+0i` | `var c complex128` |
| `*T` (pointer) | 4 / 8 байт ² | `nil` | `var p *int` |
| `[]T` (slice) | 24 байта ¹ | `nil` | `var s []int` |
| `map[K]V` | 8 байт ² | `nil` | `var m map[string]int` |
| `chan T` | 8 байт ² | `nil` | `var ch chan int` |
| `func(...)` | 8 байт ² | `nil` | `var fn func()` |
| `interface{}` / `any` | 16 байт | `nil` | `var i any` |
| `struct{}` | 0 байт | all fields zero | `var s MyStruct` |

¹ — заголовок структуры: `string` = `{*byte, int}`, `[]T` = `{*T, int, int}` (ptr + len + cap).  
² — зависит от платформы: 4 байта на 32-bit, 8 байт на 64-bit (обычно).

Проверить размер любого типа: `unsafe.Sizeof(x)`.

## Числовые типы: диапазоны и знак

| Тип | Bits | Signed | Диапазон |
|-----|------|--------|----------|
| `int8` | 8 | yes | -128 .. 127 |
| `int16` | 16 | yes | -32768 .. 32767 |
| `int32` / `rune` | 32 | yes | -2³¹ .. 2³¹-1 ≈ ±2.1B |
| `int64` | 64 | yes | -2⁶³ .. 2⁶³-1 ≈ ±9.2×10¹⁸ |
| `int` | 32 или 64 | yes | зависит от платформы |
| `uint8` / `byte` | 8 | no | 0 .. 255 |
| `uint16` | 16 | no | 0 .. 65535 |
| `uint32` | 32 | no | 0 .. 2³²-1 ≈ 4.3B |
| `uint64` | 64 | no | 0 .. 2⁶⁴-1 ≈ 1.8×10¹⁹ |
| `uint` | 32 или 64 | no | зависит от платформы |
| `float32` | 32 | — | ≈ ±3.4×10³⁸, ~7 значащих цифр |
| `float64` | 64 | — | ≈ ±1.8×10³⁰⁸, ~15 значащих цифр |

`int` на современных 64-bit серверах = 64 бита, но это **не гарантировано** спецификацией.

### Когда `int`, а когда `int64`

```go
// int — для индексов, длин, счётчиков внутри процесса (native word size)
for i := 0; i < len(s); i++ { ... }
n := len(items)

// int64 — для внешних контрактов: DB-схема, API-поля, protobuf, timestamps
type User struct {
    ID        int64  // DB primary key — фиксированный размер важен
    CreatedAt int64  // unix timestamp
    Score     int64  // поле в API
}

// float64 — стандарт для вычислений; float32 — только для GPU/graphics
var ratio float64 = float64(count) / float64(total)
```

## Переполнение (overflow)

Integer overflow в Go — **defined behavior** (в отличие от C/C++): оборачивается по модулю 2ⁿ.

```go
var x int8 = 127
x++
fmt.Println(x) // -128 — overflow, обернулся

var u uint8 = 255
u++
fmt.Println(u) // 0 — overflow

// Compile-time constant overflow компилятор ЛОВИТ:
const big = int8(200) // ошибка компиляции: constant 200 overflows int8

// Runtime overflow НЕ ловится — нужно проверять вручную:
func safeAdd(a, b int64) (int64, error) {
    if b > 0 && a > math.MaxInt64-b {
        return 0, errors.New("overflow")
    }
    if b < 0 && a < math.MinInt64-b {
        return 0, errors.New("overflow")
    }
    return a + b, nil
}
```

### Полезные константы

```go
import "math"

math.MaxInt8    // 127
math.MinInt8    // -128
math.MaxInt32   // 2147483647
math.MaxInt64   // 9223372036854775807   (2⁶³ - 1)
math.MaxUint64  // 18446744073709551615
math.MaxInt     // зависит от платформы: на 64-bit = MaxInt64
math.MaxFloat64 // 1.7976931348623157e+308
```

## Конверсии числовых типов

```go
// Преобразование между числовыми типами — всегда ЯВНОЕ, не неявное
var i int = 42
var f float64 = float64(i)
var u uint = uint(f)

// Усечение при конверсии в меньший тип — молча, без panic:
var big int64 = 1000
var small int8 = int8(big) // -24 — усечение! не ошибка

// Рекомендация: проверяй диапазон перед конверсией из большого в малый тип
```

## Критически важное поведение nil-типов

```go
// nil slice — можно append, нельзя по индексу
var s []int
s = append(s, 1)  // OK: append создает underlying array
_ = s[0]          // OK после append
var empty []int
fmt.Println(len(empty) == 0) // true — len nil slice = 0

// nil map — можно читать (возвращает zero value), НЕЛЬЗЯ писать
var m map[string]int
_ = m["key"]      // OK: возвращает 0
m["key"] = 1      // PANIC: assignment to entry in nil map
m["key"]++        // PANIC тоже: ++ это read-modify-WRITE
delete(m, "key")  // OK: delete на nil map — безопасный no-op
// исправление: m := make(map[string]int)

// nil channel — блокирует send и receive навсегда
var ch chan int
ch <- 1  // блокируется навсегда
<-ch     // блокируется навсегда
// НО: close(nil channel) → panic

// nil interface vs interface с nil внутри
var err *MyError = nil
var iface error = err
fmt.Println(iface == nil) // false! — тип задан, значение nil
fmt.Println(err == nil)   // true — конкретный тип равен nil

// nil pointer — разыменование → panic
var p *int
fmt.Println(*p) // PANIC: nil pointer dereference
```

## Инициализация через make vs new

```go
// make — только для slice, map, chan; возвращает инициализированный тип
s := make([]int, 0, 10)       // len=0, cap=10
m := make(map[string]int)     // пустая map, готова к использованию
ch := make(chan int, 5)        // buffered channel

// new — для любого типа; возвращает *T со zero value
p := new(int)      // *int, *p = 0
s := new([]int)    // *[]int, *s = nil — НЕ готова к append без инициализации

// literal — для struct
cfg := Config{Timeout: 5 * time.Second}
```

## Interview-ready answer

**1. Чем `byte` отличается от `rune`?**
`byte` = `uint8` (8 бит, один байт/ASCII), `rune` = `int32` (32 бита, один Unicode code point). Подробнее про строки — в [08-strings](./08-strings.md).

**2. Почему `nil` map и `nil` slice ведут себя по-разному?**
Чтение обоих безопасно. Но в nil slice можно `append` (runtime аллоцирует массив при первом append), а запись в nil map паникует (`assignment to entry in nil map`) — map требует `make`. `delete` на nil map — безопасный no-op.

**3. Что такое `nil` interface trap?**
Interface == nil только когда **оба** поля (type и data) nil. Typed nil pointer (`*MyError(nil)`), завёрнутый в interface, делает его non-nil. Детали — в [04-interfaces](./03-interfaces-method-sets-and-nil.md).

**4. Чем `string` отличается от `[]byte`?**
`string` immutable, `[]byte` mutable; конверсия между ними копирует. Размер заголовка: `string` = 16 байт (`{ptr, len}`), `[]T` = 24 байта (`{ptr, len, cap}`).

**5. Чем `int` отличается от `int64`?**
`int` зависит от платформы (32/64 бита, native word size), `int64` — всегда 64 бита. На 64-bit сервере размер одинаков, но типы **не** взаимозаменяемы без явной конверсии. `int` — для индексов/длин/счётчиков, `int64` — для внешних контрактов (DB, API, protobuf, timestamps).

**6. Как ведёт себя overflow?**
Defined behavior — оборачивание по модулю 2ⁿ (не UB как в C). Compile-time константы компилятор проверяет (`const big = int8(200)` — ошибка). Runtime overflow не ловится — проверяй вручную через `math.MaxIntN`. Конверсия в меньший тип молча усекает.

**7. make vs new?**
`make` — только для slice/map/chan, возвращает готовый к работе тип. `new(T)` — для любого типа, возвращает `*T` со zero value (`new([]int)` даёт `*[]int` на nil slice — не готов к индексации без инициализации).
