# reflect: интроспекция типов и значений в рантайме

`reflect` позволяет в рантайме узнавать тип значения, читать и менять его поля, вызывать методы — то, что обычно решается на этапе компиляции. На нём построены `encoding/json`, ORM, валидаторы, `fmt`, DI-контейнеры. Тема senior-уровня: важно понимать не только API, но и **на чём reflect стоит** (дескриптор типа из интерфейса), правила **settability** и **цену** (reflection медленный и аллоцирует).

## Содержание

- [На чём построен reflect](#на-чём-построен-reflect)
- [Три закона рефлексии](#три-закона-рефлексии)
- [reflect.Type и reflect.Value](#reflecttype-и-reflectvalue)
- [Kind vs Type — не путать](#kind-vs-type--не-путать)
- [Чтение структуры: поля, теги, методы](#чтение-структуры-поля-теги-методы)
- [Settability: почему `ValueOf(x)` нельзя менять](#settability-почему-valueofx-нельзя-менять)
- [Изменение значений](#изменение-значений)
- [Вызов методов и функций](#вызов-методов-и-функций)
- [Создание значений: New, MakeSlice, MakeMap](#создание-значений-new-makeslice-makemap)
- [reflect.DeepEqual](#reflectdeepequal)
- [Где reflect используется](#где-reflect-используется)
- [Производительность: цена reflection](#производительность-цена-reflection)
- [Generics часто заменяют reflect](#generics-часто-заменяют-reflect)
- [Подводные камни (паники)](#подводные-камни-паники)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

---

## На чём построен reflect

reflect работает **через интерфейс**. Любое значение, переданное в `reflect.TypeOf`/`reflect.ValueOf`, сначала попадает в `interface{}`, а у интерфейса в рантайме есть указатель на **дескриптор типа** (`_type`/`*rtype`) и указатель на данные:

```
любое x  ──упаковка в any──►  eface{ _type, data }
                                  │       │
                  reflect.TypeOf ─┘       └─ reflect.ValueOf
                  (reflect.Type)           (reflect.Value)
```

`reflect.Type` — это обёртка над тем самым `_type` из [03-interfaces](../01-go-core/03-interfaces-method-sets-and-nil.md#где-живёт-тип-compile-time-vs-runtime), а `reflect.Value` несёт `_type` + указатель на данные + флаги (адресуемость, экспортируемость). Отсюда сразу следствие: **reflect видит только то, что попало в интерфейс**, и работает с динамическим типом значения.

---

## Три закона рефлексии

Классическая формулировка (из блога Go), задаёт всю модель:

1. **Из интерфейса — в объект рефлексии.** `reflect.ValueOf(x)` / `reflect.TypeOf(x)` достают тип и значение из интерфейса.
2. **Из объекта рефлексии — обратно в интерфейс.** `v.Interface()` возвращает `any`, который можно привести к конкретному типу. Это обратная операция к закону 1.
3. **Чтобы менять значение через reflection, оно должно быть settable.** Settable = reflect.Value ссылается на **адресуемую** исходную переменную (получена через указатель) **и** поле/значение экспортируемо. Иначе `Set*` паникует.

```go
x := 3.4
v := reflect.ValueOf(x)           // закон 1
fmt.Println(v.Interface().(float64)) // закон 2 → 3.4
// v.SetFloat(7)                   // закон 3 нарушен: ValueOf(x) не settable → паника
```

---

## reflect.Type и reflect.Value

Две центральные сущности:

```go
import "reflect"

x := 42

t := reflect.TypeOf(x)    // reflect.Type  — описание типа
v := reflect.ValueOf(x)   // reflect.Value — описание значения

fmt.Println(t)            // int
fmt.Println(t.Kind())     // int            (категория)
fmt.Println(t.Name())     // int            (имя типа)
fmt.Println(v.Int())      // 42             (типизированный геттер)
fmt.Println(v.Kind())     // int
```

Типизированные геттеры `Value`: `Int()`, `Uint()`, `Float()`, `String()`, `Bool()`, `Bytes()`, `Len()`, `Index(i)`, `Field(i)`, `MapKeys()`, `Elem()`. Каждый паникует, если `Kind` не подходит (`v.Int()` на строке — паника).

---

## Kind vs Type — не путать

Это частая путаница на собесе. **`Type`** — конкретный именованный тип; **`Kind`** — его базовая категория (одна из фиксированного набора: `Int`, `String`, `Struct`, `Ptr`, `Slice`, `Map`, …).

```go
type Celsius float64
c := Celsius(20)

t := reflect.TypeOf(c)
fmt.Println(t)         // main.Celsius   ← Type (именованный)
fmt.Println(t.Kind())  // float64        ← Kind (категория)
fmt.Println(t.Name())  // Celsius
```

Логика «что это вообще — структура? слайс? указатель?» — всегда через `Kind()`. Логика «это именно `time.Time`?» — через `Type` (сравнение `t == reflect.TypeOf(time.Time{})`). `switch v.Kind()` — основа обхода произвольных значений.

---

## Чтение структуры: поля, теги, методы

```go
type User struct {
    ID    int    `json:"id" validate:"required"`
    Name  string `json:"name"`
    email string // не экспортируемое
}

t := reflect.TypeOf(User{})
for i := 0; i < t.NumField(); i++ {
    f := t.Field(i)                       // reflect.StructField
    fmt.Printf("%s  type=%s  json=%q  validate=%q  exported=%v\n",
        f.Name, f.Type, f.Tag.Get("json"), f.Tag.Get("validate"), f.IsExported())
}
// ID    type=int     json="id"    validate="required"  exported=true
// Name  type=string  json="name"  validate=""          exported=true
// email type=string  json=""      validate=""          exported=false
```

- `t.NumField()` / `t.Field(i)` — поля типа; `f.Tag.Get("json")` парсит struct-тег;
- `f.IsExported()` — экспортируемо ли (важно: reflect видит и приватные поля, но менять/читать-в-интерфейс их нельзя);
- методы: `t.NumMethod()` / `t.Method(i)`; у значения — `v.MethodByName("M")`.

Так устроены `encoding/json` (читает теги `json:"..."`), валидаторы (`validate:"..."`), ORM (`db:"..."`).

---

## Settability: почему `ValueOf(x)` нельзя менять

Главное правило reflection, и главный источник паник. `reflect.ValueOf(x)` получает **копию** значения `x` — менять её бессмысленно (исходная переменная не изменится), поэтому Go запрещает это сразу:

```go
x := 10
v := reflect.ValueOf(x)
fmt.Println(v.CanSet())   // false
// v.SetInt(20)           // panic: reflect: reflect.Value.SetInt using unaddressable value
```

Чтобы менять — нужно дать reflect **адрес** переменной и спуститься к ней через `Elem()`:

```go
x := 10
v := reflect.ValueOf(&x).Elem()  // Elem() разыменовывает указатель
fmt.Println(v.CanSet())          // true
v.SetInt(20)
fmt.Println(x)                   // 20  ← реально изменили исходную переменную
```

Условий settability **два** (оба обязательны):

1. **адресуемость** — Value получена через указатель (`ValueOf(&x).Elem()`), элемент слайса, поле адресуемой структуры;
2. **экспортируемость** — неэкспортируемые поля не settable (и `.Interface()` на них паникует).

```go
type T struct{ Public int; private int }
v := reflect.ValueOf(&T{}).Elem()
v.Field(0).CanSet()  // true  (Public)
v.Field(1).CanSet()  // false (private — даже через указатель)
```

> Перед любым `Set*` проверяй `CanSet()`. Это спасает от паник при обходе произвольных структур.

---

## Изменение значений

```go
type Config struct {
    Host string
    Port int
}

cfg := &Config{}
v := reflect.ValueOf(cfg).Elem()

v.FieldByName("Host").SetString("localhost")  // типизированный сеттер
v.FieldByName("Port").SetInt(8080)

// Универсальный Set через reflect.Value (типы должны совпадать)
v.FieldByName("Port").Set(reflect.ValueOf(9090))

fmt.Println(cfg) // &{localhost 9090}
```

Сеттеры: `SetInt`, `SetString`, `SetBool`, `SetFloat`, `Set(reflect.Value)`. `Set` требует **точного совпадения типов** — `SetInt` в поле `int32` через `Set(ValueOf(int64))` паникнет (нужен `int32`).

---

## Вызов методов и функций

```go
type Calc struct{}
func (Calc) Add(a, b int) int { return a + b }

v := reflect.ValueOf(Calc{})
m := v.MethodByName("Add")
args := []reflect.Value{reflect.ValueOf(3), reflect.ValueOf(4)}
out := m.Call(args)          // []reflect.Value
fmt.Println(out[0].Int())    // 7
```

`Call` принимает `[]reflect.Value` (по числу и типам параметров) и возвращает `[]reflect.Value` (результаты). Так работают RPC-фреймворки и DI-контейнеры, вызывающие хендлеры по имени. Дорого и без compile-time проверок — только когда сигнатура неизвестна заранее.

---

## Создание значений: New, MakeSlice, MakeMap

```go
t := reflect.TypeOf(User{})

p := reflect.New(t)        // *User (как new(User)) — Value типа *User
p.Elem().FieldByName("Name").SetString("Bob")
u := p.Elem().Interface().(User)  // достать готовое значение
fmt.Println(u)             // {0 Bob }

// Слайсы/мапы нужного типа
st := reflect.SliceOf(t)                       // []User
s := reflect.MakeSlice(st, 0, 10)
m := reflect.MakeMap(reflect.MapOf(
    reflect.TypeOf(""), reflect.TypeOf(0)))     // map[string]int
```

`reflect.New(t)` — основа десериализаторов: по `reflect.Type` создать пустое значение и заполнить поля из входных данных.

---

## reflect.DeepEqual

Глубокое сравнение произвольных значений (когда `==` неприменим — слайсы, мапы, или несравнимые типы внутри):

```go
reflect.DeepEqual([]int{1, 2}, []int{1, 2})           // true
reflect.DeepEqual(map[string]int{"a": 1}, map[string]int{"a": 1}) // true
```

Подводные камни:
- **медленный** (reflection) — не для горячего пути; в тестах ок;
- `DeepEqual(nil-slice, empty-slice)` → **false** (`[]int(nil)` ≠ `[]int{}`);
- для float `NaN != NaN` → две одинаковые структуры с `NaN` не равны;
- в новом коде для слайсов/мап лучше `slices.Equal`/`maps.Equal` (быстрее, без reflection), а `DeepEqual` — для произвольно вложенных структур.

---

## Где reflect используется

- **`encoding/json`, `encoding/xml`, `gopkg.in/yaml`** — читают теги и поля, заполняют значения;
- **ORM / query builders** (`gorm`, `sqlx`) — маппинг строк БД на поля по тегам `db:"..."`;
- **валидаторы** (`go-playground/validator`) — правила в тегах `validate:"..."`;
- **`fmt`** — `%v`/`%+v` обходят структуры рефлексией;
- **DI-контейнеры, RPC, мапперы** (`mapstructure`, `copier`) — вызов методов и копирование полей по имени;
- **`reflect.DeepEqual`** — в тестах (`testify` под капотом).

Общий признак: код должен работать с типами, **неизвестными на этапе его написания**.

---

## Производительность: цена reflection

reflect обходит статическую типизацию → платит за это. Бенчмарк доступа к полю `int` структуры (`go1.26`, arm64):

```
BenchmarkDirectGet-16    0.27 ns/op   0 allocs   // t.A
BenchmarkReflectGet-16   2.00 ns/op   0 allocs   // reflect.ValueOf(t).Field(0).Int()  (~7×)
BenchmarkDirectSet-16    0.26 ns/op   0 allocs   // t.A = 7
BenchmarkReflectSet-16   3.80 ns/op   0 allocs   // ValueOf(&t).Elem().Field(0).SetInt(7) (~14×)
```

- доступ через reflect — в разы дороже прямого (тут ×7…×14; на сложных обходах разрыв растёт);
- **`reflect.ValueOf(x)` боксирует** `x` в интерфейс, а `v.Interface()` — обратно: это аллокации для value-типов на горячем пути;
- поэтому продакшн-библиотеки **кэшируют** результат рефлексии: разобрали `reflect.Type` структуры один раз (поля, теги, оффсеты) → дальше работают по кэшу (так делает быстрый json через кодген/кэш-метаданные).

Вывод: reflect — для гибкости там, где тип неизвестен; на горячем пути его прячут за кэш или заменяют кодогенерацией/дженериками.

---

## Generics часто заменяют reflect

До Go 1.18 reflection использовали и ради «обобщённого» кода (контейнеры, утилиты над любыми типами). Теперь это делается **дженериками** — типобезопасно, без рантайм-цены и паник:

```go
// Было: reflect для «универсального» Contains (медленно, runtime-паники)
// Стало: generic — compile-time, 0 аллокаций
func Contains[T comparable](s []T, v T) bool { /* ... */ }
```

reflect остаётся нужен там, где тип реально **неизвестен в компиляции**: разбор внешних данных (JSON в `map[string]any`), маппинг по тегам, вызов методов по имени. «Обобщить алгоритм над типами» — это дженерики, а не reflect.

---

## Подводные камни (паники)

reflect щедр на рантайм-паники — почти каждый неверный вызов падает:

- `Set*` на не-settable значении → `using unaddressable value` / `using value obtained from unexported field`;
- типизированный геттер на чужом Kind: `v.Int()` при `Kind()==String` → `call of reflect.Value.Int on string Value`;
- `Field(i)` на не-структуре, `Index(i)` на не-слайсе/массиве;
- `.Interface()` на неэкспортируемом поле → паника;
- `Call` с неверным числом/типами аргументов;
- `Elem()` на не-указателе/не-интерфейсе.

Защита: проверять `Kind()` перед геттерами и `CanSet()`/`CanInterface()` перед изменением/извлечением. По возможности — не использовать reflect вовсе.

---

## Разбор примеров-загадок

### Загадка 1: почему `SetInt` паникует

```go
x := 10
reflect.ValueOf(x).SetInt(20)  // ?
```

<details>
<summary>Ответ</summary>

```
panic: reflect: reflect.Value.SetInt using unaddressable value
```

`ValueOf(x)` получает **копию** `x`, она не адресуема → не settable. Менять можно только через адрес: `reflect.ValueOf(&x).Elem().SetInt(20)`. Это закон 3 рефлексии и причина, по которой десериализаторы принимают `&v`, а не `v`.
</details>

---

### Загадка 2: Kind vs Type

```go
type ID int64
var x ID = 5
t := reflect.TypeOf(x)
fmt.Println(t.Name(), t.Kind())  // ?
```

<details>
<summary>Ответ</summary>

```
ID int64
```

`Name()` — именованный тип (`ID`), `Kind()` — базовая категория (`int64`). Обход «это число?» делают по `Kind`, а «это именно `ID`?» — по `Type`. Их путаница — классическая ошибка: например, `switch t.Name()` сломается для анонимных типов (у них `Name()` пустой), а `switch t.Kind()` — нет.
</details>

---

### Загадка 3: неэкспортируемое поле

```go
type T struct{ x int }
v := reflect.ValueOf(&T{x: 5}).Elem()
fmt.Println(v.Field(0).Int())        // (1) ?
fmt.Println(v.Field(0).Interface()) // (2) ?
```

<details>
<summary>Ответ</summary>

```
(1) 5
(2) panic: reflect.Value.Interface: cannot return value obtained from unexported field or method
```

Прочитать значение неэкспортируемого поля типизированным геттером (`Int()`) — можно. А вот «вынести его в интерфейс» через `.Interface()` — паника: это нарушило бы инкапсуляцию (код снаружи получил бы приватные данные). Менять (`Set*`) приватное поле тоже нельзя. Поэтому json и подобные работают только с экспортируемыми полями.
</details>

---

### Загадка 4: DeepEqual и nil vs empty

```go
fmt.Println(reflect.DeepEqual([]int(nil), []int{}))  // ?
```

<details>
<summary>Ответ</summary>

```
false
```

`DeepEqual` различает nil-слайс и пустой не-nil слайс — у них разное «состояние», хоть оба длины 0. Частый сюрприз в тестах: ожидали `[]int{}`, а функция вернула `nil` → тест падает. Либо приводить к одному виду, либо использовать `slices.Equal` (он считает nil и пустой равными по элементам — оба «нет элементов»).
</details>

---

## Interview-ready answer

**1. На чём построен reflect?**

- На интерфейсе: значение упаковывается в `any`, у которого в рантайме есть дескриптор типа (`_type`) и указатель на данные. `reflect.Type` — обёртка над `_type`, `reflect.Value` — `_type` + данные + флаги (адресуемость/экспортируемость). Поэтому reflect видит динамический тип и работает только с тем, что попало в интерфейс.

**2. Три закона рефлексии?**

- (1) из интерфейса в reflect-объект (`ValueOf`/`TypeOf`); (2) обратно (`v.Interface()`); (3) менять значение можно, только если оно **settable** — получено через указатель (адресуемо) и экспортируемо. Иначе `Set*` паникует.

**3. Почему `reflect.ValueOf(x).SetInt(...)` падает?**

- `ValueOf(x)` берёт копию — менять её бессмысленно, значение не адресуемо. Нужно `reflect.ValueOf(&x).Elem().SetInt(...)`. Перед `Set*` проверять `CanSet()`.

**4. Kind vs Type?**

- `Type` — конкретный именованный тип (`main.Celsius`), `Kind` — базовая категория (`float64`, `struct`, `slice`…). «Что это структурно» — по `Kind`; «это именно такой-то тип» — по `Type`.

**5. Чем reflect плох и чем заменить?**

- Медленный (в разы дороже прямого доступа), аллоцирует (боксинг в/из интерфейса), даёт рантайм-паники вместо compile-time ошибок. Заменяют: дженериками (если нужно «обобщить алгоритм»), кодогенерацией или кэшированием метаданных (как быстрый json). reflect оправдан только когда тип реально неизвестен в компиляции — разбор внешних данных, маппинг по тегам, вызов по имени.
