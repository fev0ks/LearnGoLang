# Numeric Types, Integer Sizes And Overflow

Справочник по числовым типам, их размерам и поведению при переполнении.

## Таблица числовых типов

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
| `float32` | 32 | — | ≈ ±3.4×10³⁸, 7 знаков |
| `float64` | 64 | — | ≈ ±1.8×10³⁰⁸, 15 знаков |

`int` на современных 64-bit серверах = 64 бита, но это не гарантировано спецификацией.

## Когда использовать int vs int64

```go
// int — для индексов, длин, счётчиков внутри процесса
for i := 0; i < len(s); i++ { ... }
n := len(items)

// int64 — для внешних контрактов: DB схема, API поля, protobuf, timestamps
type User struct {
    ID        int64     // DB primary key — фиксированный размер важен
    CreatedAt int64     // unix timestamp
    Score     int64     // поле в API
}

// float64 — стандарт для вычислений; float32 только для GPU/graphic workloads
var ratio float64 = float64(count) / float64(total)
```

## Переполнение

Integer overflow в Go — **defined behavior** (в отличие от C/C++): оборачивается по модулю.

```go
var x int8 = 127
x++
fmt.Println(x) // -128 — overflow, обернулся

var u uint8 = 255
u++
fmt.Println(u) // 0 — overflow

// Compile-time constant overflow компилятор ловит:
const big = int8(200) // ОШИБКА компиляции: overflows int8

// Runtime overflow не ловится — нужно проверять вручную:
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

## Полезные константы

```go
import "math"

math.MaxInt8   // 127
math.MinInt8   // -128
math.MaxInt32  // 2147483647
math.MaxInt64  // 9223372036854775807
math.MaxUint64 // 18446744073709551615
math.MaxInt    // зависит от платформы: на 64-bit = MaxInt64
math.MaxFloat64 // 1.7976931348623157e+308
```

## Конверсии

```go
// Явное преобразование — всегда явное, не неявное
var i int = 42
var f float64 = float64(i)
var u uint = uint(f)

// Усечение при конверсии в меньший тип:
var big int64 = 1000
var small int8 = int8(big) // -24 — усечение! не ошибка, не panic

// Рекомендация: проверяй диапазон перед конверсией из большого в малый тип
```

## Что спрашивают на интервью

- **Чему равен `math.MaxInt64`?** 9223372036854775807 (2⁶³ - 1).
- **Чем `int` отличается от `int64`?** `int` зависит от платформы (32 или 64 бита), `int64` — всегда 64 бита. На 64-bit серверах размер одинаков, но типы не взаимозаменяемы без явной конверсии.
- **Как ведёт себя overflow?** Defined behavior в Go — оборачивание по модулю. Compile-time constants компилятор проверяет. Runtime overflow не проверяется — надо делать вручную.
- **Когда `int64`, а когда `int`?** `int64` для внешних контрактов (DB, API, protobuf) где важен фиксированный размер. `int` для внутренней арифметики, индексов, длин — соответствует native word size платформы.
