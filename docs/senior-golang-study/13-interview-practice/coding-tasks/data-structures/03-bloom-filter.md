# Задача 3: Bloom Filter

Bloom filter — **probabilistic** структура для membership check ("есть ли X в множестве?"). Может давать **false positives** ("есть" когда нет), но **никогда false negatives** ("нет" когда есть).

Главная фишка — **очень компактный**: хранение N элементов с 1% error rate занимает ~10 бит на элемент. Для миллиарда URL'ов — около 1.5 GB вместо ~100 GB для hash set'а.

## Формулировка

> "Реализуй структуру данных для быстрой проверки 'видели ли мы этот ключ раньше'. Допустимо иногда сказать 'видели' для нового (false positive), но **никогда** нельзя сказать 'не видели' для существующего."

Use cases:
- "Видел ли я этот URL?" (web crawler — не crawl'ить дважды)
- "Email в blacklist?"
- "Cache filter" — перед дорогим lookup в БД проверить bloom filter
- DB engine pre-filtering (Cassandra, BigTable используют bloom для index lookups)

---

## Уточняющие вопросы

1. **Сколько элементов ожидается?**
   "N — основа расчёта размера. Bloom filter не resize'ится автоматически."

2. **Какой допустимый false positive rate?**
   "1% — стандарт. 0.1% — нужно больше памяти. 10% — очень мало памяти."

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
Bits: 0001 0001 0000 1000 0000

Add("bob"):    h1=1, h2=7, h3=14  → set bits 1,7,14
Bits: 0101 0001 0000 1010 0000

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
| Memory per item | ~50+ bytes (Go map overhead) | ~10 bits @ 1% FPR |
| False positives | Нет | Да (configurable rate) |
| Delete | Да | Нет (counting bloom — да) |
| Get value | Да | Нет (только yes/no) |
| Performance | O(1) hash + O(1) lookup | O(K) hashes |

Для **просто "видел/не видел"** — bloom filter выигрывает в memory × 10-50x.

---

## Базовое решение

```go
package bloom

import (
    "hash/fnv"
    "math"
)

type Filter struct {
    bits  []uint64  // bit array через uint64 для compactness
    m     uint64    // total bits
    k     uint64    // number of hash functions
}

// New создаёт фильтр для N элементов с false positive rate p.
func New(n uint64, p float64) *Filter {
    m := uint64(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
    k := uint64(math.Ceil(float64(m) / float64(n) * math.Ln2))

    return &Filter{
        bits: make([]uint64, (m+63)/64),
        m:    m,
        k:    k,
    }
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

// hash возвращает две независимые hash function через FNV + поворот.
// Используем "double hashing" чтобы избежать необходимости K hash functions.
func hash(data []byte) (uint64, uint64) {
    h1 := fnv.New64a()
    h1.Write(data)
    sum1 := h1.Sum64()

    h2 := fnv.New64()  // другой вариант FNV
    h2.Write(data)
    sum2 := h2.Sum64()

    return sum1, sum2
}
```

**Использование:**

```go
f := bloom.New(1_000_000, 0.01)  // 1M items, 1% FPR

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

## Production-grade: thread-safe + counting variant

```go
package bloom

import (
    "hash/fnv"
    "math"
    "sync/atomic"
)

// ConcurrentFilter — thread-safe Bloom filter через atomic operations.
type ConcurrentFilter struct {
    bits []atomic.Uint64
    m    uint64
    k    uint64

    // Метрики
    items atomic.Uint64
}

func NewConcurrent(n uint64, p float64) *ConcurrentFilter {
    m := uint64(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
    k := uint64(math.Ceil(float64(m) / float64(n) * math.Ln2))

    return &ConcurrentFilter{
        bits: make([]atomic.Uint64, (m+63)/64),
        m:    m,
        k:    k,
    }
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
    f.items.Add(1)
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

// CurrentFalsePositiveRate возвращает теоретическую FPR для текущего числа items.
func (f *ConcurrentFilter) CurrentFalsePositiveRate() float64 {
    n := float64(f.items.Load())
    if n == 0 {
        return 0
    }
    return math.Pow(1-math.Exp(-float64(f.k)*n/float64(f.m)), float64(f.k))
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

**Трade-off:** памяти больше (uint16 вместо 1 bit = ×16), но поддерживает remove.

---

## Тесты

```go
func TestBloom_NoFalseNegatives(t *testing.T) {
    f := New(1000, 0.01)

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

    f := New(n, targetFPR)

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

    // С tolerance 2x — пройдёт почти всегда
    if actualFPR > targetFPR*2 {
        t.Errorf("FPR too high: %f > %f", actualFPR, targetFPR*2)
    }
}

func TestBloom_Concurrent(t *testing.T) {
    f := NewConcurrent(100_000, 0.01)

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

Используй FNV, MurmurHash, xxhash — статистически uniform.

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

Используй uint16 (max 65k) или больше. Или ограничь Add при достижении max.

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

Если N очень большой и FPR должен быть низкий — память растёт линейно. 100B items @ 0.1% = 1.5 TB. Здесь уже **HyperLogLog** для cardinality estimation или distributed sketch.

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

Альтернатива с **delete support** и в 1.5x меньше памяти. Более новая структура. Cassandra использует.

### 3. Compressed Bloom Filter

Bit array compress'ить (для serialization). Меньше bandwidth для distributed scenarios.

### 4. Persistent Bloom

Mmap file → bit array на disk. Для огромных filter'ов которые не помещаются в RAM (Cassandra row index).

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

- **Cassandra / RocksDB / Bigtable** — pre-filter row reads (есть ли ключ в SSTable перед чтением с диска)
- **CDN** — есть ли URL в blacklist
- **Chrome safe browsing** — список вредоносных URL'ов
- **Bitcoin SPV** — wallet bloom filter для privacy
- **Web crawler** — visited URLs
- **Database query optimization** — join elimination

---

## Что важно показать на собеседовании

1. **Probabilistic** характер — false positive ok, false negative — нет.
2. **Memory эффективность** — ~10 бит на элемент @ 1% FPR.
3. **Расчёт m и k** — формулы по N и p.
4. **Double hashing** — оптимизация, k hashes через 2.
5. **Use cases** — Cassandra/Bigtable pre-filter, web crawler visited set.
6. **Trade-offs** — vs hash set (memory) и Cuckoo filter (delete + меньше memory).
7. **Counting Bloom** для delete'а.
8. **`bits-and-blooms/bloom`** — production library в Go.

## Связки

- [Algorithms](../../../17-algorithms-and-data-structures/README.md) — теория hash functions
- [Cache patterns](../../../06-databases/caching/01-redis-as-cache.md) — bloom перед cache
- [bits-and-blooms/bloom](https://github.com/bits-and-blooms/bloom) — Go library
- [Wikipedia: Bloom Filter](https://en.wikipedia.org/wiki/Bloom_filter) — math и proofs
