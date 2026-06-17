# io и bufio: потоки, композиция интерфейсов, буферизация

`io` — это не «пакет для файлов», а **набор крошечных интерфейсов** (`Reader`, `Writer`, `Closer`), на которых строится вся потоковая обработка в Go: сеть, файлы, сжатие, HTTP-тела, шифрование — всё это `io.Reader`/`io.Writer`. Понимание контракта `Read`, семантики `io.EOF` и того, как эти интерфейсы **композируются**, — senior-тема. `bufio` добавляет буферизацию поверх них.

## Содержание

- [io.Reader и io.Writer — базовые контракты](#ioreader-и-iowriter--базовые-контракты)
- [Контракт Read: n, err и EOF](#контракт-read-n-err-и-eof)
- [io.EOF и io.ErrUnexpectedEOF](#ioeof-и-ioerrunexpectedeof)
- [Композиция маленьких интерфейсов](#композиция-маленьких-интерфейсов)
- [io.Copy и быстрые пути (ReaderFrom/WriterTo)](#iocopy-и-быстрые-пути-readerfromwriterto)
- [io.ReadAll и опасность неограниченной памяти](#ioreadall-и-опасность-неограниченной-памяти)
- [Полезные обёртки: LimitReader, MultiReader, TeeReader, Pipe](#полезные-обёртки-limitreader-multireader-teereader-pipe)
- [bufio: зачем буферизация](#bufio-зачем-буферизация)
- [bufio.Scanner и ловушка лимита токена](#bufioscanner-и-ловушка-лимита-токена)
- [Подводные камни](#подводные-камни)
- [Разбор примеров-загадок](#разбор-примеров-загадок)
- [Interview-ready answer](#interview-ready-answer)

---

## io.Reader и io.Writer — базовые контракты

Два интерфейса по **одному методу** — фундамент всего:

```go
type Reader interface {
    Read(p []byte) (n int, err error)   // прочитать в p, вернуть сколько и ошибку
}

type Writer interface {
    Write(p []byte) (n int, err error)  // записать p, вернуть сколько и ошибку
}
```

Их реализуют сотни типов: `*os.File`, `*bytes.Buffer`, `strings.Reader`, `net.Conn`, `*http.Request.Body`, `gzip.Reader` и т.д. Любая функция, принимающая `io.Reader`, работает с **чем угодно** — это и есть «accept interfaces»:

```go
func countBytes(r io.Reader) (int, error) {
    return io.Copy(io.Discard, r) // считаем, не храня
}
// работает с файлом, сетью, строкой, HTTP-телом — без изменений
```

---

## Контракт Read: n, err и EOF

`Read(p)` **не обязан** заполнить весь `p`. Его контракт тоньше, чем кажется, и это источник багов:

- возвращает `0 ≤ n ≤ len(p)` — сколько реально прочитано в `p[:n]`;
- может вернуть `n > 0` **и** `err != nil` одновременно (например, `n>0` вместе с `io.EOF`) — обработать **данные `p[:n]` сначала**, потом ошибку;
- может вернуть `n == 0, err == nil` — это не конец, просто сейчас нечего отдать; вызывающий должен попробовать снова;
- конец потока сигнализируется `io.EOF`.

```go
r := strings.NewReader("hi")
buf := make([]byte, 5)

n, err := r.Read(buf)   // n=2, err=<nil>   ← прочитал "hi", но EOF ещё НЕ вернул
n, err = r.Read(buf)    // n=0, err=io.EOF  ← конец на следующем вызове
```

Здесь `strings.Reader` вернул данные и `nil`, а `EOF` — отдельным следующим вызовом. Другие реализации могут вернуть `n>0` **вместе** с `io.EOF`. Поэтому **правильный цикл чтения** обрабатывает `n` до проверки ошибки:

```go
for {
    n, err := r.Read(buf)
    if n > 0 {
        process(buf[:n])   // сначала данные
    }
    if err != nil {
        if err == io.EOF { break }  // нормальный конец
        return err                   // настоящая ошибка
    }
}
```

> На практике этот цикл редко пишут руками — берут `io.Copy`, `io.ReadAll`, `bufio.Scanner`. Но знать контракт нужно: ручной `Read` с проверкой `err` до обработки `n` теряет последние байты.

---

## io.EOF и io.ErrUnexpectedEOF

`io.EOF` — это **sentinel-ошибка** (`var EOF = errors.New("EOF")`), а **не** настоящая ошибка: это нормальный сигнал «данные кончились». Проверяют `err == io.EOF` (или `errors.Is`, если ошибка может быть обёрнута).

```go
if err == io.EOF {
    // штатное завершение, не логировать как ошибку
}
```

- **`io.EOF`** — поток закончился **на границе** (между значениями, ожидаемо);
- **`io.ErrUnexpectedEOF`** — поток оборвался **посреди** ожидаемых данных (например, `io.ReadFull` прочитал меньше, чем просили). Это уже признак повреждённых/неполных данных.

```go
buf := make([]byte, 10)
n, err := io.ReadFull(strings.NewReader("hi"), buf)
// n=2, err=io.ErrUnexpectedEOF — просили 10, получили 2 → данные неполные
```

---

## Композиция маленьких интерфейсов

Сила `io` — в **композиции одно-методных интерфейсов**. Из `Reader`, `Writer`, `Closer` собираются комбинации:

```go
type Closer interface{ Close() error }

type ReadWriter   interface { Reader; Writer }
type ReadCloser   interface { Reader; Closer }
type WriteCloser  interface { Writer; Closer }
type ReadWriteCloser interface { Reader; Writer; Closer }
```

Это прямое применение «interface segregation»: функция требует ровно то, что использует. Принимаешь `io.Reader`, если только читаешь; `io.ReadCloser`, если ещё и закрываешь (`http.Response.Body`).

**Декораторы** — тоже композиция: обёртка реализует `io.Reader` и оборачивает другой `io.Reader`, добавляя поведение. Так устроены `gzip`, `bufio`, `cipher`, подсчёт байт:

```go
gz, _ := gzip.NewReader(file)      // io.Reader поверх io.Reader (файла)
buffered := bufio.NewReader(gz)    // io.Reader поверх gzip
// читаем из buffered — данные идут: файл → gunzip → буфер → нам
```

---

## io.Copy и быстрые пути (ReaderFrom/WriterTo)

`io.Copy(dst, src)` перекачивает из `Reader` в `Writer` через внутренний буфер (32 KB), пока не `io.EOF` — без загрузки всего в память:

```go
written, err := io.Copy(dst, src) // потоково, фиксированный буфер 32KB
```

Неочевидное: `io.Copy` **проверяет, не реализует ли** `src` интерфейс `io.WriterTo` или `dst` — `io.ReaderFrom`. Если да — вызывает их напрямую, минуя промежуточный буфер. Поэтому `io.Copy` из файла в сокет или `bytes.Buffer` в `Writer` часто использует zero-copy/оптимальный путь:

```go
type WriterTo  interface { WriteTo(w Writer) (n int64, err error) }
type ReaderFrom interface { ReadFrom(r Reader) (n int64, err error) }
```

Варианты: `io.CopyN(dst, src, n)` — скопировать ровно `n` байт; `io.CopyBuffer` — со своим буфером (переиспользование через `sync.Pool`).

---

## io.ReadAll и опасность неограниченной памяти

`io.ReadAll(r)` читает **всё** до EOF в память:

```go
data, err := io.ReadAll(r) // []byte со всем содержимым
```

⚠️ Это потенциальная **DoS-дыра** на недоверенном входе (HTTP-тело, загрузка): клиент пришлёт гигабайты — сервис съест всю память. Ограничивай через `io.LimitReader`:

```go
const maxBody = 1 << 20 // 1 MB
data, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
// в HTTP лучше http.MaxBytesReader(w, r.Body, maxBody) — он ещё и закрывает соединение
```

---

## Полезные обёртки: LimitReader, MultiReader, TeeReader, Pipe

```go
// LimitReader — отдаёт не больше N байт, потом EOF
io.ReadAll(io.LimitReader(strings.NewReader("hello world"), 5)) // "hello"

// MultiReader — конкатенация нескольких Reader в один поток
mr := io.MultiReader(strings.NewReader("foo"), strings.NewReader("bar"))
io.ReadAll(mr) // "foobar"

// TeeReader — читает из r и одновременно пишет прочитанное в w (например, для хеша/лога)
var sum bytes.Buffer
tr := io.TeeReader(src, &sum)
io.Copy(dst, tr)  // dst получает данные, sum — их копию по пути

// io.Discard — /dev/null: Writer, который всё игнорирует (для подсчёта/сброса)
io.Copy(io.Discard, r)

// io.NopCloser — превратить Reader в ReadCloser с no-op Close (когда API требует Closer)
rc := io.NopCloser(strings.NewReader("x"))
```

**`io.Pipe`** — синхронная труба в памяти: `Write` в одном конце блокируется, пока `Read` не заберёт в другом. Соединяет производителя-`Writer` с потребителем-`Reader` (например, стримить JSON прямо в HTTP-запрос без буфера):

```go
pr, pw := io.Pipe()
go func() {
    defer pw.Close()
    json.NewEncoder(pw).Encode(bigObject) // пишем в pw
}()
http.Post(url, "application/json", pr)     // читаем из pr — стримом, без полного буфера
```

---

## bufio: зачем буферизация

«Голый» `io.Reader`/`io.Writer` может делать **syscall на каждый вызов** `Read`/`Write`. Читать файл по байту = миллион syscall'ов. `bufio` вставляет буфер в памяти: накапливает и обращается к источнику большими блоками.

```go
// Чтение с буфером + удобные методы
br := bufio.NewReader(file)
line, err := br.ReadString('\n')  // читать до разделителя
b, err := br.ReadByte()
br.Peek(4)                        // подсмотреть, не сдвигая позицию

// Запись с буфером — копит и сбрасывает блоками
bw := bufio.NewWriter(file)
bw.WriteString("hello")
// ...
bw.Flush()   // ОБЯЗАТЕЛЬНО: иначе буфер не доедет до файла
```

> ⚠️ **`bufio.Writer` нужно `Flush()`** — иначе данные останутся в буфере и пропадут:
> ```go
> var buf bytes.Buffer
> w := bufio.NewWriter(&buf)
> w.WriteString("hello")
> // buf пустой! ("")
> w.Flush()
> // теперь buf == "hello"
> ```
> Идиома — `defer w.Flush()` сразу после создания (и проверять ошибку Flush, т.к. реальная ошибка записи всплывёт именно там).

---

## bufio.Scanner и ловушка лимита токена

`bufio.Scanner` — удобное построчное (или по словам) чтение:

```go
sc := bufio.NewScanner(r)
for sc.Scan() {
    line := sc.Text()   // или sc.Bytes() — без копии, но валидно до следующего Scan()
    process(line)
}
if err := sc.Err(); err != nil {  // EOF НЕ попадает в Err() — это штатный конец
    return err
}
```

`sc.Split(...)` задаёт стратегию: `bufio.ScanLines` (по умолчанию), `ScanWords`, `ScanRunes`, `ScanBytes`.

**Главная ловушка — лимит размера токена (64 KB по умолчанию).** Длинная строка/JSON-строка в одну линию → Scanner молча останавливается с ошибкой:

```go
long := strings.Repeat("x", 70*1024) // одна строка > 64 KB
sc := bufio.NewScanner(strings.NewReader(long))
sc.Scan()          // false
sc.Err()           // "bufio.Scanner: token too long"
```

Лечится увеличением буфера:

```go
sc := bufio.NewScanner(r)
sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // стартовый и максимальный размер
```

Если строки могут быть произвольно длинными — Scanner не подходит вовсе, используй `bufio.Reader.ReadString('\n')` / `ReadLine` (нет лимита токена). Эта ошибка регулярно всплывает в продакшне на «жирных» лог-строках.

---

## Подводные камни

1. **Проверка `err` до обработки `n` в `Read`** — теряются последние байты (Read может вернуть `n>0` вместе с `io.EOF`). Сначала `p[:n]`, потом ошибка.
2. **Забыть `bufio.Writer.Flush()`** — данные остаются в буфере. `defer w.Flush()`.
3. **`io.ReadAll` без лимита** на недоверенном входе — OOM/DoS. `io.LimitReader` / `http.MaxBytesReader`.
4. **`bufio.Scanner` token too long** — длинные строки > 64 KB. `Buffer(...)` или `bufio.Reader`.
5. **`sc.Bytes()` живёт до следующего `Scan()`** — переиспользует буфер; если надо сохранить — копировать.
6. **Не закрыть `io.ReadCloser`** (`http.Response.Body`) — утечка соединений; всегда `defer body.Close()` (и дочитать тело для reuse keep-alive).
7. **`io.EOF` как ошибка** — это не ошибка, не логировать и не возвращать как сбой; у Scanner EOF вообще не попадает в `Err()`.

---

## Разбор примеров-загадок

### Загадка 1: потерянные байты при чтении

```go
r := strings.NewReader("hello")
buf := make([]byte, 8)
var out []byte
for {
    n, err := r.Read(buf)
    if err != nil {     // ❌ проверяем ошибку ДО обработки n
        break
    }
    out = append(out, buf[:n]...)
}
fmt.Printf("%q\n", out)  // ?
```

<details>
<summary>Ответ</summary>

Зависит от реализации Reader. Для `strings.Reader` — `"hello"` (он отдаёт `n>0, nil`, а EOF следующим вызовом). Но для Reader, который возвращает `n>0` **вместе** с `io.EOF` (так делают многие — файлы, сеть, `bytes.Reader` в некоторых случаях), последний кусок **потеряется**: цикл выйдет по `err`, не добавив `buf[:n]`. Поэтому контракт требует обрабатывать `n` **до** проверки `err`:

```go
n, err := r.Read(buf)
if n > 0 { out = append(out, buf[:n]...) }
if err != nil { break }
```
</details>

---

### Загадка 2: пустой результат после bufio.Writer

```go
var buf bytes.Buffer
w := bufio.NewWriter(&buf)
fmt.Fprintf(w, "result=%d", 42)
fmt.Printf("%q\n", buf.String())  // ?
```

<details>
<summary>Ответ</summary>

```
""
```

Пусто. `bufio.NewWriter` копит данные в своём буфере и сбрасывает в `&buf` только при заполнении или **явном `Flush()`**. Без `w.Flush()` (или `defer w.Flush()`) `buf` остаётся пустым — данные «висят» в буфере и пропадут. Классический баг: записали в bufio.Writer/gzip.Writer, забыли Flush/Close — на выходе пусто или обрезано.
</details>

---

### Загадка 3: Scanner молча «обрывается»

```go
data := strings.Repeat("a", 100_000) + "\nsecond line"
sc := bufio.NewScanner(strings.NewReader(data))
lines := 0
for sc.Scan() { lines++ }
fmt.Println(lines, sc.Err())  // ?
```

<details>
<summary>Ответ</summary>

```
0 bufio.Scanner: token too long
```

Первая строка — 100 000 байт, больше дефолтного лимита токена 64 KB → `Scan()` сразу вернул `false`, **ни одной строки не прочитано**, причина — только в `sc.Err()` (его легко не проверить → «тихая» потеря данных). Фикс: `sc.Buffer(make([]byte,0,64*1024), 1<<20)` или `bufio.Reader.ReadString('\n')` без лимита. Поэтому `sc.Err()` после цикла проверять обязательно.
</details>

---

## Interview-ready answer

**1. Что такое `io.Reader`/`io.Writer` и почему они везде?**

- Одно-методные интерфейсы (`Read(p)`/`Write(p)`), универсальный контракт потока байт. Их реализуют файлы, сеть, строки, HTTP-тела, сжатие — поэтому функция, принимающая `io.Reader`, работает с любым источником. Композируются (`ReadCloser`, `ReadWriter`) и декорируются (gzip/bufio/cipher оборачивают другой Reader).

**2. Контракт `Read` — в чём подвох?**

- `Read` не обязан заполнить весь буфер; может вернуть `n>0` вместе с `io.EOF`; может вернуть `0, nil`. Обрабатывать `p[:n]` **до** проверки ошибки, иначе теряются последние байты. `io.EOF` — штатный конец (sentinel), не ошибка; `io.ErrUnexpectedEOF` — обрыв посреди данных.

**3. Чем опасен `io.ReadAll`?**

- Читает всё в память — на недоверенном входе это OOM/DoS. Ограничивать `io.LimitReader(r, max)` или `http.MaxBytesReader`. Для копирования без полного буфера — `io.Copy` (потоково, 32 KB, плюс быстрые пути через `WriterTo`/`ReaderFrom`).

**4. Зачем `bufio` и где грабли?**

- Буфер в памяти убирает syscall на каждый Read/Write. `bufio.Writer` **требует `Flush()`** — иначе данные не доедут. `bufio.Scanner` имеет лимит токена **64 KB** — длинные строки дают `token too long` (молча, проверять `sc.Err()`); лечится `Buffer(...)` или переходом на `bufio.Reader.ReadString`.

**5. Полезные обёртки `io`?**

- `LimitReader` (ограничить объём), `MultiReader` (склейка), `TeeReader` (читать + копировать в сторону, напр. для хеша), `io.Pipe` (соединить Writer-producer с Reader-consumer стримом), `io.Discard` (/dev/null), `io.NopCloser` (добавить no-op Close).
