# Primitive Types And Zero Values

Быстрый справочник по встроенным типам и их zero values. Главное для интервью — поведение nil-типов и подводные камни.

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

## Что спрашивают на интервью

- **Чем `byte` отличается от `rune`?** `byte` = `uint8` (8 бит, ASCII символ), `rune` = `int32` (32 бита, Unicode code point).
- **Почему `nil` map и `nil` slice ведут себя по-разному?** Slice структурно готов к append (runtime аллоцирует массив при первом append). Map требует явной инициализации через `make`.
- **Что такое `nil` interface?** Оба поля (type и data) равны nil. Typed nil pointer в interface — не nil interface.
- **Чем `string` отличается от `[]byte`?** `string` — immutable (неизменяемая последовательность байт). `[]byte` — mutable. Конверсия между ними аллоцирует копию.
