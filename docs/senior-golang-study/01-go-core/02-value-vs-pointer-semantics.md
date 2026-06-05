# Value vs Pointer Semantics

Влияет на API design, race conditions, GC pressure и читаемость. Нужно понимать не только когда что выбрать, но и какие конкретные баги возникают при неправильном выборе.

## Содержание

- [Базовая идея](#базовая-идея)
- [Что копируется, а что разделяется](#что-копируется-а-что-разделяется)
- [Когда value semantics](#когда-value-semantics)
- [Когда pointer semantics](#когда-pointer-semantics)
- [Производительность: копия vs указатель](#производительность-копия-vs-указатель)
- [Классические production-баги](#классические-production-баги)
- [Практические правила](#практические-правила)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
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
| `interface{}` | копия 2 слов {type, data}; `data` — указатель | зависит от динамического типа ⁴ |
| `func` | копия closure header | captures shared |

⁴ — `data` в интерфейсе **всегда указатель** (с Go 1.4). Если внутри лежит значение (struct, int), Go его **боксирует** — копирует в heap, и `data` указывает на эту копию. При копировании интерфейса оба слова копируются, поэтому `data`-указатель всегда шарится. Но видно ли это:
> - **динамический тип — указатель/slice/map** → `data` указывает на общий объект, мутация видна через любую копию;
> - **динамический тип — значение** → интерфейс read-only: `x.(T)` отдаёт *копию* бокса, исходный объект не изменить.
>
> Поэтому однозначного «shared / не shared» нет — зависит от того, что внутри. Детали layout — в [03-interfaces-method-sets-and-nil](./03-interfaces-method-sets-and-nil.md).

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

## Производительность: копия vs указатель

Частое заблуждение: «передавать указатель всегда быстрее, ведь копируется всего 8 байт». На деле выбор — это **баланс двух разных стоимостей**:

| | Передача по значению | Передача по указателю |
|---|---|---|
| Стоимость вызова | копирование N байт структуры | копирование 8 байт (адрес) |
| Где живёт | на стеке (бесплатно для GC) | часто **escape в heap** → аллокация + работа GC потом |
| Доступ к полям | напрямую, cache-friendly | разыменование (возможный cache miss) |
| Выигрывает при | маленькие структуры | большие структуры / мутация / shared |

Ключевой неочевидный момент: **взятие указателя часто заставляет объект уехать в heap**. Если ты передаёшь `&x` в функцию, которая его куда-то сохраняет (или компилятор не смог доказать, что не сохраняет), `x` перестаёт жить на стеке и становится heap-аллокацией — а это потом нагружает GC. Для маленькой структуры это **дороже**, чем просто скопировать её по значению на стеке.

```go
type Small struct{ A, B, C int64 } // 24 байта

// value: 24 байта копируются на стек вызываемой функции, heap не трогается
func sumV(s Small) int64 { return s.A + s.B + s.C }

// pointer: 8 байт, НО &s часто уводит s в heap → аллокация + GC
func sumP(s *Small) int64 { return s.A + s.B + s.C }

// для Small{} вызов sumV обычно быстрее и без аллокаций — копия 24 B тривиальна
```

**Где проходит граница (rule of thumb):**

- **маленькая структура** (примерно до нескольких машинных слов, ориентир ≈ ≤ 64 байт) → **по значению**: копия дешёвая, остаётся на стеке, ноль нагрузки на GC, поля читаются напрямую;
- **большая структура** (массивы, много полей) → **по указателю**: копировать сотни байт на каждый вызов дороже, чем разыменование; но помни, что объект, скорее всего, окажется в heap;
- **мутация нужна / есть `sync`-примитив / shared identity** → **указатель** по корректности (это перевешивает любые соображения скорости).

**Нюансы:**

- **Inlining** часто стирает разницу: если функция мелкая и заинлайнилась, ни копии, ни разыменования по факту нет — компилятор работает с переменными напрямую.
- **Receiver'ы** подчиняются той же логике: value receiver у большого типа = копия на каждый вызов метода.
- Не угадывай — **меряй**: `go test -bench -benchmem` (смотри `allocs/op`) и `go build -gcflags="-m"` (escape в heap). Подробно про escape — в [memory-internals/03-escape-analysis](./memory-internals/03-escape-analysis.md).

> TL;DR: указатель экономит на копировании, но платит аллокацией в heap и нагрузкой на GC. Для маленьких структур по значению обычно **быстрее**. Корректность (mutex/мутация) важнее перформанса — там всегда указатель.

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

6. **Перформанс**: маленькую структуру (≈ ≤ 64 B) передавай **по значению** — копия дешевле, чем escape в heap + нагрузка на GC от указателя. Большую — по указателю. Корректность (mutex/мутация) важнее: там указатель независимо от размера. Проверяй `-benchmem` и `-gcflags=-m`, не угадывай.

## Разбор примеров-загадок

Запутанные кейсы на `*` и `&`: где меняется оригинал, где копия, что шарится при передаче.

### Загадка 1: `&v` цикловой переменной в range

```go
nums := []int{10, 20, 30}
var ptrs []*int
for _, v := range nums {
    ptrs = append(ptrs, &v)
}
fmt.Println(*ptrs[0], *ptrs[1], *ptrs[2])  // ?
```

<details>
<summary>Ответ</summary>

```
Go ≤ 1.21:  30 30 30
Go ≥ 1.22:  10 20 30
```

`v` в range — это **одна переменная**, переиспользуемая на каждой итерации (до Go 1.22). `&v` все три раза — один и тот же адрес, в котором к концу цикла лежит `30`. Поэтому все указатели «смотрят» на 30.

В **Go 1.22** семантику изменили: `v` теперь новая на каждой итерации → `10 20 30`. Это была одна из самых частых ловушек в Go; если код собирается под старый `go` в `go.mod` — поведение всё ещё старое.

Надёжный фикс под любую версию — брать адрес элемента слайса, а не цикловой переменной:
```go
for i := range nums {
    ptrs = append(ptrs, &nums[i])  // адрес реального элемента
}
```
</details>

---

### Загадка 2: range отдаёт копию — оригинал не меняется

```go
type Point struct{ X int }
pts := []Point{{1}, {2}, {3}}
for _, p := range pts {
    p.X *= 10          // меняем p
}
fmt.Println(pts)       // ?
```

<details>
<summary>Ответ</summary>

```
[{1} {2} {3}]
```

`p` — **копия** элемента. `p.X *= 10` меняет копию, слайс не тронут. Чтобы изменить оригинал — обращайся по индексу:

```go
for i := range pts {
    pts[i].X *= 10     // [{10} {20} {30}]
}
```

Та же ловушка с методами: `for _, p := range pts { p.Inc() }` при value receiver меняет копию. Со слайсом указателей (`[]*Point`) проблемы нет — копируется указатель, а не структура.
</details>

---

### Загадка 3: переприсваивание указателя внутри функции vs разыменование

```go
func reassign(p *int) { x := 99; p = &x }  // меняем локальную копию указателя
func deref(p *int)    { *p = 99 }           // меняем то, на что указываем

func main() {
    a := 1
    reassign(&a)
    fmt.Println(a)  // ?
    deref(&a)
    fmt.Println(a)  // ?
}
```

<details>
<summary>Ответ</summary>

```
1
99
```

В `reassign` параметр `p` — это **копия** указателя. `p = &x` переставляет локальную копию на другой адрес; у вызывающего `a` не меняется. В `deref` же `*p = 99` пишет по адресу, который держит `p` → это адрес `a` → меняется оригинал.

Мораль: чтобы функция изменила **значение** — разыменовывай (`*p = ...`). Чтобы изменила **сам указатель** вызывающего — нужен указатель на указатель (см. Загадку 4).
</details>

---

### Загадка 4: чтобы поменять указатель — нужен `**T`

```go
func setNil(p *int)   { p = nil }   // не сработает
func setNil2(pp **int){ *pp = nil } // сработает

func main() {
    x := 5
    p := &x
    setNil(p)
    fmt.Println(p == nil)  // ?
    setNil2(&p)
    fmt.Println(p == nil)  // ?
}
```

<details>
<summary>Ответ</summary>

```
false
true
```

`setNil` получает копию указателя — обнуление копии не видно снаружи. `setNil2` получает **адрес указателя** (`**int`) и через `*pp = nil` зануляет оригинальный `p`. Тот же принцип, что и в Загадке 3, но на уровень выше: меняешь не значение, а сам указатель → нужен указатель на него.
</details>

---

### Загадка 5: массив копируется, слайс шарится

```go
func modArr(a [3]int)  { a[0] = 99 }
func modSlice(s []int) { s[0] = 99 }

func main() {
    arr := [3]int{1, 2, 3}
    modArr(arr)
    fmt.Println(arr)  // ?

    sl := []int{1, 2, 3}
    modSlice(sl)
    fmt.Println(sl)   // ?
}
```

<details>
<summary>Ответ</summary>

```
[1 2 3]
[99 2 3]
```

**Массив** `[3]int` — это значение: при передаче копируется целиком, изменения в функции не видны. **Слайс** — это header `{ptr, len, cap}`; копируется header, но `ptr` указывает на тот же backing array → запись через `s[0]` видна снаружи.

Поэтому массивы передают по указателю (`*[3]int`), если нужно изменить или избежать копии. А слайс «и так ссылается» — но именно поэтому случайно мутирует чужие данные.
</details>

---

### Загадка 6: вернуть `&local` — легально, не dangling

```go
func newCounter() *int {
    n := 42
    return &n  // возвращаем адрес локальной переменной
}

func main() {
    p := newCounter()
    fmt.Println(*p)  // ?
}
```

<details>
<summary>Ответ</summary>

```
42
```

В отличие от C/C++, в Go это **безопасно**. Escape analysis видит, что адрес `n` уходит наружу, и размещает `n` в **heap**, а не на стеке. Указатель остаётся валидным, пока на него кто-то ссылается; освободит GC.

Поэтому `&Struct{}`, `new(T)`, возврат `&local` — обычная идиома. Цена — heap-аллокация (см. [memory-internals/03-escape-analysis](./memory-internals/03-escape-analysis.md)), а не висячий указатель.
</details>

---

### Загадка 7: `&map[key]` и pointer-метод на элементе map

```go
type Counter struct{ n int }
func (c *Counter) Inc() { c.n++ }

func main() {
    m := map[string]Counter{"a": {}}
    m["a"].Inc()      // ?
    p := &m["a"]      // ?
}
```

<details>
<summary>Ответ</summary>

```
оба — ошибки компиляции:
  cannot call pointer method Inc on m["a"]   (m["a"] не адресуем)
  cannot take address of m["a"]
```

Элемент map **не адресуем**: map может реаллоцироваться (рост, эвакуация бакетов), и старый адрес стал бы невалидным — Go запрещает брать `&m[k]`. А значит и pointer-метод `Inc` (которому нужен `&m["a"]`) вызвать нельзя.

Обходные пути:
```go
c := m["a"]; c.Inc(); m["a"] = c   // достать → изменить → положить обратно
// или хранить указатели:
m2 := map[string]*Counter{"a": {}}
m2["a"].Inc()                       // OK: значение и так указатель
```

Контраст: элемент **слайса** адресуем (`&s[0]` легально), потому что backing array не «переезжает» сам по себе — но осторожно с append-реаллокацией (Баг 3 выше).
</details>

---

## Interview-ready answer

**"Когда использовать pointer receiver, а когда value receiver?"**

Pointer receiver обязателен когда: метод мутирует struct, struct содержит sync-примитив (mutex нельзя копировать), struct большая и дорогая для копирования, нужна стабильная identity (несколько держателей одного объекта). Value receiver когда: метод не мутирует, struct маленькая и логически immutable (Money, Point, Config).

**"Почему slice выглядит как value, но ведет себя как reference?"**

При передаче slice копируется header: {pointer, len, cap}. Два slice указывают на один underlying array. Поэтому `s2[0] = 99` меняет оба. `append` создает новый массив только при превышении cap — если cap есть запас, оба slice начнут расходиться после append, но старые элементы будут shared.

**"Почему копирование struct с mutex опасно?"**

`sync.Mutex` содержит internal state — после копирования обе копии mutex независимы. Lock на копии не блокирует оригинал. `go vet` с `-copylocks` ловит это: "passes lock by value".
