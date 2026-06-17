# encoding/json: сериализация и её ловушки

`encoding/json` — один из самых используемых пакетов stdlib и одновременно источник классических собеседных вопросов. Большинство ловушек растёт из трёх фактов: маршалинг идёт через **reflection** и видит только **экспортируемые** поля, числа в `interface{}` всегда становятся `float64`, а `omitempty` понимает «пустоту» уже, чем кажется.

## Содержание

- [Базовый API: Marshal / Unmarshal](#базовый-api-marshal--unmarshal)
- [Теги структуры](#теги-структуры)
- [omitempty и что считается «пустым»](#omitempty-и-что-считается-пустым)
- [Декодирование в interface{}: числа становятся float64](#декодирование-в-interface-числа-становятся-float64)
- [Кастомная сериализация: Marshaler / Unmarshaler](#кастомная-сериализация-marshaler--unmarshaler)
- [Частичный и отложенный разбор: RawMessage](#частичный-и-отложенный-разбор-rawmessage)
- [Потоковая обработка: Encoder / Decoder](#потоковая-обработка-encoder--decoder)
- [Строгий разбор: DisallowUnknownFields, Number](#строгий-разбор-disallowunknownfields-number)
- [HTML-экранирование](#html-экранирование)
- [Производительность и аллокации](#производительность-и-аллокации)
- [encoding/json/v2 (экспериментальный)](#encodingjsonv2-экспериментальный)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

---

## Базовый API: Marshal / Unmarshal

```go
type User struct {
    Name  string `json:"name"`
    Email string `json:"email"`
    Age   int    `json:"age"`
}

// Маршалинг: Go → JSON
u := User{Name: "Alice", Email: "a@x.io", Age: 30}
b, err := json.Marshal(u)             // {"name":"Alice","email":"a@x.io","age":30}

// Анмаршалинг: JSON → Go
var u2 User
err = json.Unmarshal(b, &u2)          // ОБЯЗАТЕЛЬНО указатель
```

Два правила, на которых горят чаще всего:

1. **Сериализуются только экспортируемые поля.** Поле с маленькой буквы (`name string`) недоступно через reflection — оно просто молча не попадёт в JSON и не заполнится при разборе. Ошибки не будет.
2. **`Unmarshal` требует указатель.** В non-pointer reflection не может изменить значение → `json: Unmarshal(non-pointer ...)`.

```go
type bad struct {
    name string `json:"name"` // ❌ не экспортируемое — игнорируется молча
}
```

---

## Теги структуры

```go
type Product struct {
    ID        int     `json:"id"`
    Title     string  `json:"title,omitempty"`   // пропустить если пусто
    Price     float64 `json:"price,string"`       // закодировать число как строку
    Internal  string  `json:"-"`                  // никогда не сериализовать
    Dash      string  `json:"-,"`                 // поле с буквальным именем "-"
    secret    string                              // не экспортируемое → игнор
}
```

| Тег | Эффект |
|---|---|
| `json:"name"` | переименовать поле в `name` |
| `json:"name,omitempty"` | пропустить, если значение «пустое» |
| `json:"-"` | полностью исключить поле |
| `json:"-,"` | имя поля — буквально `-` |
| `json:",omitempty"` | оставить имя поля как есть, добавить omitempty |
| `json:"name,string"` | число/bool закодировать как JSON-строку |

Опция `,string` работает только для чисел, булевых и строк — частый приём, когда фронт или другой язык теряет точность на больших `int64` (JSON-число — это float64 у многих парсеров).

---

## omitempty и что считается «пустым»

`omitempty` пропускает поле, если его значение равно **zero value** базового типа:

| Тип | «Пусто» для omitempty |
|---|---|
| числа | `0` |
| string | `""` |
| bool | `false` |
| указатель, interface | `nil` |
| slice, map | `nil` **или** длина `0` |
| **struct** | **никогда** (нет понятия «пустой struct») |
| array `[N]T` | **никогда** (длина фиксирована) |

Главная ловушка — **вложенная структура не убирается** через `omitempty`:

```go
type Address struct {
    City string `json:"city,omitempty"`
}
type User struct {
    Name string  `json:"name"`
    Addr Address `json:"addr,omitempty"` // ❌ omitempty НЕ работает
}

json.Marshal(User{Name: "Bob"})
// {"name":"Bob","addr":{}}  ← addr остался, хоть и пустой
```

Решения:
- сделать поле **указателем** `*Address` — тогда `nil` действительно опускается;
- встроить (embed) и/или использовать кастомный `MarshalJSON`;
- в Go 1.24+ доступен тег **`omitzero`** — он понимает «нулевую» структуру (и уважает метод `IsZero()`), в отличие от `omitempty`.

```go
type User struct {
    Name string  `json:"name"`
    Addr Address `json:"addr,omitzero"` // ✅ Go 1.24+: пустой struct опускается
}
```

Ещё нюанс: `omitempty` НЕ различает «поле отсутствует» и «поле = zero value». Чтобы поймать «ноль пришёл явно vs не пришёл вовсе», нужен указатель (`*int`: `nil` = нет поля, `&0` = пришёл ноль).

---

## Декодирование в interface{}: числа становятся float64

При разборе в `interface{}` / `map[string]interface{}` JSON-типы маппятся фиксированно:

| JSON | Go |
|---|---|
| object | `map[string]interface{}` |
| array | `[]interface{}` |
| string | `string` |
| number | **`float64`** (всегда!) |
| bool | `bool` |
| null | `nil` |

```go
var m map[string]interface{}
json.Unmarshal([]byte(`{"id": 42}`), &m)

id := m["id"].(int)      // ❌ panic: interface conversion: interface {} is float64
id := int(m["id"].(float64)) // ✅
```

Это классическая ловушка: число всегда `float64`, даже если в JSON оно выглядит целым. Для больших `int64` это к тому же теряет точность (float64 точен только до 2⁵³). Решения — разбирать в конкретную структуру (`int64` поле) или использовать `json.Number` (см. ниже).

---

## Кастомная сериализация: Marshaler / Unmarshaler

Тип сам управляет своим JSON-представлением, реализуя интерфейсы:

```go
type Marshaler interface   { MarshalJSON() ([]byte, error) }
type Unmarshaler interface { UnmarshalJSON([]byte) error }
```

Частый кейс — своё представление времени/денег/enum:

```go
type Money int64 // храним копейки

func (m Money) MarshalJSON() ([]byte, error) {
    return []byte(fmt.Sprintf(`"%d.%02d"`, m/100, m%100)), nil
}

func (m *Money) UnmarshalJSON(b []byte) error {
    s := strings.Trim(string(b), `"`)
    // ... распарсить "12.34" в копейки
    return nil
}
```

Два подводных камня:
- `MarshalJSON` обычно на **значении** (`func (m Money)`), `UnmarshalJSON` — всегда на **указателе** (`func (m *Money)`), иначе изменения не сохранятся;
- внутри `MarshalJSON` нельзя делать `json.Marshal(m)` для того же типа без алиаса — будет **бесконечная рекурсия** (см. загадку 4).

`encoding.TextMarshaler`/`TextUnmarshaler` — родственный механизм: если тип реализует их, json использует текстовое представление (так работает `time.Time`, который маршалится в RFC 3339).

---

## Частичный и отложенный разбор: RawMessage

`json.RawMessage` — это `[]byte`, который json **не трогает**: откладывает разбор/сборку на потом. Полезно для полиморфных сообщений и проксирования.

```go
type Envelope struct {
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"` // сырой кусок, разберём позже
}

var e Envelope
json.Unmarshal(data, &e)

switch e.Type {
case "user":
    var u User
    json.Unmarshal(e.Payload, &u) // разбираем только нужную ветку
case "order":
    var o Order
    json.Unmarshal(e.Payload, &o)
}
```

На маршалинг тоже работает: положить уже готовый JSON в поле без повторной сериализации.

---

## Потоковая обработка: Encoder / Decoder

Для потоков (HTTP body, файлы, NDJSON) — `Encoder`/`Decoder`, работающие напрямую с `io.Reader`/`io.Writer`, без промежуточного `[]byte`:

```go
// Чтение: прямо из тела запроса
func handler(w http.ResponseWriter, r *http.Request) {
    var req CreateReq
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "bad json", http.StatusBadRequest)
        return
    }
    // Запись: прямо в ответ
    json.NewEncoder(w).Encode(resp) // добавляет '\n' в конце
}
```

`Decoder` умеет читать **поток из нескольких JSON-значений** подряд (streaming):

```go
dec := json.NewDecoder(r) // r содержит {…}{…}{…} или построчный NDJSON
for {
    var v Event
    if err := dec.Decode(&v); err == io.EOF {
        break
    } else if err != nil {
        return err
    }
    process(v)
}
```

> ⚠️ `json.NewEncoder(w).Encode(v)` добавляет завершающий `\n`, а `json.Marshal` — нет. Это иногда ломает побайтовое сравнение в тестах.

---

## Строгий разбор: DisallowUnknownFields, Number

По умолчанию json **прощает многое**: лишние поля игнорируются, чисел хватает float64. Для строгого/безопасного разбора:

```go
dec := json.NewDecoder(r.Body)
dec.DisallowUnknownFields()  // ошибка на неизвестных полях (защита от опечаток в API)
dec.UseNumber()              // числа → json.Number вместо float64

var req Request
err := dec.Decode(&req)
```

`json.Number` — это строка, которую можно превратить в нужный тип без потери точности:

```go
var m map[string]interface{}
dec := json.NewDecoder(strings.NewReader(`{"id": 9007199254740993}`))
dec.UseNumber()
dec.Decode(&m)

n := m["id"].(json.Number)
i, _ := n.Int64()  // точный int64, без потери на float64
```

---

## HTML-экранирование

`json.Marshal` по умолчанию экранирует `<`, `>`, `&` в `<` и т.п. — чтобы безопасно встраивать JSON в HTML/`<script>`:

```go
json.Marshal("a<b>c & d")
// "a<b>c & d"
```

Если JSON идёт не в HTML (например, в другой сервис), это засоряет вывод. Отключается только через `Encoder`:

```go
var buf bytes.Buffer
enc := json.NewEncoder(&buf)
enc.SetEscapeHTML(false)
enc.Encode("a<b>c & d")  // "a<b>c & d"
```

---

## Производительность и аллокации

`encoding/json` основан на reflection — он удобен, но не самый быстрый. Что важно знать на собесе:

- **Reflection-стоимость.** На горячем пути (десятки тысяч RPS) reflection заметен. Альтернативы: `github.com/json-iterator/go`, `github.com/goccy/go-json`, кодогенерация (`easyjson`, `ffjson`), `encoding/json/v2`.
- **`Decoder` vs `Unmarshal` для потоков.** Для больших/потоковых данных `Decoder` не держит весь вход в памяти.
- **Переиспользование буферов.** В кастомных `MarshalJSON` берите буфер из `sync.Pool`, не аллоцируйте на каждый вызов.
- **`omitempty` не бесплатен** на маршалинге — проверка «пусто ли» делается рефлексией для каждого помеченного поля.

---

## encoding/json/v2 (экспериментальный)

В Go 1.25 доступен `encoding/json/v2` под флагом `GOEXPERIMENT=jsonv2` — переработанный пакет, который чинит исторические грабли:

- значительно **быстрее** (новый decoder/encoder, меньше аллокаций);
- более предсказуемая семантика `omitempty`/`omitzero`, явные опции через `json.Options`;
- честная потоковая обработка через `jsontext`;
- v1 остаётся стабильным; v2 — путь на будущее, API ещё может меняться.

Знать о его существовании полезно: на собеседовании это сигнал, что кандидат следит за развитием экосистемы и понимает, какие именно недостатки v1 он исправляет.

---

## Разбор примеров-загадок

### Загадка 1: не экспортируемое поле молча теряется

```go
type Config struct {
    Host string `json:"host"`
    port int    `json:"port"` // маленькая буква
}

c := Config{Host: "localhost", port: 8080}
b, _ := json.Marshal(c)
fmt.Println(string(b)) // ?
```

<details>
<summary>Ответ</summary>

```
{"host":"localhost"}
```

`port` не экспортируется → reflection его не видит → поле не сериализуется, и **никакой ошибки нет**. При разборе оно тоже останется нулевым. Тег `json:"port"` на не экспортируемом поле бесполезен. Это типичный источник «почему поле не приходит» — первым делом проверяй заглавную букву.
</details>

---

### Загадка 2: число из interface{} — это float64

```go
data := []byte(`{"count": 5}`)
var m map[string]interface{}
json.Unmarshal(data, &m)

sum := m["count"].(int) + 1
fmt.Println(sum) // ?
```

<details>
<summary>Ответ</summary>

```
panic: interface conversion: interface {} is float64, not int
```

Любое JSON-число при разборе в `interface{}` становится `float64`, даже целое `5`. Нужно `int(m["count"].(float64))`, либо разбирать в типизированную структуру, либо `UseNumber()` + `json.Number`. На больших `int64` float64 ещё и теряет точность.
</details>

---

### Загадка 3: omitempty не убирает вложенный struct

```go
type Inner struct {
    X int `json:"x,omitempty"`
}
type Outer struct {
    Inner Inner `json:"inner,omitempty"`
}

b, _ := json.Marshal(Outer{})
fmt.Println(string(b)) // ?
```

<details>
<summary>Ответ</summary>

```
{"inner":{}}
```

`omitempty` не умеет считать struct «пустым» — у структуры нет zero-значения в смысле omitempty. Поле остаётся, внутри `x` опускается (там omitempty работает, int=0). Чтобы убрать `inner` целиком — сделать поле `*Inner` (тогда `nil` опустится) или использовать `omitzero` (Go 1.24+).
</details>

---

### Загадка 4: бесконечная рекурсия в MarshalJSON

```go
type Temp float64

func (t Temp) MarshalJSON() ([]byte, error) {
    return json.Marshal(t) // ?
}

json.Marshal(Temp(20.5))
```

<details>
<summary>Ответ</summary>

```
stack overflow (бесконечная рекурсия)
```

`json.Marshal(t)` внутри `MarshalJSON` для того же типа снова вызывает `MarshalJSON` → бесконечный цикл. Лекарство — **тип-алиас без методов**, который не наследует `MarshalJSON`:

```go
func (t Temp) MarshalJSON() ([]byte, error) {
    type alias Temp                 // у alias нет метода MarshalJSON
    return json.Marshal(alias(t))   // обычная сериализация числа
}
```

Тот же приём с `type alias` нужен в `UnmarshalJSON`, когда хочется «доразобрать как обычно, но потом подправить».
</details>

---

### Загадка 5: ноль пришёл или поля не было?

```go
type Patch struct {
    Active bool `json:"active,omitempty"`
}

var p Patch
json.Unmarshal([]byte(`{"active": false}`), &p)
fmt.Println(p.Active) // ?
// а как отличить это от {} ?
```

<details>
<summary>Ответ</summary>

```
false
```

`p.Active` = `false` — но **ровно то же** получится и из `{}`, где поля нет вовсе. `omitempty` + значимый тип не различают «пришёл явный ноль/false» и «не пришло». Для PATCH-семантики (обновить только присланные поля) нужен указатель:

```go
type Patch struct {
    Active *bool `json:"active"` // nil = не прислали, &false = прислали false
}
```
</details>

---

## Interview-ready answer

**1. Почему поле не попадает в JSON / не заполняется?**

- Скорее всего оно не экспортируемое (с маленькой буквы) — reflection его не видит, и это происходит молча, без ошибки. Также `Unmarshal` требует указатель, иначе он не сможет ничего записать.

**2. Почему число из `map[string]interface{}` нельзя привести к int?**

- Любое JSON-число при разборе в `interface{}` становится `float64`. Нужно `int(v.(float64))`, типизированная структура или `Decoder.UseNumber()` + `json.Number` (последнее — без потери точности для больших int64).

**3. Что именно считается «пустым» для omitempty?**

- Zero value: `0`, `""`, `false`, `nil`, пустые/nil slice и map. Вложенный **struct и array omitempty не убирает** — нужен указатель или тег `omitzero` (Go 1.24+). И omitempty не отличает «явный ноль» от «поля не было» — для этого указатель.

**4. Как разобрать полиморфный/частичный JSON?**

- `json.RawMessage` — отложить разбор куска (например, `Payload` по дискриминатору `Type`), потом `Unmarshal` нужной ветки. Кастомные `MarshalJSON`/`UnmarshalJSON` — для своего представления; внутри использовать `type alias`, чтобы не словить бесконечную рекурсию.

**5. Marshal vs Encoder?**

- `Marshal` возвращает `[]byte` (удобно, но держит весь результат в памяти и не добавляет `\n`). `Encoder`/`Decoder` работают потоково с `io.Reader/Writer`, умеют `DisallowUnknownFields`, `SetEscapeHTML(false)`, поток из нескольких значений. Для HTTP-хендлеров обычно берут `Decoder`/`Encoder`.

**6. Производительность?**

- `encoding/json` основан на reflection и не самый быстрый. На горячем пути — кодогенерация (`easyjson`), сторонние парсеры (`goccy/go-json`, `json-iterator`) или `encoding/json/v2` (экспериментальный, Go 1.25). Потоки — через `Decoder`, буферы в кастомных маршалерах — через `sync.Pool`.
