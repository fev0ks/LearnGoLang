# Задача 1: LRU Cache

**Самая частая** задача на структуры данных. Спрашивают почти везде. Тестирует понимание hashmap, linked list, и Go-специфики типов/интерфейсов.

## Формулировка

> "Реализуй LRU (Least Recently Used) cache: фиксированный размер, при заполнении выбрасывается давно не используемый элемент. Get и Put — за O(1)."

Вариации:
- "Кэш с TTL и LRU eviction"
- "Concurrent LRU для production"
- "LeetCode 146"

---

## Уточняющие вопросы

1. **Что считать "использованием" — get или put?**
   Стандартно — оба. Get обновляет order.

2. **Capacity фиксированная или dynamic?**
   Обычно фиксированная.

3. **Thread safety нужна?**
   Для собеседования — спроси. Часто да.

4. **Тип ключа и значения — generic или конкретные?**
   Generics (Go 1.18+) сейчас стандарт.

5. **Что делать при miss?**
   Вернуть zero value + `ok bool` (как в map). Не паника, не error.

6. **TTL нужен?**
   "Это уже расширение. Базовый LRU — без TTL."

---

## Решение: O(1) через hashmap + doubly linked list

**Идея:** hashmap даёт O(1) lookup. Linked list даёт O(1) move-to-front и remove. Комбинация — O(1) на всё.

```
Hashmap: key → *Node
                  ↓
Doubly linked list (head ← → tail):
  [head] ← [most recent] ← ... ← [least recent] ← [tail]

При Get: найти node через map → переместить в head
При Put: новый node в head; если over capacity — удалить из tail
```

### Базовая реализация

```go
package lru

type entry[K comparable, V any] struct {
    key  K
    val  V
    prev *entry[K, V]
    next *entry[K, V]
}

type Cache[K comparable, V any] struct {
    capacity int
    items    map[K]*entry[K, V]
    head     *entry[K, V]  // most recent
    tail     *entry[K, V]  // least recent
}

func New[K comparable, V any](capacity int) *Cache[K, V] {
    if capacity <= 0 {
        capacity = 1
    }
    // Sentinel head/tail для упрощения операций с linked list
    head := &entry[K, V]{}
    tail := &entry[K, V]{}
    head.next = tail
    tail.prev = head

    return &Cache[K, V]{
        capacity: capacity,
        items:    make(map[K]*entry[K, V], capacity),
        head:     head,
        tail:     tail,
    }
}

// Get возвращает value и true если ключ есть. Обновляет recency.
func (c *Cache[K, V]) Get(key K) (V, bool) {
    var zero V
    e, ok := c.items[key]
    if !ok {
        return zero, false
    }
    c.moveToFront(e)
    return e.val, true
}

// Put добавляет или обновляет ключ.
func (c *Cache[K, V]) Put(key K, val V) {
    if e, ok := c.items[key]; ok {
        // Update existing
        e.val = val
        c.moveToFront(e)
        return
    }

    // New entry
    e := &entry[K, V]{key: key, val: val}
    c.items[key] = e
    c.addToFront(e)

    if len(c.items) > c.capacity {
        // Evict LRU
        lru := c.tail.prev
        c.remove(lru)
        delete(c.items, lru.key)
    }
}

// Len возвращает текущий размер.
func (c *Cache[K, V]) Len() int {
    return len(c.items)
}

// --- internal linked list operations ---

func (c *Cache[K, V]) addToFront(e *entry[K, V]) {
    e.prev = c.head
    e.next = c.head.next
    c.head.next.prev = e
    c.head.next = e
}

func (c *Cache[K, V]) remove(e *entry[K, V]) {
    e.prev.next = e.next
    e.next.prev = e.prev
    e.prev = nil  // help GC
    e.next = nil
}

func (c *Cache[K, V]) moveToFront(e *entry[K, V]) {
    c.remove(e)
    c.addToFront(e)
}
```

**Использование:**

```go
cache := lru.New[string, *User](100)

cache.Put("alice", &User{Name: "Alice"})
cache.Put("bob", &User{Name: "Bob"})

if user, ok := cache.Get("alice"); ok {
    fmt.Println(user.Name)
}
```

**Что важно объяснить:**
- **Sentinel head/tail** — упрощают edge cases (нет проверок на nil prev/next)
- **Doubly linked** — нужны обе ссылки для O(1) remove (если знать только next, нужно идти от head)
- **Map хранит `*entry`** не значение — нужно для прямого доступа к node в list
- **`delete(items, lru.key)`** — обязательно, иначе map растёт

---

## Production-grade: thread safety + TTL + metrics

```go
package lru

import (
    "container/list"
    "sync"
    "time"
)

type entry[K comparable, V any] struct {
    key       K
    val       V
    expiresAt time.Time  // если zero — нет expiration
}

type Cache[K comparable, V any] struct {
    mu       sync.Mutex
    capacity int
    items    map[K]*list.Element
    order    *list.List  // *entry[K,V] — MRU at front
    ttl      time.Duration

    // Metrics
    hits   uint64
    misses uint64
    evicts uint64
}

func New[K comparable, V any](capacity int, ttl time.Duration) *Cache[K, V] {
    return &Cache[K, V]{
        capacity: capacity,
        items:    make(map[K]*list.Element, capacity),
        order:    list.New(),
        ttl:      ttl,
    }
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()

    var zero V
    elem, ok := c.items[key]
    if !ok {
        c.misses++
        return zero, false
    }

    e := elem.Value.(*entry[K, V])

    // Check TTL
    if c.ttl > 0 && time.Now().After(e.expiresAt) {
        c.removeElement(elem)
        c.misses++
        return zero, false
    }

    c.order.MoveToFront(elem)
    c.hits++
    return e.val, true
}

func (c *Cache[K, V]) Put(key K, val V) {
    c.mu.Lock()
    defer c.mu.Unlock()

    if elem, ok := c.items[key]; ok {
        // Update existing
        e := elem.Value.(*entry[K, V])
        e.val = val
        if c.ttl > 0 {
            e.expiresAt = time.Now().Add(c.ttl)
        }
        c.order.MoveToFront(elem)
        return
    }

    // New entry
    e := &entry[K, V]{key: key, val: val}
    if c.ttl > 0 {
        e.expiresAt = time.Now().Add(c.ttl)
    }
    elem := c.order.PushFront(e)
    c.items[key] = elem

    // Evict if over capacity
    if c.order.Len() > c.capacity {
        oldest := c.order.Back()
        if oldest != nil {
            c.removeElement(oldest)
            c.evicts++
        }
    }
}

func (c *Cache[K, V]) Delete(key K) bool {
    c.mu.Lock()
    defer c.mu.Unlock()

    elem, ok := c.items[key]
    if !ok {
        return false
    }
    c.removeElement(elem)
    return true
}

func (c *Cache[K, V]) Len() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.order.Len()
}

type Stats struct {
    Hits   uint64
    Misses uint64
    Evicts uint64
}

func (c *Cache[K, V]) Stats() Stats {
    c.mu.Lock()
    defer c.mu.Unlock()
    return Stats{Hits: c.hits, Misses: c.misses, Evicts: c.evicts}
}

func (c *Cache[K, V]) removeElement(elem *list.Element) {
    e := elem.Value.(*entry[K, V])
    c.order.Remove(elem)
    delete(c.items, e.key)
}
```

**Что добавлено:**
- **`container/list`** — стандартный doubly linked list, не писать свой
- **mutex** — thread safety
- **TTL** — каждая запись имеет expiry, проверяется на Get
- **Metrics** — hit/miss/evict counters

**Использование:**

```go
cache := lru.New[string, *User](1000, 5*time.Minute)

cache.Put("alice", user)
if u, ok := cache.Get("alice"); ok {
    // ...
}

stats := cache.Stats()
log.Printf("hit rate: %.2f%%", float64(stats.Hits)/float64(stats.Hits+stats.Misses)*100)
```

---

## Тесты

```go
func TestLRU_Basic(t *testing.T) {
    c := New[string, int](2, 0)

    c.Put("a", 1)
    c.Put("b", 2)

    if v, ok := c.Get("a"); !ok || v != 1 {
        t.Errorf("got %v %v, want 1 true", v, ok)
    }

    c.Put("c", 3)  // evicts "b" (oldest)

    if _, ok := c.Get("b"); ok {
        t.Error("b should be evicted")
    }
    if v, _ := c.Get("a"); v != 1 {
        t.Error("a should remain")
    }
}

func TestLRU_GetUpdatesOrder(t *testing.T) {
    c := New[string, int](2, 0)
    c.Put("a", 1)
    c.Put("b", 2)
    c.Get("a")     // a — most recent
    c.Put("c", 3)  // evicts "b"

    if _, ok := c.Get("b"); ok {
        t.Error("b should be evicted, not a")
    }
}

func TestLRU_Update(t *testing.T) {
    c := New[string, int](2, 0)
    c.Put("a", 1)
    c.Put("a", 10)  // update

    if v, _ := c.Get("a"); v != 10 {
        t.Errorf("got %d, want 10", v)
    }
    if c.Len() != 1 {
        t.Errorf("len %d, want 1", c.Len())
    }
}

func TestLRU_TTL(t *testing.T) {
    c := New[string, int](10, 50*time.Millisecond)
    c.Put("a", 1)

    if _, ok := c.Get("a"); !ok {
        t.Error("should hit fresh")
    }

    time.Sleep(100 * time.Millisecond)

    if _, ok := c.Get("a"); ok {
        t.Error("should expire after TTL")
    }
}

func TestLRU_Concurrent(t *testing.T) {
    c := New[int, int](100, 0)

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            for j := 0; j < 1000; j++ {
                c.Put(j%200, id)
                c.Get(j % 200)
            }
        }(i)
    }
    wg.Wait()

    // Не должно быть panic'и или race
    if c.Len() > 100 {
        t.Errorf("len %d > capacity 100", c.Len())
    }
}
```

`go test -race` обязательно.

---

## Подводные камни

### 1. Забыть удалить из map при evict

```go
// ❌ Удалили из list, не удалили из map
c.order.Remove(oldestElem)
// map[K] всё ещё содержит mapping на removed element
```

Map растёт бесконечно. Обязательно `delete(items, key)` при каждом remove.

### 2. Get НЕ обновляет recency

```go
// ❌ Get не двигает в front
func (c *Cache) Get(key K) (V, bool) {
    return c.items[key].Value, true  // нет MoveToFront
}
```

Это уже не LRU. Должен обновлять order.

### 3. Capacity 0 или отрицательный

```go
c := New(0)  // ← никогда не cached
c.Put("a", 1)
// Or ← evict immediately
```

Защита: `if capacity <= 0 { capacity = 1 }` или panic в конструкторе.

### 4. Concurrent без mutex

```go
// ❌ Race condition при одновременном Put/Get
type Cache struct {
    items map[...]*Element  // ← concurrent map read/write panic
}
```

Mutex или явная документация "not thread safe".

### 5. RWMutex для Get кажется хорошим но...

```go
func (c *Cache) Get(key K) (V, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    // ... но Get обновляет linked list (MoveToFront) → нужен write lock!
}
```

Get модифицирует структуру → write lock нужен. RWMutex выигрывает только если ввести "lookup без обновления order".

### 6. Memory leak через value references

```go
type User struct {
    HugeData []byte
}
cache.Put(key, &User{HugeData: ...})
// Когда entry evict'ится, User должен GC'нуться. Но если есть другие ссылки — нет.
```

Это OK behavior, просто помни — cache не освобождает память сам.

### 7. Lock contention на high throughput

100k QPS на global mutex — bottleneck. Решения:
- **Sharded cache** — N independent caches, hash(key) → shard
- **`sync.Map`** — для read-heavy без удаления
- **Lock-free** — сложно для LRU, обычно sharding

### 8. TTL без background cleanup

Expired entries occupy capacity пока кто-то не сделает Get. Если TTL очень разный — медленно cleanup. Можно добавить периодический cleanup goroutine.

### 9. PromoteOnGet vs PromoteOnPut

Если хочешь "frequency-based" → LFU а не LRU. Не путай.

---

## Возможные расширения

### 1. LFU (Least Frequently Used)

Считать количество access'ов. Evict — с наименьшим count'ом. Реализация: bucket per frequency + linked list within bucket.

### 2. ARC (Adaptive Replacement Cache)

Адаптивная: смесь LRU и LFU, динамически меняет баланс. Используется в ZFS, PostgreSQL shared_buffers.

### 3. 2Q

Two queues: in-use и hot. Защита от "одноразовый scan убил cache".

### 4. Sharded LRU

```go
type ShardedCache[K comparable, V any] struct {
    shards [N]*Cache[K, V]
}

func (s *ShardedCache[K, V]) shardFor(key K) *Cache[K, V] {
    h := hash(key)
    return s.shards[h%N]
}
```

Lock contention снижается ~N раз.

### 5. Generic cache via library

[hashicorp/golang-lru](https://github.com/hashicorp/golang-lru) — production-ready.

```go
import "github.com/hashicorp/golang-lru/v2"
c, _ := lru.New[string, *User](1000)
```

Не пиши свой в production — используй проверенную библиотеку. На собеседовании — пиши свой (показывает понимание).

### 6. Eviction callback

```go
cache.OnEvict(func(key K, val V) {
    log.Printf("evicted: %v", key)
    // Cleanup ресурсов (close connection, etc.)
})
```

### 7. Persistent (на disk)

Большие cache на SSD — Badger, BoltDB. Не in-memory.

---

## Что важно показать на собеседовании

1. **O(1) для обеих операций** — это главное. Объясни почему: hashmap + linked list.
2. **`container/list`** — знание стандартной библиотеки.
3. **Sentinel head/tail** — упрощают код.
4. **Thread safety** — обсуди trade-off mutex vs sharding.
5. **Delete from map при evict** — частая ошибка, не сделать.
6. **TTL как extension** — обсуди дополнительно.
7. **Generics (Go 1.18+)** — type-safe API.
8. **`hashicorp/golang-lru` в production** — knowledge of ecosystem.

## Связки

- [Redis as cache](../../../06-databases/caching/01-redis-as-cache.md) — production cache patterns
- [hashicorp/golang-lru](https://github.com/hashicorp/golang-lru) — battle-tested implementation
- [container/list](https://pkg.go.dev/container/list) — std doubly linked list
