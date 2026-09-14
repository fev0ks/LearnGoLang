# Структуры данных и memory layout

## Содержание

- [Конкурентные map](#конкурентные-map)
- [Как строить benchmark concurrent map](#как-строить-benchmark-concurrent-map)
- [Copy вместо ручного сдвига](#copy-вместо-ручного-сдвига)
- [Когда нужен ring buffer](#когда-нужен-ring-buffer)
- [Layout структур, padding и cache locality](#layout-структур-padding-и-cache-locality)
- [False sharing](#false-sharing)
- [Checklist](#checklist)
- [Источники](#источники)

Выбор структуры данных обычно влияет сильнее локального micro-optimization.
Один правильный access pattern может убрать общий lock или линейный сдвиг, но
платой становятся новые инварианты, memory overhead и более сложный API.

---

## Конкурентные map

Ни одна concurrent map не является лучшей для всех access patterns.

| Вариант | Подходящий workload | Цена |
| --- | --- | --- |
| `map[K]V` + `Mutex` | mixed/write-heavy, короткие операции, общие инварианты | один lock сериализует конфликтующие операции |
| `map[K]V` + `RWMutex` | много параллельных чтений при редких записях | учёт readers и ожидание writer; не всегда быстрее `Mutex` |
| `sync.Map` | write-once/read-many или независимые наборы ключей | `any`, специальный API, нет произвольных multi-key invariants |
| sharded `map` | contention высок, а ключи хорошо распределяются | hash function, несколько locks, сложнее snapshot и multi-key operations |

`map + Mutex` остаётся хорошим default: типы статически проверяются, а один
critical section может атомарно обновлять несколько связанных значений.

`RWMutex` полезен не из-за названия «read/write», а когда конкурентные readers
действительно успевают работать параллельно достаточно долго, чтобы окупить
учёт readers. При коротких операциях обычный `Mutex` может быть быстрее.

`sync.Map` имеет специализированный public contract. Он особенно подходит для
записей, которые создаются один раз и много читаются, либо для параллельной
работы с независимыми наборами ключей. Он не делает последовательность
`Load` → вычисление → `Store` атомарной; для этого используются подходящие методы
API или внешняя синхронизация.

Sharding уменьшает область contention, когда разные keys попадают в разные
locks:

```text
hash(key) -> shard index -> shard lock -> ordinary map
```

Один hot key всё равно остаётся на одном shard. Плохая hash function или
неравномерное распределение может свести выигрыш к нулю.

Public contract и актуальная реализация подробно разобраны в
[`sync.Map`](../../map-internals/sync-map/README.md).

---

## Как строить benchmark concurrent map

Benchmark должен воспроизводить:

- долю `Get`, `Set` и `Delete`;
- hit/miss ratio;
- число goroutines и `GOMAXPROCS`;
- распределение ключей, включая hot keys;
- стоимость операции внутри critical section;
- рост map и lifetime записей;
- диапазон размеров ключей и значений.

Нужно минимум несколько отдельных scenarios:

```text
95% read / 5% write, keys uniform
95% read / 5% write, один hot key
50% read / 50% write
insert-once, затем только read
delete + recreate одних и тех же keys
```

Benchmark только на чтение ничего не говорит о write-heavy workload. А тест,
где каждая goroutine работает со своим ключом, не моделирует contention на
популярной записи.

Количество allocations также требует объяснения. Оно может приходить не из
map implementation, а из boxing ключа/значения в `any`, генерации случайных
данных, преобразования строк или setup, случайно попавшего под timer.

Практическая методика и пример находятся в
[главе с benchmark `sync.Map`](../../map-internals/sync-map/04-practical-patterns.md).

---

## Copy вместо ручного сдвига

Встроенная функция `copy` поддерживает перекрывающиеся slices и выражает
сдвиг массива напрямую:

```go
func dropFirst(days []Day) {
    if len(days) == 0 {
        return
    }

    copied := copy(days, days[1:])
    clear(days[copied:])
}
```

`clear` важен, если `Day` содержит pointers: без него последний slot продолжит
удерживать старые объекты. Для типа без pointers очистка нужна ради ожидаемого
состояния, а не ради GC scanning.

Сдвиг остаётся `O(n)`, хотя runtime может выполнить его эффективнее ручного
Go-цикла. Поэтому ответ зависит от частоты операции и размера slice:

```text
редкий сдвиг небольшого slice -> copy
частый сдвиг большого slice   -> проверить другую структуру данных
```

Ручной цикл остаётся уместным, когда одновременно со сдвигом выполняется
необходимое преобразование, validation или aggregation. Если его тело только
присваивает соседний элемент, `copy` точнее выражает намерение.

---

## Когда нужен ring buffer

Ring buffer хранит logical head отдельно от физического начала массива:

```text
physical: [D4 D5 D6 D1 D2 D3]
                  ^ head

logical:  [D1 D2 D3 D4 D5 D6]
```

Удаление первого элемента меняет `head` за `O(1)`. Цена:

- logical index переводится в physical через modulo или branch;
- диапазон может состоять из двух физических частей;
- iteration и выдача непрерывного slice усложняются;
- resize требует аккуратно восстановить logical order;
- API должен явно определить поведение при переполнении.

Linked list тоже удаляет head за `O(1)`, однако теряет плотный layout, добавляет
links и обычно ухудшает cache locality. Для календаря с тысячами дней массив или
ring buffer часто лучше соответствует последовательному чтению.

Если внешний consumer требует непрерывный slice на каждый запрос, ring buffer
может вернуть стоимость через дополнительное копирование. Структура выбирается
по полному access pattern, а не по одной операции удаления.

---

## Layout структур, padding и cache locality

Compiler вставляет padding, чтобы каждое поле находилось на допустимом для его
типа alignment. Размер структуры округляется до её alignment, поэтому порядок
полей меняет размер.

Для типичной 64-bit architecture:

```go
type Sparse struct {
    Ready bool
    Count int64
    Dirty bool
}

type Compact struct {
    Count int64
    Ready bool
    Dirty bool
}

fmt.Println(unsafe.Sizeof(Sparse{}))  // обычно 24
fmt.Println(unsafe.Sizeof(Compact{})) // обычно 16
```

Числа проверяются на целевом `GOARCH`: layout является деталью конкретной
реализации и набора типов, а не универсальным размером на всех платформах.

Экономия имеет практический смысл, когда экземпляров много. Для одного объекта
разница в 8 bytes несущественна; для 10 миллионов объектов это:

```text
10 000 000 × 8 B = 80 000 000 B ≈ 80 MB
```

Это raw storage без накладных расходов allocator и containers.

Кроме padding, важна locality:

- поля, которые читаются вместе, полезно держать близко;
- плоский `[]T` обычно лучше использует cache lines, чем `[]*T`;
- редко используемые большие поля можно вынести из плотной горячей структуры;
- hot/cold split уменьшает объём данных, читаемых частым path.

Минимальный размер и минимальный latency не всегда достигаются одним порядком
полей. Например, группировка только по размеру уменьшает padding, но может
разнести поля, совместно читаемые каждой операцией.

Перестановка полей может сломать код, который зависит от binary layout через
`unsafe`, cgo, shared memory или ручную сериализацию. Для обычных encoders,
работающих по именам/тегам, отдельно проверяется wire compatibility.

Подробный расчёт offsets и padding:
[Unsafe And Low-Level](../../08-unsafe-and-low-level.md).

---

## False sharing

Cache locality помогает, когда одна goroutine читает соседние данные. Но поля,
которые часто изменяют разные CPU cores, могут мешать друг другу, если попали в
одну cache line. Cache coherence начинает передавать line между cores, хотя
goroutines логически работают с разными значениями.

```go
type Counters struct {
    Requests atomic.Uint64
    Errors   atomic.Uint64
}
```

Если два counters обновляются разными горячими goroutines, близость может
создать false sharing. Padding или раздельные структуры иногда уменьшают
contention, но:

- типичный размер cache line часто равен 64 bytes, но не является гарантией Go;
- ручной padding увеличивает память и зависит от архитектуры;
- эффект виден только под параллельной нагрузкой;
- scheduler может менять размещение goroutines по cores.

Поэтому field padding для false sharing применяется после benchmark на целевом
hardware или близкой production-платформе. Одного `unsafe.Sizeof` недостаточно:
он показывает layout, но не cache coherence traffic.

---

## Checklist

- Выбранная map проверена на реальном read/write ratio и hot-key distribution.
- `RWMutex` сравнивается с обычным `Mutex`, а не считается быстрее по умолчанию.
- Multi-key invariants остаются атомарными после sharding или перехода на
  `sync.Map`.
- Хэш распределяет production keys, а не только случайные данные benchmark.
- После `copy` tail очищается, если может удерживать pointers.
- Для частого сдвига сравнивается ring buffer и полный API consumer.
- Размер структуры пересчитан через `unsafe.Sizeof` на целевых `GOARCH`.
- Field reorder не ломает binary layout или wire compatibility.
- Compact layout проверяется вместе с access locality.
- Padding против false sharing подтверждён параллельным benchmark.

---

## Источники

- [`sync.Map` documentation](https://pkg.go.dev/sync#Map)
- [Go Memory Model](https://go.dev/ref/mem)
- [`copy` specification](https://go.dev/ref/spec#Appending_and_copying_slices)
- [`unsafe.Sizeof` and `Alignof`](https://pkg.go.dev/unsafe)
