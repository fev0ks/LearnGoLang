# Interfaces, Method Sets And Nil Pitfalls

Тема почти всегда всплывает на интервью, потому что касается и API design, и реальных production-багов. Senior-уровень — это не просто знать nil-ловушку, а понимать iface/eface layout, виртуальный dispatch и метод сеты.

## Содержание

- [Runtime представление интерфейса](#runtime-представление-интерфейса)
- [Nil interface vs typed nil: как это работает](#nil-interface-vs-typed-nil-как-это-работает)
- [Когда interface вызов аллоцирует](#когда-interface-вызов-аллоцирует)
- [Method sets](#method-sets)
- [Addressability и interface satisfaction](#addressability-и-interface-satisfaction)
- [Interface design: практические правила](#interface-design-практические-правила)
- [Production-антипаттерны](#production-антипаттерны)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

## Runtime представление интерфейса

Go различает два вида интерфейсов на уровне runtime.

### eface — пустой интерфейс (`interface{}` / `any`)

```go
// runtime/iface.go
type eface struct {
    _type *_type          // тип значения (nil если значение nil)
    data  unsafe.Pointer  // указатель на данные (или само значение если помещается)
}
```

### iface — интерфейс с методами

```go
type iface struct {
    tab  *itab            // (интерфейс, конкретный тип) → таблица методов
    data unsafe.Pointer   // указатель на данные
}

type itab struct {
    inter *interfacetype  // описание interface-типа
    _type *_type          // конкретный тип, реализующий интерфейс
    hash  uint32          // копия _type.hash (для type switch без разыменования)
    _     [4]byte
    fun   [1]uintptr      // vtable: fun[0], fun[1], ... — указатели на методы
}
```

`itab` кэшируется глобально: одна пара `(interface type, concrete type)` → один `itab`. Создается при первом использовании, потом переиспользуется.

**Вызов метода через интерфейс:**
```go
var w io.Writer = os.Stdout
w.Write(data)

// Компилируется примерно в:
// tab := w.tab           // 1 pointer load
// fn  := tab.fun[0]      // 1 pointer load (индекс метода в vtable)
// fn(w.data, data)       // вызов через function pointer
```

Два indirect pointer loads — вот почему interface вызов чуть медленнее прямого вызова и почти всегда не инлайнится.

## Nil interface vs typed nil: как это работает

Это самая частая ошибка на Go-интервью.

**Nil interface** — оба поля (tab и data) равны nil:
```go
var err error // tab=nil, data=nil → interface == nil ✓
fmt.Println(err == nil) // true
```

**Typed nil** — tab не nil (тип задан), data = nil:
```go
func getError() error {
    var p *MyError = nil // *MyError, значение nil
    return p             // tab=*itab{MyError}, data=nil → interface != nil !
}

err := getError()
fmt.Println(err == nil) // false — ЛОВУШКА
```

Визуально в памяти:
```
nil interface:           typed nil interface:
┌──────────┐            ┌──────────────────┐
│ tab: nil │            │ tab: → itab{      │
│ data: nil│            │       MyError}    │
└──────────┘            │ data: nil         │
                        └──────────────────┘
```

**Классический баг:**
```go
type MyError struct{ msg string }
func (e *MyError) Error() string { return e.msg }

func fetchData() error {
    var err *MyError // typed nil
    if someFailed {
        err = &MyError{"something failed"}
    }
    return err // ВСЕГДА возвращает non-nil error, даже если err == nil!
}

// Правильно:
func fetchData() error {
    var err *MyError
    if someFailed {
        err = &MyError{"something failed"}
    }
    if err != nil {
        return err // возвращаем конкретный тип только если ненулевой
    }
    return nil // явный nil интерфейса
}
```

**Ещё один частый случай — логгирование:**
```go
// Плохо: logger принимает interface{}, typed nil логируется как не-nil
func maybeLog(err error) {
    if err != nil { // true если typed nil!
        log.Error(err)
    }
}

// Правильно: проверять конкретный тип ДО передачи в интерфейс
var dbErr *DBError
if dbErr != nil { // проверяем concrete type
    return dbErr
}
return nil
```

## Когда interface вызов аллоцирует

Хранение значения в interface может вызвать heap allocation:

```go
// НЕ аллоцирует: значение помещается в pointer-sized word
var i interface{} = 42          // small int → хранится в data напрямую
var i interface{} = true        // bool → inline
var i interface{} = (*int)(ptr) // pointer → inline

// АЛЛОЦИРУЕТ: значение не помещается в один pointer
var i interface{} = MyStruct{...}  // struct → копия уходит в heap
var i interface{} = [10]int{...}   // array → уходит в heap
```

```go
// Проверить через escape analysis:
// go build -gcflags="-m" ./...

func storeInInterface(s MyStruct) interface{} {
    return s // s escapes to heap
}
```

Это важно для горячих путей: если вызываешь `fmt.Println(x)` или `json.Marshal(x)` в tight loop, каждый вызов может аллоцировать.

## Method sets

Правила для удовлетворения интерфейса:

| Receiver   | Реализован методами T | Реализован методами *T |
|------------|----------------------|------------------------|
| T value    | ✅ оба               | ❌ только *T           |
| *T pointer | ✅ оба               | ✅ оба                 |

```go
type Stringer interface {
    String() string
}

type MyType struct{ val int }

// Value receiver — доступен и для T, и для *T
func (m MyType) String() string {
    return fmt.Sprintf("%d", m.val)
}

var s Stringer
s = MyType{42}   // OK: value receiver доступен для T
s = &MyType{42}  // OK: value receiver доступен и для *T
```

```go
// Pointer receiver — только *T удовлетворяет интерфейсу
func (m *MyType) Reset() {
    m.val = 0
}

type Resetter interface {
    Reset()
}

var r Resetter
r = &MyType{42}  // OK
r = MyType{42}   // ОШИБКА КОМПИЛЯЦИИ: MyType не реализует Resetter
                 // (метод Reset имеет pointer receiver)
```

Почему так: если бы `T` мог удовлетворять интерфейсу с pointer receiver, это означало бы изменение **копии** — бессмысленно и confusing.

## Addressability и interface satisfaction

Интересный edge case:

```go
type Counter struct{ n int }
func (c *Counter) Inc() { c.n++ }

type Incer interface{ Inc() }

// Это работает:
c := Counter{}
var i Incer = &c  // OK: &c адресуема

// Это тоже работает (компилятор может взять адрес):
c := Counter{}
c.Inc()  // компилятор переписывает в (&c).Inc() — c адресуема

// Это НЕ работает (non-addressable):
var i Incer = Counter{}    // ОШИБКА: Counter{} — временное значение, не адресуемо
Counter{}.Inc()            // ОШИБКА: нельзя взять адрес временного значения
m := map[string]Counter{}
m["x"].Inc()               // ОШИБКА: элемент map не адресуем
```

## Interface design: практические правила

**Интерфейс описывает поведение потребителя, а не поставщика:**

```go
// Плохо: интерфейс объявлен со стороны реализации
// package storage
type StorageService interface {
    Get(id string) (*Item, error)
    Set(id string, item *Item) error
    Delete(id string) error
    List(prefix string) ([]*Item, error)
    // ... 10 методов
}

// Хорошо: интерфейс объявлен со стороны потребителя,
// содержит только то, что нужно именно этому потребителю
// package handler
type ItemGetter interface {
    Get(id string) (*Item, error)
}

type OrderHandler struct {
    items ItemGetter // только один метод — легче тестировать
}
```

**Маленькие интерфейсы — это не компромисс, это дизайн:**

```go
// Стандартная библиотека как пример:
type Reader interface {
    Read(p []byte) (n int, err error)  // 1 метод
}
type Writer interface {
    Write(p []byte) (n int, err error) // 1 метод
}
type ReadWriter interface {  // композиция, не монолит
    Reader
    Writer
}
```

**Accept interfaces, return concrete types:**

```go
// Принимать интерфейс: позволяет любой реализации
func Process(r io.Reader) error { ... }

// Возвращать конкретный тип: caller сам решит, нужен ли интерфейс
func NewProcessor() *Processor { ... }  // а не ProcessorInterface

// Исключение: возвращать error (стандартный интерфейс ошибки)
func fetchData() (*Data, error) { ... }  // OK
```

## Production-антипаттерны

```go
// Антипаттерн 1: возврат typed nil через error интерфейс
func loadConfig() error {
    var err *ConfigError
    // ... если ошибки не было ...
    return err  // BUG: всегда non-nil!
}

// Антипаттерн 2: god object interface
type Service interface {
    DoA(); DoB(); DoC(); DoD(); DoE(); DoF() // 20+ методов
    // невозможно подменить в тестах без написания полного mock
}

// Антипаттерн 3: interface "на будущее" без второй реализации
type DB interface {
    Query(sql string) (*Rows, error)
}
// Если реализация одна — это преждевременная абстракция;
// добавить интерфейс всегда можно позже, когда появится вторая реализация

// Антипаттерн 4: копирование struct с embedded sync.Mutex
type Counter struct {
    sync.Mutex
    n int
}
c1 := Counter{}
c2 := c1  // BUG: копируем mutex → c2 работает с копией lock, не с оригиналом
```

## Разбор примеров-загадок

### Загадка 1: typed nil через error

```go
type MyError struct{ msg string }
func (e *MyError) Error() string { return e.msg }

func find() error {
    var e *MyError  // nil-указатель
    return e        // возвращаем конкретный тип
}

func main() {
    err := find()
    fmt.Println(err == nil)  // ?
}
```

<details>
<summary>Ответ</summary>

```
false
```

`err` — это interface value `{ tab: *MyError, data: nil }`. Поле типа **не nil** (тип известен — `*MyError`), поэтому весь интерфейс ≠ nil, хотя внутри лежит nil-указатель.

Это та самая «typed nil trap». Фикс — не возвращать typed nil: проверить `if e != nil { return e }; return nil`, либо вообще объявлять функцию с конкретным типом возврата, а интерфейс отдавать на самом верхнем уровне.

> Бонус: `fmt.Println(err)` тут не напечатает `<nil>`, а попробует вызвать `Error()` на nil-указателе → `e.msg` разыменует nil → panic. fmt перехватит её и выведет `%!v(PANIC=Error method: ...)`.
</details>

---

### Загадка 2: метод на nil-указателе не паникует

```go
type Node struct{ next *Node }
func (n *Node) Len() int {
    if n == nil {
        return 0
    }
    return 1 + n.next.Len()
}

func main() {
    var n *Node  // nil
    fmt.Println(n.Len())  // ?
}
```

<details>
<summary>Ответ</summary>

```
0
```

Вызов метода на nil-указателе **сам по себе не паника** — паникует только *разыменование* nil. Здесь receiver `n == nil`, метод это проверяет и возвращает 0, не трогая поля.

Это легально и даже используется в дизайне (рекурсивные структуры, nil как «пустой список»). `n.Len()` компилируется в `(*Node).Len(n)` — указатель просто передаётся аргументом, и пока его не разыменовали, всё ок.
</details>

---

### Загадка 3: сравнение интерфейсов с несравнимым типом

```go
func main() {
    var a interface{} = []int{1, 2}
    var b interface{} = []int{1, 2}
    fmt.Println(a == b)  // ?
}
```

<details>
<summary>Ответ</summary>

```
panic: runtime error: comparing uncomparable type []int
```

`==` на интерфейсах сравнивает (тип, значение). Компилируется без ошибок — статический тип `interface{}` сравним. Но **в рантайме**, если внутри лежит несравнимый тип (`slice`, `map`, `func`), Go паникует.

Поэтому `interface{}` с неизвестным содержимым опасно сравнивать через `==` — нужен `reflect.DeepEqual`. Та же ловушка взрывается неявно: такой тип как ключ map или в `==` внутри `switch` → рантайм-паника.
</details>

---

### Загадка 4: nil concrete-типа в интерфейсе

```go
func main() {
    var s []int           // nil slice
    var m map[string]int  // nil map
    var i interface{} = s
    var j interface{} = m

    fmt.Println(s == nil, m == nil)  // ?
    fmt.Println(i == nil, j == nil)  // ?
}
```

<details>
<summary>Ответ</summary>

```
true true
false false
```

Сам по себе nil slice/map/chan/func **равен nil**. Но как только его кладут в `interface{}`, у интерфейса появляется тип → интерфейс становится non-nil. Это обобщение typed-nil-ловушки не только на указатели: **любой** конкретный nil, завёрнутый в интерфейс, делает интерфейс ≠ nil.
</details>

---

### Загадка 5: method value захватывает копию receiver

```go
type Counter struct{ n int }
func (c Counter) Get() int { return c.n }  // value receiver

func main() {
    c := Counter{n: 1}
    f := c.Get        // method value
    c.n = 100
    fmt.Println(f())      // ?
    fmt.Println(c.Get())  // ?
}
```

<details>
<summary>Ответ</summary>

```
1
100
```

`f := c.Get` — это **method value**: receiver вычисляется и **сохраняется копией прямо сейчас**. Поскольку receiver — по значению, `f` держит снимок `Counter{n: 1}`. Последующее `c.n = 100` на снимок не влияет → `f()` == 1.

Если бы receiver был указателем (`func (c *Counter) Get()`), `c.Get` сохранил бы `&c`, и `f()` увидел бы 100. Различие «value receiver method value = снимок копии» — частый сюрприз с замыканиями и колбэками.
</details>

---

### Загадка 6: String() с бесконечной рекурсией

```go
type Celsius float64
func (c Celsius) String() string {
    return fmt.Sprintf("%v°C", c)  // ?
}

func main() {
    var c Celsius = 20
    fmt.Println(c)
}
```

<details>
<summary>Ответ</summary>

```
fatal error: stack overflow
```

`fmt.Println(c)` видит, что `Celsius` реализует `Stringer`, и зовёт `String()`. Внутри `%v` снова применяется к `Celsius` → снова `String()` → бесконечная рекурсия → переполнение стека.

Фикс — на время форматирования убрать метод, приведя к базовому типу: `fmt.Sprintf("%v°C", float64(c))`. Классический реальный баг при добавлении `String()`/`Error()` к типу.
</details>

---

### Загадка 7: nil embedded interface

```go
type Service struct {
    io.Reader  // встроенный интерфейс, по умолчанию nil
}

func main() {
    s := Service{}
    buf := make([]byte, 8)
    _, err := s.Read(buf)  // ?
    _ = err
}
```

<details>
<summary>Ответ</summary>

```
panic: runtime error: invalid memory address or nil pointer dereference
```

Встроенный в структуру **интерфейс** (`io.Reader`) по умолчанию `nil`. Метод `Read` промоутится в `Service`, но вызывается на nil-интерфейсе → паника. В отличие от Загадки 2 (nil *указатель* + receiver-метод), здесь нет конкретного типа вообще — звать нечего.

Поэтому встраивание интерфейса требует его инициализации: `Service{Reader: r}`. Приём встречается в декораторах (обернуть `io.Reader`, переопределив часть методов) — но забытая инициализация = nil-паника.
</details>

---

## Interview-ready answer

Короткие ответы на реально частые вопросы про интерфейсы.

**1. Чем `iface` отличается от `eface`?**
`eface` — пустой интерфейс (`any`): `{ *_type, data }`. `iface` — с методами: `{ *itab, data }`, где `itab` хранит конкретный тип и vtable методов. Вызов метода — два pointer load (`tab → fun[i]`) + call, потому не инлайнится.

**2. Когда interface value равен nil?**
Только когда **оба** поля (тип и data) nil. Завёрнутый конкретный nil (`*T`, nil slice/map/chan) даёт non-nil интерфейс — это typed nil trap. Фикс: не возвращать typed nil, отдавать `nil` явно.

**3. Value receiver vs pointer receiver в method set?**
Метод с value receiver входит в set и `T`, и `*T`. С pointer receiver — только `*T`. Значит интерфейс с pointer-receiver методами удовлетворяет только `*T`. Невозможность присвоить — частая ошибка компиляции (`T does not implement I`).

**4. Когда укладывание значения в интерфейс аллоцирует?**
Когда значение не помещается в одно слово-указатель: struct, array, большой тип → копия escapes в heap. Указатели, маленькие inline-значения — без аллокации. Важно на горячих путях (`fmt`, `json` в цикле).

**5. Можно ли вызвать метод на nil?**
На nil-**указателе** — да, если метод не разыменовывает receiver (часто проверяют `if n == nil`). На nil-**интерфейсе** — нет, паника (нет ни типа, ни данных).

**6. Почему `interface{} == interface{}` может паниковать?**
`==` сравнивает (тип, значение). Если внутри несравнимый тип (slice/map/func) — рантайм-паника `comparing uncomparable type`. Для произвольного содержимого — `reflect.DeepEqual`.

**7. Где объявлять интерфейс?**
На стороне потребителя, маленьким (1–3 метода). «Accept interfaces, return concrete types». Интерфейс «на будущее» без второй реализации — преждевременная абстракция.
