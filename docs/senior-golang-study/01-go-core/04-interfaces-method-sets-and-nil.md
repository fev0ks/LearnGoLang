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

## Interview-ready answer

**"Чем iface отличается от eface?"**

`eface` — представление пустого интерфейса (`any`): два поля `{ *_type, unsafe.Pointer }`. `iface` — непустого: `{ *itab, unsafe.Pointer }`, где `itab` содержит указатель на vtable методов конкретного типа. Вызов метода через интерфейс — два pointer dereference (iface.tab → fun[i]) + call, поэтому не инлайнится.

**"Что такое nil interface trap?"**

Interface value == nil только когда **оба** поля (type и data) nil. Если вернуть `*MyError(nil)` как `error` — в interface поле type = `*MyError`, data = nil, и `err == nil` вернёт **false**. Фикс: возвращать `nil` явно, не возвращать typed nil pointer.

**"Расскажи про method sets"**

Value receiver метод входит в method set и типа T, и *T. Pointer receiver — только в *T. Поэтому только `*T` может удовлетворять интерфейсу с pointer receiver методами. Это защищает от случайного изменения копии.
