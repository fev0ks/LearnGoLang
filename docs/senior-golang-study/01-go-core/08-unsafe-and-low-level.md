# Unsafe и низкоуровневая память

`unsafe` — пакет, который выключает гарантии безопасности типов и памяти Go ради прямого доступа к layout'у. На нём построены `reflect`, `sync/atomic.Pointer`, zero-copy конверсии `string`↔`[]byte`, и куски рантайма. В прикладном коде нужен редко, но на senior-собесах про него спрашивают: понимаешь ли ты, чем `unsafe.Pointer` отличается от `uintptr`, почему GC может «увести» объект, и как устроены padding и alignment.

## Содержание

- [Зачем нужен unsafe](#зачем-нужен-unsafe)
- [Что в пакете unsafe](#что-в-пакете-unsafe)
- [unsafe.Pointer против uintptr](#unsafepointer-против-uintptr)
- [Шесть легальных паттернов конверсий](#шесть-легальных-паттернов-конверсий)
- [Memory layout: Sizeof, Alignof, Offsetof](#memory-layout-sizeof-alignof-offsetof)
- [Zero-copy конверсии string и []byte](#zero-copy-конверсии-string-и-byte)
- [Где unsafe в рантайме и stdlib](#где-unsafe-в-рантайме-и-stdlib)
- [Правила безопасности](#правила-безопасности)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

---

## Зачем нужен unsafe

Три легитимные причины (всё остальное — почти всегда плохая идея):

1. **Производительность:** убрать копию на горячем пути — конверсия `string`↔`[]byte` без аллокации, доступ к полю по offset.
2. **Interop:** работа с C-памятью (cgo), syscall'ы, бинарные форматы с фиксированным layout.
3. **Инфраструктура:** так написаны `reflect`, `atomic.Pointer[T]`, сериализаторы, ORM, куски рантайма.

> Правило: `unsafe` — это «я беру ответственность на себя». Компилятор и runtime больше не страхуют. Любая ошибка — это не паника с понятным сообщением, а порча памяти, dangling pointer, рандомные краши.

---

## Что в пакете unsafe

Пакет крошечный — несколько функций и один тип:

| Имя | Что делает | Версия |
|---|---|---|
| `unsafe.Pointer` | универсальный указатель, конвертируется в/из любого `*T` | — |
| `unsafe.Sizeof(x)` | размер **типа** x в байтах (compile-time const) | — |
| `unsafe.Alignof(x)` | требование выравнивания типа | — |
| `unsafe.Offsetof(s.f)` | смещение поля `f` внутри структуры | — |
| `unsafe.Add(ptr, n)` | арифметика указателей: `ptr + n` байт | 1.17 |
| `unsafe.Slice(ptr, len)` | собрать `[]T` из указателя и длины | 1.17 |
| `unsafe.SliceData(s)` | указатель на backing array слайса | 1.20 |
| `unsafe.String(ptr, len)` | собрать `string` из `*byte` и длины | 1.20 |
| `unsafe.StringData(s)` | указатель на байты строки | 1.20 |

`Sizeof/Alignof/Offsetof` — это **константы времени компиляции**, они безопасны и не требуют включения `unsafe`-режима мышления. Опасны только конверсии через `unsafe.Pointer`.

---

## unsafe.Pointer против uintptr

Это **главный** вопрос темы. Оба «просто адрес», но разница критична:

| | `unsafe.Pointer` | `uintptr` |
|---|---|---|
| Что это | указатель (для GC — настоящая ссылка) | **целое число** |
| GC видит? | **да** — держит объект живым, обновляет при росте стека | **нет** — для GC это просто `int` |
| Арифметика | нельзя напрямую | можно (это число) |
| Безопасно хранить | да | **нет** — объект может уехать/освободиться |

Ключевое следствие: как только ты превратил `unsafe.Pointer` в `uintptr`, **GC перестал считать это ссылкой**. Объект может быть собран сборщиком или сдвинут при росте стека — и твой `uintptr` будет указывать в мусор.

```go
p := unsafe.Pointer(&x)
u := uintptr(p)        // ← с этого момента x НЕ защищён от GC через u
// runtime.GC() / рост стека здесь могут сделать u невалидным
back := unsafe.Pointer(u) // ← может указывать в никуда
```

Поэтому конверсия `uintptr → Pointer` с арифметикой должна быть в **одном выражении**, без промежуточных переменных (см. ниже).

---

## Шесть легальных паттернов конверсий

Документация `unsafe` явно перечисляет, что **разрешено**. Всё за их пределами — undefined behavior.

**1. `*T1` → `Pointer` → `*T2`** (type punning, при совместимом layout):
```go
var f float64 = 3.14
bits := *(*uint64)(unsafe.Pointer(&f))  // биты float64 как uint64
```

**2. `Pointer` → `uintptr`** (только для печати/сравнения, не обратно как валидный указатель):
```go
fmt.Printf("%#x\n", uintptr(unsafe.Pointer(&x)))
```

**3. `Pointer` → `uintptr` → арифметика → `Pointer` в ОДНОМ выражении** (доступ к полю по offset):
```go
// Доступ к полю структуры по смещению
p := unsafe.Pointer(uintptr(unsafe.Pointer(&s)) + unsafe.Offsetof(s.field))

// Go 1.17+ — то же, но безопаснее и читаемее:
p := unsafe.Add(unsafe.Pointer(&s), unsafe.Offsetof(s.field))
```
Почему в одном выражении: пока вычисляется выражение, компилятор гарантирует, что объект жив. Разбей на два statement — между ними GC/рост стека сломают `uintptr`.

**4. Конверсия при вызове `syscall.Syscall`** (uintptr-аргументы) — должна быть прямо в вызове.

**5. Результат `reflect.Value.Pointer()`/`UnsafeAddr()` → `Pointer`** в одном выражении (они возвращают `uintptr`).

**6. `reflect.SliceHeader`/`StringHeader`** — только когда они указывают на реальные слайс/строку (а лучше с 1.20 — `unsafe.Slice`/`unsafe.String`).

---

## Memory layout: Sizeof, Alignof, Offsetof

### Sizeof считает размер ТИПА, не данных

```go
s := []int{1, 2, 3, 4, 5}
unsafe.Sizeof(s)   // 24 — размер slice header {ptr,len,cap}, НЕ 40 байт данных
unsafe.Sizeof("x") // 16 — string header {ptr,len}
unsafe.Sizeof(42)  // 8  — int на 64-bit
```

### Alignment и padding

Поля выравниваются по своему `Alignof`, из-за чего в структуре появляются «дыры» (padding). Порядок полей влияет на размер:

```go
type Bad struct {
    a bool   // 1 байт
    b int64  // 8 байт → нужно выравнивание по 8
    c bool   // 1 байт
}
// layout: a(1) + padding(7) + b(8) + c(1) + padding(7) = 24 байта

type Good struct {
    b int64  // 8
    a bool   // 1
    c bool   // 1
}
// layout: b(8) + a(1) + c(1) + padding(6) = 16 байт

unsafe.Sizeof(Bad{})  // 24
unsafe.Sizeof(Good{}) // 16  ← те же поля, на 33% меньше
```

**Практика:** в горячих/массовых структурах располагай поля от больших к малым — экономит память и улучшает cache locality. Проверить дыры: `go vet -fieldalignment` или `fieldalignment` из `golang.org/x/tools`.

### Offsetof — смещение поля

```go
type T struct { a int32; b int64; c int8 }
unsafe.Offsetof(T{}.a) // 0
unsafe.Offsetof(T{}.b) // 8  (а не 4 — выравнивание int64)
unsafe.Offsetof(T{}.c) // 16
```

---

## Zero-copy конверсии string и []byte

Обычные `[]byte(s)` и `string(b)` **копируют** (string immutable, slice mutable — общую память делить нельзя). На горячем пути копия бывает неприемлема. Go 1.20+ даёт официальные мостики:

```go
// []byte → string без копии
func bytesToString(b []byte) string {
    return unsafe.String(unsafe.SliceData(b), len(b))
}

// string → []byte без копии
func stringToBytes(s string) []byte {
    return unsafe.Slice(unsafe.StringData(s), len(s))
}
```

⚠️ **Цена:** ты нарушаешь immutability строки.
- После `unsafe.String(...)` **нельзя менять** исходный `b` — строка «поедет».
- `[]byte`, полученный из строки, **нельзя писать**: строковые литералы лежат в read-only сегменте → запись = `SIGSEGV`.

Используй только когда точно контролируешь lifetime и отсутствие мутаций. В обычном коде — обычные конверсии. (Подробнее — в [07-strings](./07-strings.md#unsafe-конверсии-без-копии).)

> До Go 1.20 то же делали через `reflect.StringHeader`/`SliceHeader` — теперь это считается хрупким и устаревшим способом.

---

## Где unsafe в рантайме и stdlib

Чтобы оценить, насколько это фундамент:

- **`reflect`** — весь пакет стоит на `unsafe.Pointer`; `reflect.Value` хранит `unsafe.Pointer` на данные.
- **`sync/atomic.Pointer[T]`** (Go 1.19) — типобезопасная обёртка над атомарным `unsafe.Pointer`.
- **`strings.Builder`** — `String()` отдаёт результат без копии буфера через `unsafe`.
- **рантайм map/slice/string** — `hmap`, slice header, string header описаны через `unsafe.Pointer`.
- **`encoding/json`, ORM (gorm, sqlx), сериализаторы** — доступ к полям по offset вместо reflection на горячем пути.
- **`context`, `sync.Pool`** и др. — точечные оптимизации.

---

## Правила безопасности

1. **`uintptr` — не ссылка.** Не храни его в переменной/поле, если ждёшь, что объект жив. Конверсия `uintptr→Pointer` с арифметикой — в одном выражении.
2. **Держи исходный указатель живым.** Если работаешь с производным `unsafe.Pointer`, оригинальный объект должен быть достижим (иначе GC соберёт). Иногда нужен `runtime.KeepAlive(x)`.
3. **Не мутируй то, что immutable.** Строки (особенно литералы) — read-only.
4. **Layout-совместимость.** `*T1→*T2` валидно только если размеры/раскладка совпадают; не полагайся на порядок полей разных типов.
5. **Проверяй детектором:** `go vet` ловит часть ошибок; есть `go build -gcflags=-d=checkptr` (checkptr) — рантайм-проверки unsafe-конверсий в тестах.
6. **Минимизируй поверхность.** Заверни unsafe в маленькую функцию с безопасным API и тестами, не размазывай по коду.

---

## Разбор примеров-загадок

### Загадка 1: uintptr между двумя statement — dangling

```go
type T struct{ a, b int64 }
t := &T{1, 2}

u := uintptr(unsafe.Pointer(t))          // (1)
// ... тут может сработать GC / вырасти стек ...
pb := unsafe.Pointer(u + unsafe.Offsetof(t.b))  // (2)
fmt.Println(*(*int64)(pb))               // ?
```

<details>
<summary>Ответ</summary>

**Undefined behavior** — может напечатать 2, а может упасть или выдать мусор.

Между (1) и (2) `t` существует только как `uintptr` — для GC это **число, не ссылка**. Если `t` больше нигде не используется, GC вправе собрать объект; при росте стека адрес мог измениться. `u` указывает в никуда.

Правильно — одно выражение (или `unsafe.Add`):
```go
pb := unsafe.Add(unsafe.Pointer(t), unsafe.Offsetof(t.b)) // Go 1.17+
```
Тут компилятор гарантирует, что `t` жив на всё время вычисления.
</details>

---

### Загадка 2: Sizeof слайса

```go
s := make([]int64, 1000)
fmt.Println(unsafe.Sizeof(s))  // ?
```

<details>
<summary>Ответ</summary>

```
24
```

`Sizeof` возвращает размер **типа** — у слайса это header `{ptr, len, cap}` = 24 байта на 64-bit, **независимо** от числа элементов. Сами 1000×8=8000 байт лежат в backing array, на который header лишь ссылается. Та же логика: `Sizeof(string)` = 16 всегда, `Sizeof(map)` = 8 (указатель на hmap).
</details>

---

### Загадка 3: порядок полей меняет размер

```go
type A struct { x bool; y int64; z bool }
type B struct { y int64; x bool; z bool }
fmt.Println(unsafe.Sizeof(A{}), unsafe.Sizeof(B{}))  // ?
```

<details>
<summary>Ответ</summary>

```
24 16
```

`A`: `bool(1)` + padding(7, чтобы `int64` встал по адресу кратному 8) + `int64(8)` + `bool(1)` + padding(7, чтобы размер был кратен 8) = **24**.
`B`: `int64(8)` + `bool(1)` + `bool(1)` + padding(6) = **16**.

Те же поля, но из-за выравнивания порядок «большие → малые» экономит 8 байт. На миллионах структур — заметная разница в RSS и cache. Лови дыры через `go vet -fieldalignment`.
</details>

---

### Загадка 4: запись в строку через unsafe

```go
s := "hello"
b := unsafe.Slice(unsafe.StringData(s), len(s))
b[0] = 'H'           // ?
fmt.Println(s)
```

<details>
<summary>Ответ</summary>

```
обычно: SIGSEGV (segmentation violation) — крах
```

Строковый литерал `"hello"` лежит в **read-only** сегменте бинарника. `unsafe.Slice` дал срез на те же байты, и запись `b[0] = 'H'` пытается изменить read-only память → сигнал от ОС, программа падает (без recover).

Даже если строка не литерал (а из heap) — мутация нарушит immutability: любые её копии «поедут», map с этим ключом сломается. Из строки в `[]byte` через unsafe можно только **читать**.
</details>

---

### Загадка 5: type punning требует совместимого layout

```go
var f float32 = 1.5
p := (*int64)(unsafe.Pointer(&f))   // ?
fmt.Println(*p)
```

<details>
<summary>Ответ</summary>

**Undefined behavior / порча памяти.** `float32` — 4 байта, `int64` — 8. Читая `*(*int64)` по адресу `float32`, ты залезаешь на 4 байта **за** переменную (в соседнюю память/padding). Под `checkptr` это поймается как ошибка.

Type punning через `unsafe.Pointer` легален только при **совпадающих размере и раскладке**: `float32`↔`uint32`, `float64`↔`uint64`. Правильно:
```go
bits := *(*uint32)(unsafe.Pointer(&f))  // OK: оба 4 байта
```
</details>

---

## Interview-ready answer

**1. Чем `unsafe.Pointer` отличается от `uintptr`?**
`unsafe.Pointer` — настоящий указатель: GC считает его ссылкой (держит объект живым, обновляет при росте стека). `uintptr` — просто число: GC его игнорирует, объект может быть собран или сдвинут. Поэтому `uintptr` нельзя хранить, а конверсию `uintptr→Pointer` с арифметикой делают в одном выражении.

**2. Зачем нужен unsafe?**
Производительность (zero-copy `string`↔`[]byte`, доступ к полям по offset), interop (cgo, syscall, бинарные форматы), инфраструктура (`reflect`, `atomic.Pointer`, сериализаторы). В прикладном коде — редко.

**3. Что делают `Sizeof`/`Alignof`/`Offsetof`?**
Compile-time константы: размер типа (для слайса — 24, header, не данные), требование выравнивания, смещение поля. Безопасны. Порядок полей влияет на размер из-за padding — «большие→малые» экономит память.

**4. Как сделать zero-copy `[]byte`→`string`?**
Go 1.20+: `unsafe.String(unsafe.SliceData(b), len(b))`. Цена — нельзя менять `b` после этого (нарушится immutability строки). Обратно — `unsafe.Slice(unsafe.StringData(s), len(s))`, и в этот `[]byte` нельзя писать (литералы read-only → SIGSEGV).

**5. Главные правила безопасности?**
`uintptr` не хранить и арифметику с ним — в одном выражении; держать исходный объект живым (`runtime.KeepAlive`); не мутировать immutable; конвертировать типы только при совпадающем layout; тестировать под `-gcflags=-d=checkptr`.

**6. Где unsafe используется в самом Go?**
`reflect` (целиком), `sync/atomic.Pointer`, `strings.Builder.String()`, рантайм-структуры map/slice/string, JSON/ORM-библиотеки для доступа к полям без рефлексии.
