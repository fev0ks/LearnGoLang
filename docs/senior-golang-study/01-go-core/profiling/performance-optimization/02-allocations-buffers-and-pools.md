# Аллокации, буферы и pools

## Содержание

- [Строки и предварительное выделение памяти](#строки-и-предварительное-выделение-памяти)
- [Caller-provided destination](#caller-provided-destination)
- [Меньше объектов, а не только меньше байт](#меньше-объектов-а-не-только-меньше-байт)
- [Переиспользование через sync.Pool](#переиспользование-через-syncpool)
- [Pools по размеру](#pools-по-размеру)
- [Optional bool без указателя](#optional-bool-без-указателя)
- [Checklist](#checklist)
- [Источники](#источники)

Цель этой группы приёмов — уменьшить allocation rate и работу GC на измеренном
горячем пути. Нулевое `allocs/op` не является самостоятельным требованием:
иногда одна понятная allocation дешевле сложного ownership и удержания большого
backing array.

---

## Строки и предварительное выделение памяти

`fmt.Sprintf` решает общую задачу форматирования: разбирает format string,
работает с `any` и поддерживает множество типов. Эта гибкость имеет цену, но
сама по себе не делает `fmt` проблемой вне горячего пути.

Если формат фиксирован, данные типизированы, а функция видна в CPU или alloc
profile, строку можно строить через `append` и `strconv.Append*`:

```go
func appendAvailabilityKey(
    dst []byte,
    hotelID int64,
    date time.Time,
) []byte {
    dst = strconv.AppendInt(dst, hotelID, 10)
    dst = append(dst, ':')
    return date.AppendFormat(dst, "2006-01-02")
}

buf := make([]byte, 0, 32)
buf = appendAvailabilityKey(buf, 42, date)
```

Capacity `32` — оценка для конкретного формата, а не магическая константа. Если
она достаточна, `append` не меняет backing array. Если недостаточна, корректность
сохраняется, но buffer вырастет.

| Инструмент | Когда удобен | Основная цена |
| --- | --- | --- |
| `fmt.Sprintf` | cold path, сложное или динамическое форматирование | общий parser и работа с `any` |
| `strings.Builder` | конечный результат — `string` | нужно оценить рост и не копировать непустой builder |
| `bytes.Buffer` | нужен `io.Writer` и работа с bytes | API общего назначения и удержание capacity |
| `append` + `strconv.Append*` | небольшой стабильный формат в hot path | больше ручного кода |

Если результат зависит от небольшого конечного набора входов, рассматривается
precomputation. Таблица заранее подготовленных значений убирает форматирование
из request path, но тратит память и требует явного обновления при смене данных.

Предварительное выделение памяти помогает только при разумной оценке. Сильное
завышение capacity увеличивает RSS, а сохранение маленького subslice большого
buffer может удержать весь backing array.

Глубокий разбор: [Strings](../../07-strings.md).

---

## Caller-provided destination

Вместо функции, которая каждый раз создаёт результат, API может принимать
destination buffer и дописывать данные в него:

```go
func appendRecord(dst []byte, r Record) []byte {
    dst = strconv.AppendInt(dst, r.ID, 10)
    dst = append(dst, '|')
    dst = append(dst, r.Name...)
    return dst
}

func encodeBatch(records []Record) []byte {
    dst := make([]byte, 0, estimateBatchSize(records))
    for _, r := range records {
        dst = appendRecord(dst, r)
        dst = append(dst, '\n')
    }
    return dst
}
```

Caller управляет lifetime и capacity, а вложенная функция не обязана создавать
отдельный backing array. Паттерн особенно полезен для codecs, логирования и
построения сетевого ответа.

API вида `AppendTo(dst []byte)` хорошо композируется: несколько функций пишут в
один buffer. Цена — caller должен понимать, что возвращаемый slice может
ссылаться либо на прежний, либо на новый backing array, поэтому результат
обязательно присваивается обратно.

Передача `*Result` вместо возврата `Result` сама по себе ничего не гарантирует.
Compiler может разместить возвращаемое значение на стеке caller после inlining,
а указатель может, наоборот, escape в heap. Решение проверяется через `-m=2` и
`-benchmem`.

---

## Меньше объектов, а не только меньше байт

GC работает не только с суммой выделенных байт. Миллионы небольших объектов и
указателей создают расходы на allocation, scanning и cache misses. Полезными
могут быть:

- значения вместо указателей для небольших immutable объектов;
- один плоский `[]T` вместо `[]*T`, если не нужна стабильность адресов;
- пакетная обработка нескольких элементов одним буфером;
- хранение данных в компактном представлении до момента, когда object model
  действительно нужна;
- reuse одного destination buffer в пределах запроса.

У `[]T` есть и цена: append может скопировать крупные values при росте capacity,
а передача большого struct по значению иногда дороже указателя. Выбор зависит от
размера типа, мутабельности, lifetime и access pattern.

Подробнее: [Escape Analysis](../../memory-internals/03-escape-analysis.md) и
[Allocator](../../memory-internals/02-allocator.md).

---

## Переиспользование через sync.Pool

`sync.Pool` хранит временные объекты для возможного повторного использования и
снижает allocation pressure при высокой частоте одинаковых операций.

```go
var responseBuffers = sync.Pool{
    New: func() any {
        return new(bytes.Buffer)
    },
}

func writeResponse(w io.Writer, value Value) error {
    buf := responseBuffers.Get().(*bytes.Buffer)
    buf.Reset()

    defer func() {
        const maxPooledCapacity = 64 << 10
        if buf.Cap() <= maxPooledCapacity {
            responseBuffers.Put(buf)
        }
    }()

    encodeValue(buf, value)
    _, err := w.Write(buf.Bytes())
    return err
}
```

Ограничение capacity не даёт одному редкому большому ответу навсегда увеличить
обычный pooled buffer. Порог `64 KiB` — допущение: его выбирают по распределению
размеров и memory budget.

Инварианты `sync.Pool`:

- runtime может удалить любой объект из pool без уведомления;
- `Get` не обязан вернуть объект, ранее переданный в `Put`;
- перед повторным использованием состояние полностью сбрасывается;
- объект нельзя одновременно использовать и возвращать в pool;
- нельзя вернуть наружу view на pooled memory, а затем сразу сделать `Put`;
- `sync.Pool` не подходит для cache, где запись обязана пережить GC;
- сам `Pool` нельзя копировать после начала использования.

Возврат pointer types обычно удобнее: значение pointer помещается в `any` без
дополнительной allocation для самого объекта interface. Typed wrapper поверх
`sync.Pool` улучшает API, но не меняет публичный контракт и внутреннюю природу
`Get() any`.

Pool полезен, когда его overhead амортизируется большим числом независимых
операций. Free list внутри короткоживущего объекта часто проще и дешевле хранить
в самом объекте.

---

## Pools по размеру

Если размеры buffers сильно различаются, один pool может отдавать слишком
большие backing arrays маленьким запросам. Несколько pools по size classes
решают эту проблему:

```text
request 900 B  -> class 1 KiB
request 5 KiB  -> class 8 KiB
request 40 KiB -> class 64 KiB
request 2 MiB  -> не pool-ить
```

Обычно классы растут степенями двойки: выбор класса быстрый, а внутренняя
фрагментация ограничена разницей до следующего класса. Но это не обязательный
закон: распределение реальных payloads может оправдать другие границы.

Цена подхода:

- внутренняя фрагментация;
- логика выбора и возврата в правильный pool;
- необходимость ограничить максимальный сохраняемый размер;
- риск создать собственный allocator, который дублирует работу runtime;
- больше вариантов для ownership bugs.

Польза подтверждается alloc profile и RSS под длительной нагрузкой. Ускоренный
микробенчмарк на коротком запуске может не показать удержание памяти после
редкого большого запроса.

---

## Optional bool без указателя

`*bool` часто используют для трёх состояний: поле отсутствует, `false` и
`true`. Значение с явным признаком присутствия выражает те же состояния без
указателя:

```go
type OptionalBool struct {
    Value bool
    Set   bool
}
```

Преимущества появляются, когда pointer действительно создавал отдельный heap
object или увеличивал pointer scanning в большой коллекции. Само выражение
`&value` не гарантирует heap allocation: escape analysis может оставить объект
на стеке.

Value representation также меняет API. Для JSON, database nullability или
generated protocol types может понадобиться adapter. Производительность не
должна незаметно менять wire semantics.

Альтернативы зависят от задачи:

- отдельные `Value` и `Set` — простой domain type;
- enum — если состояний больше трёх или у них есть имена;
- bitmap присутствия — для очень плотных массовых структур;
- `*bool` — когда этого требует generated API или pointer lifetime уже не
  создаёт измеримой проблемы.

---

## Checklist

- Allocation hotspot найден через `alloc_space`, а не по числу `new` в source.
- Capacity основана на распределении размеров, а не максимуме из одного случая.
- Возвращаемый slice после `append` всегда присваивается caller.
- Pooled object полностью очищается перед reuse.
- Большие buffers не возвращаются в общий pool без cap policy.
- Наружу не уходит alias на memory уже возвращённого объекта.
- `sync.Pool` не используется как надёжный cache.
- Optional representation сохраняет прежнюю wire/database semantics.
- После `B/op` и `allocs/op` проверены RSS, GC CPU и p99 полного сервиса.

---

## Источники

- [`sync.Pool` documentation](https://pkg.go.dev/sync#Pool)
- [`bytes.Buffer` documentation](https://pkg.go.dev/bytes#Buffer)
- [`strconv` package](https://pkg.go.dev/strconv)
- [A Guide to the Go Garbage Collector](https://go.dev/doc/gc-guide)
