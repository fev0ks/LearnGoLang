# Задача 3: Bloom Filter

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Механика и параметры](#теория)
- [Базовая реализация](#базовое-решение)
- [Конкурентный и counting-варианты](#потокобезопасный-и-counting-варианты)
- [Тесты](#тесты)
- [Типичные ошибки](#подводные-камни)
- [Расширения](#возможные-расширения)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связки)

Bloom filter — вероятностная структура для проверки принадлежности. Ответ
`false` означает «ключа точно нет», а `true` — «ключ, возможно, есть». Гарантия
об отсутствии false negative действует при неизменной конфигурации, корректной
синхронизации и отсутствии небезопасного удаления.

При целевой вероятности false positive `1%` требуется примерно `9.6` бита на
ожидаемый элемент. Для миллиарда элементов это около `1.2 GB` только под битовый
массив.

---

## Формулировка

> "Реализуй структуру данных для быстрой проверки 'видели ли мы этот ключ раньше'. Допустимо иногда сказать 'видели' для нового (false positive), но **никогда** нельзя сказать 'не видели' для существующего."

Use cases:
- "Видел ли я этот URL?" (web crawler — не crawl'ить дважды)
- "Email в blacklist?"
- "Cache filter" — перед дорогим lookup в БД проверить bloom filter
- Предварительная проверка SSTable перед чтением данных с диска

---

## Уточняющие вопросы

1. **Сколько элементов ожидается?**
   "N — основа расчёта размера. Bloom filter не resize'ится автоматически."

2. **Какой допустимый false positive rate?**
   "Значение зависит от цены лишнего lookup и бюджета памяти. Чем меньше FPR,
   тем больше бит и хешей требуется."

3. **Нужно ли удалять элементы?**
   "Стандартный Bloom — нет (false positives могут вырасти). Counting Bloom — да."

4. **Concurrent access?**
   "Часто read-heavy после initial fill. Можно sync.RWMutex или atomic operations."

5. **Persistence?**
   "Можно сериализовать bit array."

6. **Distributed?**
   "Каждый pod свой filter; periodic sync через broker если нужно."

---

## Теория

### Как работает

1. Bit array размера m (всё нули)
2. K hash функций h1, h2, ..., hK
3. **Add(x):** установить биты на позициях h1(x) % m, h2(x) % m, ..., hK(x) % m
4. **Contains(x):** проверить что **все** биты на этих позициях = 1

```
m = 16 bits, k = 3 hash functions

Add("alice"):  h1=3, h2=7, h3=12  → set bits 3,7,12
Set positions: {3, 7, 12}

Add("bob"):    h1=1, h2=7, h3=14  → set bits 1,7,14
Set positions: {1, 3, 7, 12, 14}

Contains("alice"): check 3,7,12 → все 1 → "yes"
Contains("eve"):   h1=5, h2=9, h3=11 → 0,0,0 → "no"
Contains("charlie"): h1=1, h2=7, h3=12 → все 1 → "yes" (но мы не добавляли!) ← FALSE POSITIVE
```

### Параметры

Связь между N (элементов), m (бит), k (hash функций), p (false positive rate):

```
m = -N * ln(p) / (ln(2)^2)
k = (m/N) * ln(2)
```

Примеры:
- N=1M, p=0.01 → m ≈ 9.6M bits (1.2 MB), k ≈ 7
- N=1B, p=0.01 → m ≈ 9.6B bits (1.2 GB), k ≈ 7
- N=1M, p=0.001 → m ≈ 14.4M bits (1.8 MB), k ≈ 10

False positive rate растёт по мере добавления — формула выше **для известного N**. Если добавить больше — p вырастет.

### Compared to hash set

| | Hash Set | Bloom Filter |
|---|---|---|
| Memory per item | зависит от ключа, значения и реализации table | ~9.6 бит @ 1% FPR |
| False positives | Нет | Да (configurable rate) |
| Delete | Да | Нет (counting bloom — да) |
| Get value | Да | Нет (только yes/no) |
| Performance | O(1) hash + O(1) lookup | O(K) hashes |

Bloom filter полезен, когда достаточно предварительного ответа о принадлежности,
а точная хеш-таблица не помещается в допустимый объём памяти. Конкретный выигрыш
зависит от размера ключей и реализации точного множества.

---

## Базовое решение

```go
package bloom

import (
    "crypto/sha256"
    "encoding/binary"
    "errors"
    "math"
)

var ErrInvalidParameters = errors.New("bloom: n must be positive and p must be between 0 and 1")

type Filter struct {
    bits  []uint64  // bit array через uint64 для compactness
    m     uint64    // total bits
    k     uint64    // number of hash functions
}

// New создаёт фильтр для N элементов с false positive rate p.
func New(n uint64, p float64) (*Filter, error) {
    m, k, err := parameters(n, p)
    if err != nil {
        return nil, err
    }

    return &Filter{
        bits: make([]uint64, (m+63)/64),
        m:    m,
        k:    k,
    }, nil
}

func (f *Filter) Add(data []byte) {
    h1, h2 := hash(data)
    for i := uint64(0); i < f.k; i++ {
        // Двойное хеширование: h_i = h1 + i*h2
        pos := (h1 + i*h2) % f.m
        f.bits[pos/64] |= 1 << (pos % 64)
    }
}

func (f *Filter) Contains(data []byte) bool {
    h1, h2 := hash(data)
    for i := uint64(0); i < f.k; i++ {
        pos := (h1 + i*h2) % f.m
        if f.bits[pos/64]&(1<<(pos%64)) == 0 {
            return false  // точно нет
        }
    }
    return true  // может быть (false positive возможен)
}

func parameters(n uint64, p float64) (uint64, uint64, error) {
    if n == 0 || math.IsNaN(p) || p <= 0 || p >= 1 {
        return 0, 0, ErrInvalidParameters
    }

    m := uint64(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
    k := uint64(math.Round(float64(m) / float64(n) * math.Ln2))
    if k == 0 {
        k = 1
    }
    return m, k, nil
}

// hash получает две 64-битные части одного криптографического digest.
// Для учебного примера это проще проверить, чем набор собственных хешей.
func hash(data []byte) (uint64, uint64) {
    digest := sha256.Sum256(data)
    h1 := binary.LittleEndian.Uint64(digest[0:8])
    h2 := binary.LittleEndian.Uint64(digest[8:16])
    if h2 == 0 {
        h2 = 0x9e3779b97f4a7c15
    }
    return h1, h2
}
```

**Использование:**

```go
f, err := bloom.New(1_000_000, 0.01)
if err != nil {
    log.Fatal(err)
}

f.Add([]byte("alice@example.com"))
f.Add([]byte("bob@example.com"))

if f.Contains([]byte("alice@example.com")) {
    // Точно или возможно — есть
}
if !f.Contains([]byte("eve@example.com")) {
    // Точно нет
}
```

**Ключевые моменты:**
- **Bit array через []uint64** — компактнее `[]bool` (1 bit vs 8 bits)
- **Double hashing** — `h_i = h1 + i*h2` симулирует k независимых hash'ей через 2 (стандартный трюк Kirsch-Mitzenmacher)
- **`(m+63)/64` для размера uint64 array** — округление вверх
- **`pos/64` для index, `pos%64` для bit** в uint64

---

## Потокобезопасный и counting-варианты

```go
package bloom

import (
    "sync/atomic"
)

// ConcurrentFilter — thread-safe Bloom filter через atomic operations.
type ConcurrentFilter struct {
    bits []atomic.Uint64
    m    uint64
    k    uint64

}

func NewConcurrent(n uint64, p float64) (*ConcurrentFilter, error) {
    m, k, err := parameters(n, p)
    if err != nil {
        return nil, err
    }

    return &ConcurrentFilter{
        bits: make([]atomic.Uint64, (m+63)/64),
        m:    m,
        k:    k,
    }, nil
}

func (f *ConcurrentFilter) Add(data []byte) {
    h1, h2 := hash(data)
    for i := uint64(0); i < f.k; i++ {
        pos := (h1 + i*h2) % f.m
        idx := pos / 64
        bit := uint64(1) << (pos % 64)

        // Atomic OR
        for {
            old := f.bits[idx].Load()
            if old&bit != 0 {
                break  // bit уже set
            }
            if f.bits[idx].CompareAndSwap(old, old|bit) {
                break
            }
        }
    }
}

func (f *ConcurrentFilter) Contains(data []byte) bool {
    h1, h2 := hash(data)
    for i := uint64(0); i < f.k; i++ {
        pos := (h1 + i*h2) % f.m
        idx := pos / 64
        bit := uint64(1) << (pos % 64)
        if f.bits[idx].Load()&bit == 0 {
            return false
        }
    }
    return true
}

```

**Counting Bloom Filter** (поддерживает delete):

```go
// Вместо bits — counter в каждой позиции (обычно uint8 или uint16).
type CountingFilter struct {
    counters []uint16  // не bits, а counters
    m, k     uint64
}

func (f *CountingFilter) Add(data []byte) {
    h1, h2 := hash(data)
    for i := uint64(0); i < f.k; i++ {
        pos := (h1 + i*h2) % f.m
        if f.counters[pos] < math.MaxUint16 {
            f.counters[pos]++
        }
    }
}

func (f *CountingFilter) Remove(data []byte) {
    h1, h2 := hash(data)
    for i := uint64(0); i < f.k; i++ {
        pos := (h1 + i*h2) % f.m
        if f.counters[pos] > 0 {
            f.counters[pos]--
        }
    }
}
```

Счётчик `uint16` требует в 16 раз больше памяти, чем один бит. `Remove` безопасен
только для элемента, который действительно был добавлен нужное число раз.
Удаление отсутствующего элемента уменьшит общие счётчики и может создать false
negative для других ключей. Насыщение счётчика также нужно учитывать.

---

## Тесты

```go
func TestBloom_NoFalseNegatives(t *testing.T) {
    f, err := New(1000, 0.01)
    if err != nil {
        t.Fatal(err)
    }

    items := make([]string, 1000)
    for i := range items {
        items[i] = fmt.Sprintf("item-%d", i)
        f.Add([]byte(items[i]))
    }

    // Все добавленные должны давать true (нет false negative)
    for _, item := range items {
        if !f.Contains([]byte(item)) {
            t.Errorf("missed: %s", item)
        }
    }
}

func TestBloom_FalsePositiveRate(t *testing.T) {
    n := uint64(10_000)
    targetFPR := 0.01

    f, err := New(n, targetFPR)
    if err != nil {
        t.Fatal(err)
    }

    // Добавляем N элементов
    for i := uint64(0); i < n; i++ {
        f.Add([]byte(fmt.Sprintf("added-%d", i)))
    }

    // Проверяем N других — считаем false positives
    var falsePositives int
    testSize := 100_000
    for i := 0; i < testSize; i++ {
        s := fmt.Sprintf("test-%d", i)
        if f.Contains([]byte(s)) {
            falsePositives++
        }
    }

    actualFPR := float64(falsePositives) / float64(testSize)
    t.Logf("FPR: actual %.4f, target %.4f", actualFPR, targetFPR)

    // Большая детерминированная выборка оставляет запас на статистический шум.
    if actualFPR > targetFPR*2 {
        t.Errorf("FPR too high: %f > %f", actualFPR, targetFPR*2)
    }
}

func TestBloom_Concurrent(t *testing.T) {
    f, err := NewConcurrent(100_000, 0.01)
    if err != nil {
        t.Fatal(err)
    }

    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            for j := 0; j < 10000; j++ {
                f.Add([]byte(fmt.Sprintf("w%d-item-%d", workerID, j)))
            }
        }(i)
    }
    wg.Wait()

    // Все must be found
    for w := 0; w < 10; w++ {
        for j := 0; j < 10000; j++ {
            if !f.Contains([]byte(fmt.Sprintf("w%d-item-%d", w, j))) {
                t.Errorf("missed w%d-item-%d", w, j)
            }
        }
    }
}
```

---

## Подводные камни

### 1. Несбалансированные hash functions

```go
// ❌ "Hash function" = взять первый байт
h := uint64(data[0])
// Слишком мало uniqueness — все начинающиеся одинаково clashes
```

Нужен качественно перемешивающий хеш. Для недоверенного ввода отдельно важна
защита от специально подобранных коллизий.

### 2. False positive rate растёт с числом items

```go
f := bloom.New(1_000, 0.01)
// Добавляем 1_000_000 элементов (× 1000 больше планируемого)
// → FPR не 1%, а почти 100%
```

Bloom filter **не resize**. Если N может расти — лучше **scalable bloom filter** (несколько filters in series, каждый больше предыдущего).

### 3. K hash function вычислений

Каждый Add/Contains делает K хешей. Для K=10 и миллионов lookups в секунду — CPU нагрузка. **Double hashing** оптимизирует: считаем 2 hash, симулируем K через `h1 + i*h2`.

### 4. Memory layout не cache-friendly

```go
bits := make([]uint64, m/64)
// Random access по pos%m → cache misses
```

Для огромных filter'ов (GB) — partition'ируй на меньшие блоки, размер блока ≤ L3 cache. Хеш сначала определяет блок, потом позицию внутри.

### 5. Concurrent set без atomic = race

```go
bits[idx] |= mask  // ← read-modify-write, race condition в Go race detector
```

Используй `atomic.Uint64` или mutex.

### 6. Counting Bloom overflow

```go
counters []uint8  // ← max 255
```

Hot key добавляется 1000 раз → counter переполняется → delete сломается.

Насыщение вместо переполнения предотвращает wraparound, но после насыщения
точное удаление всё равно невозможно: структура потеряла число добавлений.

### 7. Hash сидинг для security

Если злоумышленник знает hash функции — может **специально создать collisions** для DoS. Защита — randomized hash seed (как SipHash). Для большинства задач — не критично.

### 8. Не сохранять filter в JSON

Bit array — binary. JSON сериализация бесполезна (raw bytes как base64 — OK).

### 9. False positive = "yes, maybe"

Запомни семантику:
- `Contains == true` → "может быть да" (нужна verification через real lookup)
- `Contains == false` → "точно нет"

```go
if filter.Contains(key) {
    // Дорогой lookup в БД
    if exists, _ := db.Get(key); exists {
        // действительно есть
    }
    // иначе false positive
}
```

### 10. Не для большого set'а уникальных

Если `N` очень велико и FPR должен быть низким, память растёт линейно. Для
`100` миллиардов элементов и `p = 0.001` формула даёт примерно `1.44` триллиона
бит, то есть около `180 GB`. HyperLogLog требует намного меньше памяти, но решает
другую задачу — оценивает число уникальных элементов и не проверяет
принадлежность конкретного ключа.

---

## Возможные расширения

### 1. Scalable Bloom Filter

Когда N неизвестен — используется serial array filters, каждый в 2 раза больше:
```
Filter 0: capacity N0, FPR=0.01
Filter 1: capacity 2N0, FPR=0.005
Filter 2: capacity 4N0, FPR=0.0025
```

Add → в последний. Contains → проверить все. Cumulative FPR контролируется.

### 2. Cuckoo Filter

Альтернатива, которая поддерживает удаление и хранит короткие fingerprints в
бакетах. Сравнение памяти зависит от целевого FPR и заполнения; Cassandra здесь
не является примером, её SSTable lookup традиционно использует Bloom filters.

### 3. Compressed Bloom Filter

Bit array compress'ить (для serialization). Меньше bandwidth для distributed scenarios.

### 4. Persistent Bloom

Битовый массив можно хранить в отдельном бинарном формате или отображать в
память через `mmap`. Формат должен фиксировать размер, число хешей, алгоритм
хеширования и порядок байт.

### 5. Bloom + LRU cache

Перед dispatch'ем в дорогой backend (БД):
```go
if !bloom.Contains(key) {
    return nil, ErrNotFound  // 100% precision
}
if val, ok := cache.Get(key); ok {
    return val, nil
}
return db.Get(key)
```

Bloom фильтрует "точно нет", cache хранит recent results, DB как ultimate source.

### 6. Distributed Bloom через Redis

Redis имеет `BF.ADD`, `BF.EXISTS` (через RedisBloom module). Distributed bloom без custom code.

### 7. Time-windowed Bloom

Rotating filters для "видели за последний час". Когда window expires — старый filter забывается.

---

## Реальные применения

- **LSM-хранилища —** пропустить SSTable, в которой ключа точно нет.
- **Web crawler —** отсеять URL, которые, вероятно, уже посещались.
- **Cache pre-filter —** не обращаться к cache или storage для заведомо
  отсутствующего ключа.
- **Потоковая обработка —** дешёвая предварительная дедупликация перед точной
  проверкой.

---

## Interview-ready answer

**1. Какую гарантию даёт Bloom filter?**

- Отрицательный ответ — ключ точно не был добавлен при соблюдении контракта.
- Положительный ответ — ключ мог быть добавлен, но возможен false positive.
- Ограничение — удаление, гонки или несовместимые хеши могут разрушить гарантию
  об отсутствии false negative.

**2. Как выбираются размер и число хешей?**

- Входные данные — ожидаемое число элементов `N` и допустимый FPR `p`.
- Размер — `m = -N * ln(p) / ln(2)^2` бит.
- Число хешей — `k = (m/N) * ln(2)` с округлением хотя бы до одного.
- Пример — при `p = 0.01` получается примерно `9.6` бита на элемент и `7` хешей.

**3. Какие основные trade-offs?**

- Память — значительно меньше точного множества, но значения получить нельзя.
- CPU — каждая операция проверяет `k` позиций; double hashing сокращает число
  полных вычислений хеша.
- Ёмкость — превышение запланированного `N` повышает FPR, поэтому при неизвестном
  росте нужен scalable-вариант.
- Удаление — обычный Bloom filter его не поддерживает; counting-вариант требует
  больше памяти и строгого контракта удаления.

---

## Связки

- [Algorithms](../../../16-algorithms-and-data-structures/README.md) — теория hash functions
- [Cache patterns](../../../06-databases/caching/01-redis-as-cache.md) — bloom перед cache
- [bits-and-blooms/bloom](https://github.com/bits-and-blooms/bloom) — Go library
- [Wikipedia: Bloom Filter](https://en.wikipedia.org/wiki/Bloom_filter) — math и proofs
