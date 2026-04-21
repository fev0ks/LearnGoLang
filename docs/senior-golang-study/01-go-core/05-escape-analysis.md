# Escape Analysis

Escape analysis — это как компилятор Go решает, где разместить значение: на стеке (дёшево, не нагружает GC) или в heap (дороже, GC должен его отслеживать).

## Содержание

- [Базовая идея](#базовая-идея)
- [Что вызывает escape](#что-вызывает-escape)
- [Как читать вывод компилятора](#как-читать-вывод-компилятора)
- [Inlining и его эффект на escape](#inlining-и-его-эффект-на-escape)
- [Что реально дает выигрыш](#что-реально-дает-выигрыш)
- [Практический подход](#практический-подход)
- [Interview-ready answer](#interview-ready-answer)

## Базовая идея

```
Стек: аллокация и cleanup бесплатны (двигаем stack pointer)
Heap: аллокация + GC должен отслеживать объект → дороже
```

Компилятор выбирает стек если может **доказать**, что значение не переживает текущий stack frame. Если не может — переносит в heap (escape).

Важно: ни `new(T)`, ни `&x` не гарантируют heap — компилятор сам решает:
```go
func sum(a, b int) *int {
    result := a + b
    return &result // result ESCAPES to heap — живет дольше функции
}

func process() int {
    x := computeExpensive() // x может остаться на стеке, если не утекает
    return x
}
```

## Что вызывает escape

### 1. Возврат указателя на локальную переменную

```go
func newVal() *int {
    x := 42
    return &x // x escapes to heap: переживает stack frame функции
}
```

### 2. Сохранение в интерфейс

```go
func toInterface(x int) interface{} {
    return x // x escapes to heap (для non-pointer значений, не влезающих в pointer word)
}

func toAny(s MyStruct) any {
    return s // s escapes: struct копируется в heap
}

// Но pointer остается pointer — дополнительного escape нет:
func ptrToInterface(p *int) interface{} {
    return p // p (pointer) НЕ escapes: сам pointer помещается в interface.data
}
```

### 3. Closure захватывает переменную

```go
func makeCounter() func() int {
    count := 0         // count escapes: живет в замыкании дольше функции
    return func() int {
        count++
        return count
    }
}
```

### 4. Передача в другую горутину

```go
func spawn(x int) {
    go func() {
        fmt.Println(x) // x escapes: горутина переживает текущий stack frame
    }()
}
```

### 5. Слишком большой объект для стека

```go
func bigArray() [1 << 20]byte {
    var arr [1 << 20]byte // 1MB — слишком велик для стека, escapes to heap
    return arr
}
```

### 6. Сложный data flow, который компилятор не может проанализировать

```go
func storeInSlice(items []*int, val int) {
    items = append(items, &val) // val escapes: хранится в slice, живущем снаружи
}
```

## Как читать вывод компилятора

```bash
# Базовый вывод escape analysis
go build -gcflags="-m" ./...

# Детальный вывод с причинами (level 2)
go build -gcflags="-m=2" ./...

# Для конкретного файла
go build -gcflags="-m" ./path/to/package
```

Пример вывода:

```go
// main.go
package main

import "fmt"

func newInt() *int {
    x := 42
    return &x
}

func printVal(v interface{}) {
    fmt.Println(v)
}

func main() {
    p := newInt()
    printVal(*p)
}
```

```bash
$ go build -gcflags="-m" .
./main.go:6:2: moved to heap: x          ← x в функции newInt escapes
./main.go:10:14: v escapes to heap       ← аргумент printVal(v interface{}) escapes
./main.go:11:13: ... argument does not escape  ← fmt.Println инлайнит и оптимизирует
```

Ключевые сообщения:
- `moved to heap: x` — переменная x перемещена в heap;
- `x escapes to heap` — x уходит в heap из-за...;
- `does not escape` — хорошо, остается на стеке;
- `inlining call to ...` — функция была инлайнена.

## Inlining и его эффект на escape

Когда функция инлайнится, её переменные анализируются в контексте вызывающей функции — и часто перестают escape:

```go
func add(a, b int) *int {
    result := a + b
    return &result // БЕЗ inlining: result escapes
}

func main() {
    p := add(1, 2) // С inlining: result может остаться на стеке main
    fmt.Println(*p)
}
```

```bash
$ go build -gcflags="-m=2" .
./main.go:3:2: add: result escapes to heap      # без inlining
./main.go:3:2: inlining call to add              # с inlining
# → result больше не escapes
```

Компилятор инлайнит "дешевые" функции (по умолчанию budget = 80 AST nodes). Инлайнинг может **убирать** escape и снижать allocation pressure.

## Что реально дает выигрыш

Escape analysis важен в **горячих путях** с высоким RPS:

```go
// Плохо: аллокация на каждый вызов
func buildKey(userID, resource string) *string {
    key := userID + ":" + resource // string concatenation → heap allocation
    return &key
}

// Лучше: возвращать значение (если caller не хранит указатель долго)
func buildKey(userID, resource string) string {
    return userID + ":" + resource // может остаться на стеке
}

// Ещё лучше для hot path: pre-allocated buffer
func buildKey(buf []byte, userID, resource string) []byte {
    buf = append(buf[:0], userID...)
    buf = append(buf, ':')
    buf = append(buf, resource...)
    return buf // reuse buffer, no heap allocation
}
```

```go
// Плохо: interface вызов с value type аллоцирует копию
type Handler interface {
    Handle(req Request) Response
}
type MyHandler struct{ ... }
func (h MyHandler) Handle(req Request) Response { ... }  // value receiver

var h Handler = MyHandler{...}
h.Handle(req) // MyHandler копируется в heap при каждом присваивании

// Лучше для частых переприсваиваний: pointer receiver
var h Handler = &MyHandler{...}  // указатель в interface — нет лишней аллокации
```

## Практический подход

1. **Не оптимизируй вслепую** — сначала найди hot path через `pprof` CPU/alloc profile.
2. **Измеряй аллокации** в benchmark перед изменениями:
   ```bash
   go test -bench=BenchmarkHandler -benchmem
   # BenchmarkHandler-8   100000   15234 ns/op   2048 B/op   8 allocs/op
   ```
3. **Читай `-gcflags=-m`** только для горячих функций.
4. **Проверяй эффект** — `allocs/op` в benchmark должен уменьшиться.
5. Если код стал менее читаемым ради нулевой аллокации вне hot path — не стоит.

Антипаттерны:
```go
// Не делай это без измерений:
// - замена interface на concrete type везде
// - *T везде вместо T "для оптимизации"
// - sync.Pool для объектов вне горячего пути
```

## Interview-ready answer

**"Что такое escape analysis и почему это важно?"**

Escape analysis — это статический анализ компилятора, который определяет, может ли значение безопасно жить на стеке, или должно уйти в heap. Стековые аллокации бесплатны (stack pointer сдвинулся). Heap аллокации дороже — GC должен их отслеживать, что влияет на allocation rate и tail latency.

Значение escapes в heap если: возвращается по указателю из функции, хранится в интерфейсе (если не fits in pointer), захватывается замыканием, передается в горутину. Проверить можно через `go build -gcflags="-m"`.

Важный нюанс: `new(T)` и `&x` не гарантируют heap — компилятор может держать их на стеке. И наоборот, `x := T{}` может уйти в heap, если компилятор не может доказать, что x не переживет frame. На практике оптимизируют только hot path после измерения `allocs/op` в benchmark.
