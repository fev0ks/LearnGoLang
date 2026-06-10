# Пакет `strings`: основные функции

Этот файл — практический справочник по пакету `strings`: что есть, когда брать, где ловушки. Про **устройство** самого типа `string` (header, immutability, byte vs rune, `Builder`, сравнение и `collate`, `unsafe`-конверсии) — в [01-go-core/07-strings](../01-go-core/07-strings.md); здесь не дублируем, а ссылаемся.

Ключевая вещь, из которой растут все «странности»: строка — это **UTF-8 байты**, поэтому функции с «индексами» возвращают **смещение в байтах**, а не номер символа.

## Содержание

- [Поиск и проверка вхождения](#поиск-и-проверка-вхождения)
- [Префиксы и суффиксы](#префиксы-и-суффиксы)
- [Разбиение и склейка](#разбиение-и-склейка)
- [Замена](#замена)
- [Регистр](#регистр)
- [Обрезка (trim)](#обрезка-trim)
- [Построение и повтор](#построение-и-повтор)
- [Итераторы (Go 1.24+)](#итераторы-go-124)
- [Частые ошибки](#частые-ошибки)
- [Interview-ready answer](#interview-ready-answer)

---

## Поиск и проверка вхождения

Две группы: `Contains*` отвечают «есть ли» (`bool`), `Index*` — «где» (байтовое смещение или `-1`).

| Функция | Что делает | Возвращает |
|---|---|---|
| `Contains(s, substr)` | есть ли подстрока | `bool` |
| `ContainsAny(s, chars)` | есть ли **любой** из рун `chars` | `bool` |
| `ContainsRune(s, r)` | есть ли руна `r` | `bool` |
| `ContainsFunc(s, f)` | есть ли руна, для которой `f(r)==true` | `bool` |
| `Index(s, substr)` | смещение первого вхождения | `int` (байты) или `-1` |
| `LastIndex(s, substr)` | смещение последнего вхождения | `int` или `-1` |
| `IndexByte(s, b)` | первый байт `b` | `int` или `-1` |
| `IndexRune(s, r)` | первая руна `r` | `int` или `-1` |
| `IndexAny(s, chars)` | первая из рун `chars` | `int` или `-1` |
| `IndexFunc(s, f)` | первая руна с `f(r)==true` | `int` или `-1` |
| `Count(s, substr)` | число непересекающихся вхождений | `int` |

```go
strings.Contains("seafood", "foo")   // true
strings.Index("chicken", "ken")      // 4  — байтовое смещение
strings.Index("chicken", "dmr")      // -1 — нет
strings.Count("cheese", "e")         // 3
strings.Count("five", "")            // 5  — пустая подстрока: len(s)+1 (между байтами)
```

⚠️ `Index` — это **байты**, а не позиция символа. На не-ASCII смещение не равно номеру руны:

```go
strings.Index("абвг", "в")  // 4, а не 2 — каждая буква по 2 байта
```

Если нужен номер символа — переводи в `[]rune` (см. [07-strings](../01-go-core/07-strings.md#конверсии-string--byte--rune)).

---

## Префиксы и суффиксы

| Функция | Делает |
|---|---|
| `HasPrefix(s, p)` / `HasSuffix(s, suf)` | проверка начала/конца → `bool` |
| `TrimPrefix(s, p)` / `TrimSuffix(s, suf)` | убрать префикс/суффикс, **если он есть** (иначе вернуть `s` как есть) |
| `CutPrefix(s, p)` / `CutSuffix(s, suf)` (Go 1.20+) | то же, но возвращает `(string, found bool)` |

```go
strings.TrimPrefix("https://example.com", "https://") // "example.com"
strings.TrimPrefix("example.com", "https://")          // "example.com" — префикса нет, без изменений

after, ok := strings.CutPrefix("v1.2.3", "v")          // after="1.2.3", ok=true
```

`CutPrefix` удобнее, когда важно **знать**, был ли префикс (например, разный разбор для `v`-тегов и без).

---

## Разбиение и склейка

| Функция | Делает | Нюанс |
|---|---|---|
| `Split(s, sep)` | разрезать по `sep` | пустой `sep` → срез **рун**; пустые части сохраняются |
| `SplitN(s, sep, n)` | максимум `n` частей (последняя — остаток) | `n<0` = без лимита, `n==0` = `nil` |
| `SplitAfter(s, sep)` | как `Split`, но `sep` остаётся в конце частей | |
| `Cut(s, sep)` (Go 1.18+) | разрез на **первом** `sep` → `(before, after, found)` | идиома для key=value |
| `Fields(s)` | разрезать по **любым** пробелам (`unicode.IsSpace`), пустые отбрасываются | |
| `FieldsFunc(s, f)` | разрезать там, где `f(r)==true` | |
| `Join(elems, sep)` | склеить срез через `sep` | |

```go
strings.Split("a,b,c", ",")      // ["a" "b" "c"]
strings.Split("a,,c", ",")       // ["a" "" "c"]  — пустая часть сохраняется
strings.Split("абв", "")         // ["а" "б" "в"] — по рунам, не по байтам
strings.SplitN("a,b,c,d", ",", 2)// ["a" "b,c,d"]

strings.Fields("  foo   bar ")   // ["foo" "bar"]  — схлопывает пробелы, без пустых

k, v, ok := strings.Cut("key=value", "=")  // "key", "value", true
host, port, ok := strings.Cut("host:8080", ":")
```

**`Split` vs `Fields` — частая путаница:**

```go
strings.Split("a  b", " ")  // ["a" "" "b"]  — двойной пробел даёт пустую часть
strings.Fields("a  b")      // ["a" "b"]      — пробелы схлопнуты, пустых нет
```

Берёшь `Fields`, когда режешь «текст по словам»; `Split(s, " ")` — когда разделитель строго один символ и пустые поля значимы (например, CSV-подобное).

---

## Замена

| Функция | Делает |
|---|---|
| `Replace(s, old, new, n)` | заменить первые `n` вхождений (`n<0` — все) |
| `ReplaceAll(s, old, new)` | заменить все (= `Replace` с `n=-1`) |
| `NewReplacer(pairs...).Replace(s)` | много замен **за один проход** |
| `Map(f, s)` | заменить/удалить **каждую руну** функцией |

```go
strings.Replace("oink oink oink", "k", "ky", 2) // "oinky oinky oink"
strings.ReplaceAll("a-b-c", "-", "_")            // "a_b_c"

// NewReplacer — эффективнее цепочки ReplaceAll, потокобезопасен, создавай один раз
r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
r.Replace("a < b & c")  // "a &lt; b &amp; c"

// Map — вернуть -1, чтобы удалить руну
strings.Map(func(r rune) rune {
    if unicode.IsDigit(r) { return -1 } // выкинуть все цифры
    return r
}, "a1b2c3")  // "abc"
```

`NewReplacer` берут, когда замен несколько: он проходит строку один раз, тогда как `n` вызовов `ReplaceAll` — это `n` проходов и `n` промежуточных строк.

---

## Регистр

| Функция | Делает |
|---|---|
| `ToUpper(s)` / `ToLower(s)` | смена регистра (Unicode-aware) |
| `ToTitle(s)` | все руны в title-case (не «слова с заглавной»!) |
| `EqualFold(a, b)` | регистронезависимое сравнение → `bool` |
| ~~`Title(s)`~~ | **deprecated** — неверно работает с Unicode-словами |

```go
strings.ToUpper("straße")          // "STRASSE" — ß раскрывается
strings.EqualFold("Go", "GO")      // true — правильный способ сравнить без регистра
```

**Регистронезависимое сравнение — только `EqualFold`**, а не `ToLower(a) == ToLower(b)`: `EqualFold` корректно обрабатывает спецслучаи (тот же `ß`, греческая сигма) и не аллоцирует две новые строки.

`Title` устарел (ломается на апострофах и т.п.) — для «Заглавных Слов» бери `golang.org/x/text/cases`:

```go
import ("golang.org/x/text/cases"; "golang.org/x/text/language")
cases.Title(language.Russian).String("привет мир") // "Привет Мир"
```

---

## Обрезка (trim)

⚠️ Главная ловушка: `Trim`/`TrimLeft`/`TrimRight` принимают **cutset — набор рун**, а не строку-префикс.

| Функция | Делает |
|---|---|
| `TrimSpace(s)` | убрать пробельные с обоих концов (`unicode.IsSpace`) |
| `Trim(s, cutset)` | убрать с обоих концов **любые руны из** `cutset` |
| `TrimLeft(s, cutset)` / `TrimRight(s, cutset)` | то же с одной стороны |
| `TrimFunc(s, f)` / `TrimLeftFunc` / `TrimRightFunc` | обрезать руны, пока `f(r)==true` |
| `TrimPrefix` / `TrimSuffix` | убрать **строку целиком** (см. выше) |

```go
strings.TrimSpace("  hi \n")     // "hi"
strings.Trim("xxhixx", "x")      // "hi"
strings.Trim("[1,2,3]", "[]")    // "1,2,3" — cutset = {'[', ']'}

// ⚠️ cutset — это МНОЖЕСТВО символов, а не подстрока:
strings.Trim("mississippi", "ips") // "m" — съело все i/p/s с концов!
strings.TrimRight("hello", "lo")   // "he" — убрало и 'l', и 'o'

// Чтобы убрать именно строку целиком — TrimSuffix:
strings.TrimSuffix("report.txt", ".txt") // "report"
```

Если видишь `Trim(path, "/api")` в коде — почти всегда баг: оно срежет с концов любые из `/`,`a`,`p`,`i`. Нужен `TrimPrefix(path, "/api")`.

---

## Построение и повтор

| Функция / тип | Делает |
|---|---|
| `Repeat(s, count)` | повторить строку `count` раз |
| `strings.Builder` | эффективная сборка строки без лишних копий |

```go
strings.Repeat("ab", 3)  // "ababab"
```

Для конкатенации в цикле — `strings.Builder` (амортизированный `O(n)` вместо `O(n²)` у `+=`), детали и бенчмарк в [07-strings, Builder](../01-go-core/07-strings.md#конкатенация-в-цикле-on²-vs-stringsbuilder).

---

## Итераторы (Go 1.24+)

С Go 1.23 (`range`-over-func) и 1.24 у `strings` появились ленивые версии split-функций — отдают `iter.Seq[string]` и не аллоцируют промежуточный срез:

| Итератор | Аналог-«срез» |
|---|---|
| `Lines(s)` | разбивка по строкам (с сохранением `\n`) |
| `SplitSeq(s, sep)` | `Split` |
| `SplitAfterSeq(s, sep)` | `SplitAfter` |
| `FieldsSeq(s)` | `Fields` |
| `FieldsFuncSeq(s, f)` | `FieldsFunc` |

```go
for line := range strings.Lines("a\nb\nc\n") {
    fmt.Print(line) // "a\n", "b\n", "c\n"
}

for word := range strings.FieldsSeq("  foo  bar ") {
    fmt.Println(word) // foo, bar — без аллокации []string
}
```

Берут на больших входах/горячем пути, где не нужен весь срез сразу: экономят аллокацию слайса и позволяют `break` раньше времени.

---

## Частые ошибки

- **`Index`/`Count` считают в байтах**, а не в символах — на не-ASCII смещения не совпадают с номерами рун.
- **`Trim(s, cutset)` — это множество рун, а не префикс.** Для срезания строки целиком — `TrimPrefix`/`TrimSuffix`.
- **`Split(s, " ")` ≠ `Fields(s)`**: первый сохраняет пустые части на повторных пробелах, второй схлопывает.
- **Регистронезависимое сравнение — `EqualFold`**, не `ToLower(a)==ToLower(b)` (корректность + 0 аллокаций).
- **`Title` deprecated** → `golang.org/x/text/cases`.
- **Цепочка `ReplaceAll`** для нескольких замен — это N проходов; бери `NewReplacer` (один проход, создаётся один раз и переиспользуется).
- Все функции возвращают **новую строку** (исходная immutable); для тяжёлой сборки — `Builder`.

---

## Interview-ready answer

**1. Почему `strings.Index` возвращает странное число на кириллице?**
Это байтовое смещение, а не номер символа. Строка — UTF-8, кириллица по 2 байта, поэтому `Index("аб", "б")` = 2. Для индекса по символам — `[]rune`.

**2. Чем `Split` отличается от `Fields`?**
`Fields` режет по группам пробелов (`unicode.IsSpace`) и выкидывает пустые поля — для «текста по словам». `Split(s, sep)` режет строго по разделителю и **сохраняет** пустые части (двойной разделитель → пустая строка в результате).

**3. В чём ловушка `Trim`?**
Второй аргумент — это **набор рун** (cutset), которые срезаются с концов, а не строка-префикс. `Trim("/api/x", "/api")` срежет любые `/api`-символы с краёв. Чтобы убрать ровно префикс — `TrimPrefix`.

**4. Как правильно сравнить строки без учёта регистра?**
`strings.EqualFold(a, b)` — корректно по Unicode и без аллокаций. `ToLower(a)==ToLower(b)` хуже: две лишние строки и ошибки на спецсимволах (`ß`, сигма).

**5. Как сделать несколько замен эффективно?**
`strings.NewReplacer(old1,new1, old2,new2, ...)` — один проход по строке, потокобезопасен, создаётся один раз. Цепочка `ReplaceAll` — это несколько проходов и промежуточных строк.

**6. Что нового в `strings` в свежих версиях?**
`Cut` (1.18), `CutPrefix`/`CutSuffix` (1.20), и итераторы `Lines`/`SplitSeq`/`FieldsSeq` (1.24) — ленивые, без аллокации `[]string`.
