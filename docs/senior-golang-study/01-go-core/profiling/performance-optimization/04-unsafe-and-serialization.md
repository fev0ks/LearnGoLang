# Unsafe, zero-copy и сериализация

## Содержание

- [Когда zero-copy имеет смысл](#когда-zero-copy-имеет-смысл)
- [Zero-copy byte и string](#zero-copy-byte-и-string)
- [Reinterpretation слайса](#reinterpretation-слайса)
- [Проверки unsafe-кода](#проверки-unsafe-кода)
- [Protobuf и FlatBuffers](#protobuf-и-flatbuffers)
- [Как сравнивать codecs](#как-сравнивать-codecs)
- [План миграции формата](#план-миграции-формата)
- [Checklist](#checklist)
- [Источники](#источники)

Zero-copy убирает перемещение bytes, но не убирает стоимость владения памятью.
Вместо CPU и allocations появляются lifetime, aliasing и immutability
инварианты. Такой обмен оправдан только для измеренного hot path.

---

## Когда zero-copy имеет смысл

Сначала оценивается верхняя граница выигрыша. Если копирование занимает 2% CPU,
полное его устранение не может само по себе уменьшить CPU сервиса на 20%.

Перед `unsafe` проверяются более безопасные варианты:

1. не создавать промежуточное представление;
2. передать destination buffer от caller;
3. использовать `append`/`WriteTo` вместо `string` между двумя byte APIs;
4. предварительно выделить capacity;
5. переиспользовать temporary buffer в пределах request;
6. изменить API так, чтобы ownership передавался явно;
7. применить `sync.Pool`, если проблема именно в частых временных allocations.

Zero-copy становится кандидатом, когда копия остаётся в профиле после этих
изменений, а lifetime исходного buffer контролируется приложением.

---

## Zero-copy byte и string

Обычная конверсия `string(data)` создаёт независимую immutable строку. Zero-copy
view связывает содержимое двух значений:

```go
func bytesToStringView(data []byte) string {
    return unsafe.String(unsafe.SliceData(data), len(data))
}

func stringToBytesReadOnlyView(value string) []byte {
    return unsafe.Slice(unsafe.StringData(value), len(value))
}
```

Тип `[]byte` не умеет выразить read-only contract, поэтому второй helper
особенно опасен: запись в полученный slice нарушает immutability строки и может
завершить процесс при работе со string literal.

### Инварианты

- backing memory нельзя менять всё время жизни string view;
- pooled buffer нельзя вернуть в pool, пока существует view;
- view нельзя сохранять дольше network/request buffer, из которого он построен;
- API не отдаёт mutable alias к данным, считающимся immutable;
- helper изолируется в маленьком package и получает тесты;
- обычная копирующая конверсия остаётся default вне измеренного hot path.

Особенно опасный сценарий:

```go
buf := pool.Get().([]byte)
name := bytesToStringView(buf)
pool.Put(buf)

// Другая goroutine получает buf и меняет его.
// Строка name незаметно меняется вместе с тем же backing array.
```

Race detector может найти одновременную запись и чтение, но не обязан найти
последовательное нарушение ownership. Контракт важнее инструмента проверки.

Глубокий разбор: [Strings](../../07-strings.md#unsafe-конверсии-без-копии) и
[Unsafe And Low-Level](../../08-unsafe-and-low-level.md).

---

## Reinterpretation слайса

Типы `[]int64` и `[]HotelID`, где `type HotelID int64`, имеют совместимое
представление элементов в текущей реализации, но являются разными типами.
Unsafe reinterpretation обходит type system, aliasing и будущие изменения типа:

```go
type HotelID int64

func asHotelIDs(values []int64) []HotelID {
    return unsafe.Slice(
        (*HotelID)(unsafe.Pointer(unsafe.SliceData(values))),
        len(values),
    )
}
```

Helper корректен только пока оба element types имеют одинаковые size, alignment
и semantics. Если `HotelID` позже станет struct, старый helper продолжит
выглядеть низкоуровнево убедительно, но станет некорректным.

Безопасная альтернатива явно копирует элементы:

```go
func copyHotelIDs(values []int64) []HotelID {
    result := make([]HotelID, len(values))
    for i, value := range values {
        result[i] = HotelID(value)
    }
    return result
}
```

Если отдельный domain type не нужен, alias `type HotelID = int64` убирает
конверсию, но одновременно убирает защиту от смешивания разных identifiers.
Решение о type safety нельзя принимать только по микробенчмарку.

Для types с pointers reinterpretation ещё опаснее: GC должен видеть корректную
pointer bitmap. Совпадения общего размера недостаточно.

---

## Проверки unsafe-кода

```bash
go vet ./...
go test -race ./...
go test -gcflags=all=-d=checkptr=2 ./...
```

Эти инструменты находят только часть ошибок:

- `go vet` проверяет известные некорректные patterns `unsafe.Pointer`;
- race detector ищет одновременные конфликтующие доступы;
- `checkptr` проверяет часть pointer arithmetic и conversions;
- ни один из них не доказывает application-level ownership;
- ни один из них не гарантирует совместимость с будущей версией compiler.

Полезные дополнительные меры:

- compile-time или unit checks для `unsafe.Sizeof` и `unsafe.Alignof`;
- fuzz tests на пустых, коротких и больших buffers;
- benchmark безопасного и unsafe вариантов в одном package;
- комментарий с lifetime и mutation contract над helper;
- минимальная область видимости unsafe API;
- canary rollout с проверкой data corruption signals.

---

## Protobuf и FlatBuffers

В докладе смена Protobuf на FlatBuffers дала большой выигрыш в конкретном
сервисе, где сериализация занимала основную часть CPU. Из этого не следует, что
FlatBuffers быстрее для каждого API.

| Аспект | Protobuf | FlatBuffers |
| --- | --- | --- |
| Модель чтения | bytes обычно разбираются в generated object model | generated accessors читают поля из исходного buffer |
| Аллокации при чтении | зависят от runtime, message shape и API | можно читать без распаковки всего объекта |
| Построение сообщения | привычная mutable object model | builder и layout-oriented API сложнее |
| Lifetime | parsed object живёт по правилам runtime | accessors зависят от lifetime исходного buffer |
| Schema evolution | зрелая экосистема и известные compatibility rules | свои evolution rules и ограничения layout |
| RPC/tooling | широкая интеграция, включая gRPC | интеграцию и operational tooling проверяют отдельно |

FlatBuffers особенно интересен, когда consumer читает несколько полей из
большого сообщения и может сохранить исходный buffer без построения полной
object graph.

Преимущество уменьшается, если приложение сразу копирует все поля в обычные Go
structs, выполняет глубокую validation или преобразует сообщение в другую
модель на каждом запросе.

Protobuf тоже не сводится к одному неизменному benchmark. Результат зависит от
generated runtime, формы message, unknown fields, repeated values, reuse
объектов и паттерна `MarshalAppend`/`Unmarshal` конкретной Go implementation.

---

## Как сравнивать codecs

Сравнивается полный сценарий:

1. чтение network buffer;
2. framing и ограничения размера;
3. validation недоверенных bytes;
4. доступ к реально используемым полям;
5. преобразование в domain model, если оно требуется;
6. формирование и сериализация ответа;
7. размер payload и стоимость сети;
8. allocations, CPU, throughput и p99;
9. размер generated code и binary;
10. сложность debugging и observability.

Нужны разные формы сообщений:

```text
маленькое сообщение, все поля читаются
большое сообщение, читаются 2-3 поля
много repeated scalar values
много вложенных strings/bytes
unknown/optional fields
пустые и максимально допустимые payloads
```

Benchmark `Marshal` отдельно от `Unmarshal` полезен для локализации стоимости,
но архитектурное решение принимается по end-to-end workload.

Размер payload влияет на network bandwidth и cache behavior. Codec, который
быстрее читает, но создаёт существенно более крупный wire format, может
проиграть в распределённой системе.

---

## План миграции формата

Смена wire format — изменение distributed contract, а не локальный refactoring.

Безопасный план:

1. описать schema compatibility и versioning;
2. добавить golden tests для старого и нового formats;
3. внедрить dual-read или versioned endpoint;
4. включить shadow decode и сравнение результатов без влияния на ответ;
5. измерить decode failures, CPU, allocations и latency;
6. постепенно включить new writer;
7. сохранить rollback, пока старые consumers не мигрировали;
8. удалить старый format только после проверки данных и traffic.

Если message хранится в database, cache или broker, учитываются данные с долгим
lifetime. Rollback приложения не поможет, если новый writer уже создал записи,
которые старый reader не понимает.

Zero-copy format также требует контроля buffer lifetime. Нельзя вернуть network
buffer в pool, если generated accessor или domain wrapper продолжает на него
ссылаться.

---

## Checklist

- Копирование найдено в профиле и имеет достаточную долю для нужного выигрыша.
- Безопасные варианты reuse/preallocation проверены до `unsafe`.
- У zero-copy helper описаны owner, lifetime и mutation contract.
- Buffer не возвращается в pool раньше последнего view.
- Reinterpreted types имеют проверенные size, alignment и pointer semantics.
- Безопасный и unsafe варианты сравниваются одним benchmark.
- `go vet`, race detector и `checkptr` включены в проверки.
- Codec сравнивается end-to-end, а не только на одном `Marshal`.
- Migration учитывает schema evolution, stored data и rollback.
- После rollout отслеживаются decode errors и признаки data corruption.

---

## Источники

- [`unsafe` package documentation](https://pkg.go.dev/unsafe)
- [FlatBuffers: Go guide](https://flatbuffers.dev/languages/go/)
- [Protocol Buffers: Encoding](https://protobuf.dev/programming-guides/encoding/)
- [Protocol Buffers: Techniques](https://protobuf.dev/programming-guides/techniques/)
