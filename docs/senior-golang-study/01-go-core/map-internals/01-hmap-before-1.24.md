# Историческая map до Go 1.24: hmap и buckets

Эта глава описывает реализацию Go 1.23 и ниже. Она полезна для чтения старых статей и интервью, но современный runtime использует [Swiss Tables](./02-swiss-tables-since-1.24.md).

Главная идея этой реализации: hash выбирает bucket из восьми slots; при коллизиях bucket получает overflow chain, а рост постепенно переносит старые buckets в новый массив.

## Содержание

- [Ментальная модель](#ментальная-модель)
- [Что хранит hmap](#что-хранит-hmap)
- [Как устроен bmap](#как-устроен-bmap)
- [Как выполняется lookup](#как-выполняется-lookup)
- [Зачем нужен tophash](#зачем-нужен-tophash)
- [Insert и overflow](#insert-и-overflow)
- [Почему существует mapextra](#почему-существует-mapextra)
- [Инкрементальный рост](#инкрементальный-рост)
- [Как runtime эвакуирует bucket](#как-runtime-эвакуирует-bucket)
- [Delete и память](#delete-и-память)
- [Как работает итератор](#как-работает-итератор)
- [Что объясняет эта модель](#что-объясняет-эта-модель)
- [Сравнение со Swiss Tables](#сравнение-со-swiss-tables)
- [Interview-ready answer](#interview-ready-answer)

## Ментальная модель

```text
map value
   |
   v
 hmap
   ├── count
   ├── B             number of buckets = 2^B
   ├── hash seed
   ├── buckets ─────> bucket 0, bucket 1, ...
   └── oldbuckets     non-nil while growing

bucket
   ├── tophash[8]
   ├── keys[8]
   ├── values[8]
   └── overflow ─────> next bucket with the same bucket index
```

Один bucket содержит восемь slots. Ключи и значения лежат отдельными блоками: все keys, затем все values. Такая раскладка уменьшает padding для некоторых сочетаний типов.

`hmap` — header, на который ссылается runtime-представление map. Присваивание `m2 := m1` копирует map value, а не содержимое `hmap`, поэтому обе переменные продолжают работать с общими entries.

<details>
<summary>Упрощённые runtime structures</summary>

```go
type hmap struct {
	count      int
	flags      uint8
	B          uint8
	noverflow  uint16
	hash0      uint32
	buckets    unsafe.Pointer
	oldbuckets unsafe.Pointer
	nevacuate  uintptr
	extra      *mapextra
}

type bmap struct {
	tophash [8]uint8
	// После tophash compiler размещает keys, values и overflow pointer.
}
```

Это иллюстрация layout Go 1.23, а не типы для application code. Runtime использует `unsafe` и metadata конкретных `K`/`V`, чтобы вычислять offsets.

</details>

## Что хранит hmap

`hmap` не содержит сами entries. Это небольшой header с состоянием map и pointers на bucket arrays.

| Поле | Зачем нужно |
| --- | --- |
| `count` | Количество живых entries; именно его возвращает `len(m)` |
| `flags` | Состояние runtime: например, выполняется ли итерация или запись |
| `B` | Логарифм числа основных buckets: `bucketCount = 1 << B` |
| `noverflow` | Приблизительное число overflow buckets; помогает решить, нужен ли same-size grow |
| `hash0` | Случайный seed конкретной map для hash-функции |
| `buckets` | Текущий массив основных buckets |
| `oldbuckets` | Предыдущий массив во время grow; в обычном состоянии равен `nil` |
| `nevacuate` | Номер следующего старого bucket, который runtime планирует эвакуировать |
| `extra` | Дополнительные ссылки на overflow buckets и свободные buckets |

Seed хранится отдельно для каждой map. Поэтому одинаковые keys в двух разных maps не обязаны попадать в одинаковые buckets. Это усложняет атаки, которые пытаются заранее подобрать множество keys с одинаковыми hashes, и не позволяет application code полагаться на внутреннее расположение entries.

```text
m1.hash0 = 0xA17F...  ─┐
                       ├─ hash("user:42", seed) может различаться
m2.hash0 = 0x03C2...  ─┘
```

`B` описывает только число **основных** buckets. Overflow buckets в `1 << B` не входят.

## Как устроен bmap

В исходниках `bmap` выглядит обманчиво коротко:

```go
type bmap struct {
	tophash [8]uint8
	// Дальше в памяти находятся 8 keys,
	// затем 8 values и overflow pointer.
}
```

Фактический layout зависит от типов `K` и `V`, поэтому compiler создаёт bucket type для конкретного `map[K]V`, а runtime вычисляет offsets через type metadata.

```text
bmap for map[K]V

┌──────────────────────────────────────┐
│ tophash: t0 t1 t2 t3 t4 t5 t6 t7    │
├──────────────────────────────────────┤
│ keys:    k0 k1 k2 k3 k4 k5 k6 k7    │
├──────────────────────────────────────┤
│ values:  v0 v1 v2 v3 v4 v5 v6 v7    │
├──────────────────────────────────────┤
│ overflow pointer                     │
└──────────────────────────────────────┘
```

Keys и values лежат не как восемь чередующихся пар `key/value`, а двумя блоками. Это экономит padding. Например, при `map[int64]int8` чередующаяся раскладка часто требует выравнивания после каждого `int8`, а блочная — только между целыми массивами.

<details>
<summary>Почему в bucket ровно восемь slots</summary>

В legacy-реализации `bucketCnt` равен `8`. Маленький bucket даёт несколько полезных свойств:

- scan metadata остаётся коротким;
- часто весь bucket или значительная его часть помещается в небольшое число cache lines;
- при умеренных collisions не нужна отдельная allocation на каждый entry;
- стоимость последовательного просмотра ограничена восемью slots до перехода в overflow.

Это не универсальное «идеальное число» и не обещание языка. В Go 1.24 реализация меняется, хотя новая group тоже содержит восемь slots.

</details>

<details>
<summary>Что происходит с большими keys и values</summary>

В Go 1.23 key или value размером до `128` bytes хранится непосредственно в slot. Если тип больше этого порога, slot хранит pointer, а сам объект размещается отдельно:

```text
small key: slot -> [ key bytes ]
large key: slot -> [ *key ] ──> [ key bytes in separate allocation ]
```

Для больших типов runtime metadata включает признаки `IndirectKey` и `IndirectElem`. Это сохраняет разумный размер bucket, но добавляет allocation и pointer dereference. Кроме того, hashing и equality большого key сами по себе проходят по его данным.

Практический вывод: огромная comparable struct технически может быть key, но часто дешевле хранить компактный идентификатор или заранее вычисленный digest. Порог `128` bytes — деталь реализации, а не ограничение языка.

</details>

## Как выполняется lookup

Для `value, ok := m[key]` runtime делает примерно следующее:

1. вычисляет `hash(key, mapSeed)`;
2. берёт младшие `B` bits как индекс bucket;
3. получает верхнюю часть hash как короткий `tophash`;
4. проходит основной bucket и overflow chain;
5. сравнивает полный key только в slots с подходящим `tophash`;
6. возвращает value либо zero value и `ok=false`.

```text
hash = 10110110 ... 01101
       ^^^^^^^^       ^^^
       tophash         B low bits -> bucket index
```

Hash collision не означает равенство. `tophash` лишь отбрасывает большинство неподходящих slots; candidate key всё равно сравнивается полностью.

Упрощённо lookup можно представить так:

```text
h = hash(key, hash0)
bucketIndex = h & ((1 << B) - 1)
top = upperBits(h)

for bucket := buckets[bucketIndex]; bucket != nil; bucket = bucket.overflow {
    for slot := 0; slot < 8; slot++ {
        if bucket.tophash[slot] != top {
            continue
        }
        if bucket.key[slot] == key {
            return bucket.value[slot], true
        }
    }
}
return zeroValue, false
```

Реальный код дополнительно обрабатывает специальные состояния `tophash`, indirect keys/values, grow и оптимизации hash/equality для конкретного типа.

<details>
<summary>Ручной пример с B=2</summary>

При `B=2` существует четыре buckets, а индекс задают два младших bits:

```text
hash(key) ... 1101
mask          0011
result        0001  -> bucket 1
```

Пусть `tophash(key)=A3`, а bucket выглядит так:

```text
slot:       0       1       2       3
tophash:   5C      A3      A3      empty
key:      "x"     "y"   "target"
```

Slot 0 отбрасывается без сравнения key. В slots 1 и 2 fingerprint совпал, поэтому runtime сравнивает полные keys и находит `"target"` только в slot 2.

</details>

### Lookup во время grow

Пока `oldbuckets != nil`, часть старых buckets уже эвакуирована, а часть ещё нет. Runtime не может всегда читать только новый массив.

Алгоритм выглядит так:

1. вычисляет индекс bucket по текущему `B`;
2. находит соответствующий bucket старого массива;
3. проверяет marker эвакуации;
4. если старый bucket ещё не эвакуирован — ищет в нём и его overflow chain;
5. если эвакуирован — ищет в новом массиве.

При grow в два раза новый индекс содержит на один bit больше, поэтому два новых buckets соответствуют одному старому:

```text
new bucket 1 ───────┐
                    ├── old bucket 1
new bucket 1+2^B ───┘
```

Так lookup находит key независимо от того, успел ли runtime перенести конкретный bucket.

## Зачем нужен tophash

Сравнение большого string или struct key дороже сравнения одного metadata byte. Поэтому bucket хранит маленький fingerprint hash для каждого slot.

`tophash` хранит не только fingerprint, но и состояние slot. В Go 1.23 используются такие значения:

| Значение | Имя в runtime | Смысл |
| ---: | --- | --- |
| `0` | `emptyRest` | Этот slot и все следующие slots в bucket/chain сейчас пусты |
| `1` | `emptyOne` | Этот slot пуст, но дальше ещё могут находиться entries |
| `2` | `evacuatedX` | Entry переезжает в нижнюю половину нового массива |
| `3` | `evacuatedY` | Entry переезжает в верхнюю половину нового массива |
| `4` | `evacuatedEmpty` | Bucket эвакуирован, а slot пуст |
| `>= 5` | обычный `tophash` | Slot занят entry с таким fingerprint |

Значения `0..4` зарезервированы, поэтому runtime корректирует слишком маленький fingerprint вверх.

Разница между `emptyRest` и `emptyOne` важна для lookup:

```text
[occupied] [emptyOne] [occupied] [emptyRest] [emptyRest] ...
             ^           ^            ^
             |           |            └─ здесь scan можно закончить
             |           └─ entry после удалённого slot ещё существует
             └─ здесь scan заканчивать нельзя
```

Точные числа — implementation details Go 1.23, но сами роли помогают понять lookup, delete и grow.

## Insert и overflow

При `m[key] = value` runtime сначала ищет существующий key и первый свободный slot:

- key найден — value заменяется;
- свободный slot найден — туда записывается новая пара;
- bucket и chain заполнены — добавляется overflow bucket;
- превышен load/overflow threshold — начинается grow, затем insert повторяется в новой структуре.

<details>
<summary>Полный путь записи в упрощённом псевдокоде</summary>

```text
assign(m, key, value):
    if m == nil:
        panic("assignment to entry in nil map")

    hash = hash(key, m.hash0)
    check/set hashWriting flag

again:
    bucketIndex = hash & ((1 << m.B) - 1)

    if m.oldbuckets != nil:
        growWork(bucketIndex)

    top = tophash(hash)
    found = nil
    firstEmpty = nil

    for bucket in buckets[bucketIndex] + overflow chain:
        for slot in bucket:
            if slot is empty:
                remember firstEmpty
                if slot is emptyRest:
                    stop scan
                continue

            if slot.tophash == top && slot.key == key:
                found = slot
                stop scan

    if found != nil:
        found.value = value
        clear hashWriting flag
        return

    if grow threshold reached and grow is not active:
        start double-size or same-size grow
        goto again

    if firstEmpty == nil:
        firstEmpty = allocate overflow slot

    firstEmpty.tophash = top
    firstEmpty.key = key
    firstEmpty.value = value
    m.count++
    clear hashWriting flag
```

Почему после запуска grow алгоритм идёт в `again`: поле `B` или активный bucket array уже меняется, поэтому старый `bucketIndex` и найденный свободный slot больше нельзя использовать для новой записи.

Флаг `hashWriting` помогает runtime заметить некоторые одновременные writes или read/write и завершить программу с runtime throw. Это защитная диагностика, а не синхронизация и не замена `Mutex`; concurrency-поведение разобрано в [gotchas](./03-puzzles-and-gotchas.md).

</details>

Overflow chains решают collisions просто, но ухудшают locality: lookup переходит по pointers между отдельными buckets. Большое число overflow buckets может запустить same-size grow — перепаковку без увеличения числа основных buckets.

```text
main bucket i        overflow #1         overflow #2
┌─────────────┐      ┌─────────────┐     ┌─────────────┐
│ up to 8     │ ───> │ up to 8     │ ──> │ up to 8     │
│ entries     │      │ entries     │     │ entries     │
└─────────────┘      └─────────────┘     └─────────────┘
```

Overflow не обязательно означает плохую hash-функцию: несколько buckets могут переполниться и при нормальном распределении. Проблемой становятся многочисленные или длинные chains, потому что они добавляют pointer chasing, comparisons и cache misses.

### Когда начинается grow

Legacy runtime проверяет две разные причины:

| Причина | Действие | Зачем |
| --- | --- | --- |
| Средняя заполненность становится слишком высокой | Grow в два раза | Даёт entries больше основных buckets |
| Overflow buckets становится слишком много при невысокой заполненности | Same-size grow | Перепаковывает entries и убирает накопившиеся пустые overflow buckets |

В Go 1.23 порог load factor составляет примерно `6.5` entries на основной bucket, причём `count` также должен превышать восемь. Это параметр реализации, а не основание заранее вычислять точное число allocations в application code.

Same-size grow нужен, например, после такой истории:

```text
1. Map постепенно получает неравномерно распределённые entries.
2. Возникают overflow chains.
3. Часть entries удаляется.
4. len(map) уже невелик, но выделенные overflow buckets и разреженные chains остаются.
5. Следующая серия writes запускает перепаковку в то же число основных buckets.
```

## Почему существует mapextra

Для maps, у которых и key, и value не содержат pointers, compiler может пометить bucket type как pointer-free. Тогда GC не сканирует содержимое bucket целиком — это заметно уменьшает GC work для больших `map[int]int` и похожих maps.

Но у каждого bucket всё равно есть логический overflow pointer. Чтобы GC не собрал overflow buckets как недостижимые, runtime хранит дополнительные ссылки в `mapextra`:

```go
type mapextra struct {
	overflow     *[]*bmap
	oldoverflow  *[]*bmap
	nextOverflow *bmap
}
```

- `overflow` удерживает overflow buckets текущего массива;
- `oldoverflow` удерживает их для `oldbuckets` во время grow;
- `nextOverflow` помогает переиспользовать заранее выделенные overflow buckets.

Это хороший пример связи runtime layout с GC: отсутствие pointers в `K` и `V` меняет не семантику map, а способ, которым runtime сохраняет служебные allocations живыми.

## Инкрементальный рост

Runtime не переносит всю map одной длинной операцией:

```text
before grow: buckets     -> old array
after start: oldbuckets  -> old array
             buckets     -> new array, usually 2x
```

Следующие writes выполняют небольшую часть evacuation work. Reads помогают корректно найти данные, но не продвигают grow. Пока перенос не завершён, lookup выбирает старый или новый массив в зависимости от состояния нужного bucket.

При удвоении каждый старый bucket делится на два новых по следующему bit hash:

```text
old bucket i
    |
    +-- next hash bit = 0 --> new bucket i
    |
    +-- next hash bit = 1 --> new bucket i + oldBucketCount
```

Так runtime распределяет стоимость роста по нескольким map operations, но усложняет lookup и iteration во время evacuation.

В header при этом меняются роли полей:

```text
до grow:
    buckets     -> active array
    oldbuckets  -> nil

во время grow:
    buckets     -> destination array
    oldbuckets  -> source array
    nevacuate   -> evacuation progress

после grow:
    buckets     -> active array
    oldbuckets  -> nil
```

Runtime не начинает следующий grow, пока текущая evacuation не завершена. Это ограничивает число одновременно живых поколений bucket arrays двумя.

## Как runtime эвакуирует bucket

Каждая изменяющая операция вызывает `growWork`: runtime эвакуирует старый bucket, связанный с bucket текущей операции, и дополнительно продвигает `nevacuate`. Поэтому работа распределяется по writes, но нужный участок не остаётся навсегда в старом массиве.

При удвоении используются две цели, которые runtime называет `x` и `y`:

```text
old bucket index = i
old bucket count = N

hash & N == 0  -> x: new bucket i
hash & N != 0  -> y: new bucket i + N
```

`N` — новый значимый bit bucket index. Остальные младшие bits уже определили старый bucket, поэтому для выбора `x/y` достаточно проверить именно его.

<details>
<summary>Пошаговый пример evacuation</summary>

Пусть до grow `B=2`, то есть существуют четыре старых buckets. После удвоения `B=3`, и новый массив содержит восемь buckets.

Для старого bucket `1` возможны только две цели:

```text
old bucket: 001

next bit = 0 -> new bucket 001 = 1
next bit = 1 -> new bucket 101 = 5 = 1 + 4
```

Допустим, chain старого bucket содержит четыре entries:

```text
A: hash ...0001 -> bucket 1
B: hash ...0101 -> bucket 5
C: hash ...1001 -> bucket 1
D: hash ...1101 -> bucket 5
```

Во время evacuation runtime:

1. проходит основной bucket и всю overflow chain;
2. для каждого живого entry проверяет дополнительный bit;
3. копирует entry в destination chain `x` или `y`;
4. записывает в старый `tophash` marker `evacuatedX` или `evacuatedY`;
5. отмечает пустые slots как `evacuatedEmpty`;
6. после завершения bucket lookup использует новый массив.

Распределение не требует заново менять младшие bits: при удвоении добавляется ровно один bit индекса.

</details>

При same-size grow число buckets не меняется, поэтому все живые entries старого bucket попадают в bucket с тем же индексом. Цель такого переноса — уплотнить chain, а не перераспределить entries по большему массиву.

### Зачем нужны evacuated markers

Одного `nevacuate` недостаточно, чтобы ответить на все вопросы lookup и iterator: связанный с текущей записью bucket может быть эвакуирован раньше общего cursor. Markers непосредственно в `tophash` дают локальный ответ:

- эвакуирован ли конкретный bucket;
- в какую половину нового массива переходит entry;
- пуст ли старый slot.

После завершения всего grow старый массив больше не нужен, `oldbuckets` очищается, и GC может освободить его, когда не остаётся других служебных ссылок.

### Что происходит, если writes прекращаются

Evacuation не работает в фоне. Если после запуска grow map становится read-only, чтения продолжают корректно работать со старым и новым массивами, но `growWork` больше не вызывается. Поэтому `oldbuckets` может оставаться живым до следующего write или delete.

Это редкая, но полезная для диагностики деталь: временный пик памяти сразу после grow не обязан исчезать только потому, что дальнейшая нагрузка состоит из reads.

<details>
<summary>Почему элемент map нельзя адресовать</summary>

```go
type Point struct{ X, Y int }

m := map[string]Point{"a": {X: 1, Y: 2}}

// Не компилируется:
// m["a"].X = 10
// p := &m["a"]

p := m["a"]
p.X = 10
m["a"] = p
```

Неaddressable map element — правило языка, которое действует и после перехода на Swiss Tables. Перемещение elements при grow помогает понять, почему стабильный address становится неудобным контрактом, но само правило задаёт specification, а не конкретный `hmap` layout.

</details>

## Delete и память

`delete(m, key)` находит slot, очищает key/value для GC и отмечает metadata как empty. Bucket allocation при этом не исчезает.

Runtime не всегда может поставить `emptyRest`. Если дальше в текущем bucket или overflow chain находятся живые entries, lookup обязан продолжать scan, поэтому удалённый slot получает `emptyOne`. Если после него ничего нет, runtime может превратить последовательность пустых slots в `emptyRest` и позволить будущему lookup остановиться раньше.

```text
до delete:
[ A ][ B ][ C ][ emptyRest ]

delete B:
[ A ][ emptyOne ][ C ][ emptyRest ]
        ^ lookup C должен пройти дальше

delete C:
[ A ][ emptyRest ][ emptyRest ][ emptyRest ]
        ^ теперь scan можно закончить
```

Очистка key/value зависит от их layout. Если данные содержат pointers, runtime зануляет их, чтобы удалённые объекты не удерживались через bucket. Для pointer-free данных важнее освободить логический slot; сам backing bucket остаётся allocation map.

Эта map не уменьшает число основных buckets после массового delete. Same-size grow убирает лишние overflow chains, но не является shrink по низкой заполненности.

```go
m := make(map[int]int)
for i := range 1_000_000 {
	m[i] = i
}
for i := range 1_000_000 {
	delete(m, i)
}
fmt.Println(len(m)) // 0; это ничего не обещает о retained backing memory
```

Если долгоживущий cache проходит через большой peak, практическое решение — заменить map новым экземпляром и позволить GC собрать старый. Возврат RSS операционной системе всё равно не гарантирован немедленно.

<details>
<summary>Практический паттерн пересоздания map</summary>

```go
type Cache struct {
	items map[string]*Item
}

func (c *Cache) Reset() {
	// Старый backing storage становится доступен для GC,
	// если на него больше никто не ссылается.
	c.items = make(map[string]*Item)
}
```

Для concurrent cache нужна синхронизация вокруг замены map. Само пересоздание не обещает немедленное уменьшение RSS: сначала GC определяет старую map как недостижимую, затем runtime управляет освобождёнными страницами.

</details>

## Как работает итератор

Итератор `range` хранит больше состояния, чем просто индекс текущего bucket:

- pointer на `hmap` и снимок нужного bucket state;
- случайный начальный bucket;
- начальный offset внутри bucket;
- текущий bucket и overflow chain;
- состояние, необходимое для grow и уже эвакуированных entries.

Случайный старт и offset мешают application code случайно привыкнуть к одному порядку. Но правильный контракт ещё проще: specification не гарантирует порядок iteration, поэтому нельзя использовать детали runtime для его предсказания.

Во время grow iterator должен избегать потери и повторной выдачи entries. Для этого он учитывает старый массив и evacuation state. Из-за этой логики `hmap.flags` также сообщает runtime, что активна iteration и что старые overflow buckets пока могут понадобиться.

<details>
<summary>Что можно и нельзя выводить из внутренностей iterator</summary>

Можно:

- понимать, почему порядок `range` не сортирован;
- понимать, почему порядок может отличаться между запусками и iterations;
- увидеть, почему incremental grow усложняет iterator.

Нельзя:

- считать порядок случайной равномерной перестановкой;
- полагаться на «циклический сдвиг» текущего расположения;
- использовать первый полученный key как unbiased random choice;
- переносить детали Go 1.23 на Go 1.24+.

Если нужен стабильный порядок, keys явно собирают и сортируют.

</details>

## Что объясняет эта модель

Историческая реализация полезно объясняет:

- зачем hash делится на bucket index и fingerprint;
- почему collision требует полного сравнения key;
- как overflow ухудшает locality;
- как работает incremental grow;
- почему runtime layout нельзя считать API.

Но пользовательская семантика не выводится из `hmap`: правила nil map, `range`, comparable keys и concurrency задаются language/runtime contract и разобраны в [gotchas](./03-puzzles-and-gotchas.md).

## Сравнение со Swiss Tables

| | hmap до Go 1.24 | Swiss Tables с Go 1.24 |
| --- | --- | --- |
| Collision strategy | bucket + overflow chaining | open addressing по groups |
| Metadata | `tophash[8]` | 8 control bytes с H2/state |
| Candidate scan | slots bucket по одному | metadata восьми slots проверяется вместе |
| Growth | incremental evacuation старого массива | rebuild одной table; большие maps split на tables |
| Deleted slot | empty metadata | empty или tombstone |

В обоих случаях hash только сужает поиск, а равенство подтверждает full key comparison.

## Interview-ready answer

**1. Как устроена map в Go 1.23 и ниже?**

- `hmap` указывает на массив buckets. Bucket содержит восемь slots, `tophash` fingerprints и optional overflow chain. Младшие bits hash выбирают bucket, а fingerprint сокращает число полных сравнений key.

**2. Зачем нужны overflow buckets?**

- Они хранят элементы, которым не хватает восьми slots основного bucket. Это простое collision handling, но длинная chain ухудшает cache locality и lookup.

**3. Что хранится в `tophash`?**

- Для занятого slot это верхняя часть hash — быстрый фильтр перед полным сравнением key. Значения `0..4` кодируют `emptyRest`, `emptyOne` и состояния evacuation.

**4. Как происходит grow?**

- Runtime выделяет новый массив и сохраняет старый в `oldbuckets`. Следующие writes постепенно эвакуируют старые buckets; при удвоении элементы делятся по следующему bit hash.

**5. Чем double-size grow отличается от same-size grow?**

- Double-size grow увеличивает число основных buckets и снижает среднюю заполненность. Same-size grow сохраняет размер, но перепаковывает entries, чтобы убрать накопившиеся overflow buckets.

**6. Как lookup работает во время grow?**

- Runtime проверяет соответствующий старый bucket. Если он ещё не эвакуирован, данные ищутся в нём; если эвакуирован — в новом массиве. Markers в `tophash` показывают состояние конкретного bucket.

**7. Почему после `delete` память map может остаться большой?**

- Delete очищает entry, но не уменьшает основной bucket array. Для долгоживущей map после большого peak её можно пересоздать; освобождение памяти затем зависит от GC и scavenger.

**8. Зачем нужен `mapextra`?**

- Он удерживает overflow buckets живыми, когда key и value не содержат pointers и GC не сканирует сами buckets. Там же runtime хранит pointer на следующий заранее выделенный overflow bucket.

**9. Что из этой модели актуально сейчас?**

- Общая идея hash → короткий fingerprint → candidates → full key equality. Структуры `hmap/bmap`, overflow chains и incremental evacuation больше не описывают builtin map Go 1.24+.

**10. Почему нельзя взять адрес `m[key]`?**

- Map element по правилам языка неaddressable. В legacy-модели grow физически переносит entries между buckets, поэтому стабильный pointer на slot мешает реализации. Для struct value используют read-modify-write или хранят `*T` как value.

## Официальные источники

- [Go 1.23 runtime map source](https://github.com/golang/go/blob/release-branch.go1.23/src/runtime/map.go)
- [Go 1.24 release notes](https://go.dev/doc/go1.24)
- [Current Swiss Tables source](https://go.dev/src/internal/runtime/maps/)
