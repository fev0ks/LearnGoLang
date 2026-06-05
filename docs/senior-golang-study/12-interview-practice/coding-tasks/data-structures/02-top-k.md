# Задача 2: Top-K Elements

Найти K самых частых/больших/маленьких элементов в потоке или массиве. Очень частая инженерная задача: "топ-10 запросов", "топ покупателей за день", "топ-100 ошибок".

## Формулировка

> "Дан поток данных (или массив). Найди K элементов с наибольшим значением (или частотой). Память O(K), не O(N)."

Вариации:
- "Top 10 hottest URL'ов за последний час"
- "Топ-K по частоте слов в логах"
- "Streaming median (top-1 для p50)"

---

## Уточняющие вопросы

1. **N известен или streaming?**
   "N известен — sort + slice. Streaming — heap, O(N log K) time, O(K) memory."

2. **K << N или K ≈ N?**
   "Если K=100, N=1B — heap идеал. K=N/2 — лучше sort."

3. **По count/частоте или по значению?**
   "Если по частоте — нужен сначала count (map), потом top-K по count'у."

4. **Strict accurate или approximate?**
   "Точный — heap. Approximate (огромный N) — Count-Min Sketch + top-K."

5. **Один проход или несколько?**
   "Streaming = один проход. Иначе можно много."

6. **Concurrent — параллельные writers/readers?**
   "Это extension. Базовый — single thread."

---

## Решение 1: Min-Heap (классика для streaming)

**Идея:** держать heap размера K. Для нового элемента: если больше минимума heap'а — заменить, иначе пропустить.

```
Streaming: 5, 2, 8, 1, 9, 3, ...

K=3:
  [5]           → heap (min at top): [5]
  [2,5]         → [2,5]
  [2,5,8]       → [2,5,8]
  [1] vs 2 → ignore
  [9] vs 2 → replace → [5,8,9]
  [3] vs 5 → ignore
```

Time: O(N log K). Memory: O(K).

```go
package topk

import "container/heap"

// minHeap для int с фиксированным размером K
type minHeap []int

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool  { return h[i] < h[j] }
func (h minHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)         { *h = append(*h, x.(int)) }
func (h *minHeap) Pop() any {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[0 : n-1]
    return x
}

// TopK возвращает K наибольших значений из stream'а.
func TopK(stream <-chan int, k int) []int {
    h := &minHeap{}
    heap.Init(h)

    for v := range stream {
        if h.Len() < k {
            heap.Push(h, v)
        } else if v > (*h)[0] {
            // Заменить min
            (*h)[0] = v
            heap.Fix(h, 0)
        }
    }

    return *h
}
```

**Использование:**

```go
stream := make(chan int)
go func() {
    defer close(stream)
    for _, v := range data {
        stream <- v
    }
}()

top10 := TopK(stream, 10)
sort.Sort(sort.Reverse(sort.IntSlice(top10)))  // если нужно отсортированный
fmt.Println(top10)
```

---

## Решение 2: Top-K по частоте (для слов/URL'ов)

Когда нужно "топ-K самых частых элементов" — два прохода:

```go
package topk

import (
    "container/heap"
)

type freqItem[T comparable] struct {
    val   T
    count int
}

type freqHeap[T comparable] []freqItem[T]

func (h freqHeap[T]) Len() int            { return len(h) }
func (h freqHeap[T]) Less(i, j int) bool  { return h[i].count < h[j].count }
func (h freqHeap[T]) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *freqHeap[T]) Push(x any)         { *h = append(*h, x.(freqItem[T])) }
func (h *freqHeap[T]) Pop() any {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[:n-1]
    return x
}

// TopKByFrequency возвращает K самых частых элементов и их counts.
func TopKByFrequency[T comparable](items []T, k int) []freqItem[T] {
    // 1. Count occurrences
    counts := make(map[T]int)
    for _, item := range items {
        counts[item]++
    }

    // 2. Min-heap по count'у
    h := &freqHeap[T]{}
    heap.Init(h)

    for val, count := range counts {
        if h.Len() < k {
            heap.Push(h, freqItem[T]{val: val, count: count})
        } else if count > (*h)[0].count {
            (*h)[0] = freqItem[T]{val: val, count: count}
            heap.Fix(h, 0)
        }
    }

    return *h
}
```

**Использование:**

```go
words := []string{"foo", "bar", "foo", "baz", "foo", "bar"}
top := TopKByFrequency(words, 2)
// [{val: "bar", count: 2}, {val: "foo", count: 3}]
```

**Time:** O(N + M log K) где M = количество **уникальных** элементов.
**Memory:** O(M) для counts + O(K) для heap.

Если M очень большое (миллиарды уникальных URL'ов) — memory проблема. См. Count-Min Sketch ниже.

---

## Решение 3: Sort (когда N помещается в память и K ≈ N)

```go
func TopKSort(items []int, k int) []int {
    sort.Slice(items, func(i, j int) bool { return items[i] > items[j] })
    if k > len(items) {
        k = len(items)
    }
    return items[:k]
}
```

Time: O(N log N). Memory: O(N) (или O(log N) для in-place sort).

**Когда выбирать:**
- N известен и помещается в RAM
- K не сильно меньше N (e.g., K = N/2)
- Один-shot, не streaming

Heap выигрывает когда K << N (e.g., K=100, N=1B → 100 in memory, log 100 = 7 ops per element).

---

## Решение 4: Quickselect (O(N) average)

Для one-shot задачи — quickselect даёт **O(N) average**, лучше heap'а если K произвольный.

```go
// QuickSelectTopK находит K наибольших за O(N) average через partial quicksort.
func QuickSelectTopK(items []int, k int) []int {
    if k <= 0 {
        return nil
    }
    if k >= len(items) {
        return items
    }

    // Партиционируем так, чтобы первые K были самыми большими
    quickSelect(items, 0, len(items)-1, k)
    return items[:k]
}

func quickSelect(arr []int, lo, hi, k int) {
    if lo >= hi {
        return
    }

    // Pivot — медиана из 3 для defense против worst-case
    pivot := medianOfThree(arr, lo, hi)
    p := partition(arr, lo, hi, pivot)

    // Слева — большие, справа — маленькие (descending order partition)
    if p == k-1 {
        return
    }
    if p < k-1 {
        quickSelect(arr, p+1, hi, k)
    } else {
        quickSelect(arr, lo, p-1, k)
    }
}

// partition: descending — большие слева, маленькие справа
func partition(arr []int, lo, hi, pivot int) int {
    // ...классическая Lomuto или Hoare partition
}
```

В Go 1.21+ есть `slices.SortFunc` и можно через `slices` сделать. Но для O(N) selection — лучше своя реализация или библиотека.

**Trade-off vs heap:**
- Quickselect: O(N) **average**, O(N²) worst-case
- Heap: O(N log K) guaranteed

В production heap чаще выбирают за predictability.

---

## Тесты

```go
func TestTopK_Heap(t *testing.T) {
    stream := make(chan int, 10)
    for _, v := range []int{5, 2, 8, 1, 9, 3, 7, 4, 6, 0} {
        stream <- v
    }
    close(stream)

    result := TopK(stream, 3)
    sort.Sort(sort.Reverse(sort.IntSlice(result)))

    expected := []int{9, 8, 7}
    if !reflect.DeepEqual(result, expected) {
        t.Errorf("got %v, want %v", result, expected)
    }
}

func TestTopK_Frequency(t *testing.T) {
    words := []string{"a", "b", "c", "a", "a", "b", "d"}
    top := TopKByFrequency(words, 2)

    sort.Slice(top, func(i, j int) bool { return top[i].count > top[j].count })

    if len(top) != 2 || top[0].val != "a" || top[0].count != 3 {
        t.Errorf("got %v", top)
    }
}

func TestTopK_LargeStream(t *testing.T) {
    stream := make(chan int, 1000)
    go func() {
        defer close(stream)
        for i := 0; i < 1_000_000; i++ {
            stream <- rand.Intn(1_000_000)
        }
    }()

    result := TopK(stream, 100)

    if len(result) != 100 {
        t.Errorf("got %d, want 100", len(result))
    }

    // Все должны быть близко к максимуму
    sort.Ints(result)
    minTop := result[0]
    if minTop < 900_000 {
        t.Errorf("min of top-100 = %d, expected ≥ 900k", minTop)
    }
}
```

---

## Подводные камни

### 1. `container/heap` запутанный API

```go
heap.Push(h, value)  // ← не h.Push(value)!
heap.Init(h)         // ← обязательно после прямого присваивания
heap.Fix(h, 0)       // ← если изменил элемент через индекс
```

Нужно implement интерфейс с 5 методами (`Len`, `Less`, `Swap`, `Push`, `Pop`). Не интуитивно для новичков.

### 2. Использование max-heap когда нужен min-heap

Для top-K **maximum** values — используется **min-heap** размера K. Если элемент больше минимума — заменить.

```go
// ❌ Max-heap размера K и удаляем minimum снизу — O(N log K) то же,
// но для K большого K элементов в heap → O(K log K) на extract.
// Min-heap из K → top автоматически: max(N values - K смалых).
```

Кажется counter-intuitive, но min-heap правильно для top-K largest.

### 3. Heap.Fix vs Heap.Pop+Push

```go
// Эффективнее: in-place update
(*h)[0] = newValue
heap.Fix(h, 0)

// Менее эффективно: 2 operation вместо 1
heap.Pop(h)
heap.Push(h, newValue)
```

### 4. Result не отсортирован

Top-K через heap даёт K элементов, но **не отсортированных**. Если нужен sorted — sort.Reverse в конце.

### 5. K > N edge case

```go
TopK(stream{1,2,3}, 5)  // ← возвращай все 3 а не error
```

Стандартное поведение — возвращать `min(K, N)` элементов.

### 6. Ties (одинаковые значения)

```go
[1,1,1,2,2]  top-3  →  [2,2,1] or [2,2,1] или [1,1,1]?
```

Стандартно — implementation-defined. Документируй.

### 7. Memory для count map при streaming

```go
counts := make(map[string]int)
for token := range tokenStream {
    counts[token]++  // ← может вырасти до миллионов
}
```

Если уникальных миллиарды (e.g., URL'ы в большом сервисе) — OOM. Решение: Count-Min Sketch (probabilistic) или sampling.

### 8. Concurrent updates

```go
// ❌ Map + heap не thread-safe
go func() { counts["a"]++ }()
go func() { counts["b"]++ }()  // race
```

Mutex или atomic счётчики (sync.Map хороший fit для read-heavy).

---

## Возможные расширения

### 1. Top-K в sliding window

Топ-10 за последний **час**. Window expires + recompute. Сложнее: нужен skip list или специализированная structure.

### 2. Approximate top-K через Count-Min Sketch

[Count-Min Sketch](https://en.wikipedia.org/wiki/Count%E2%80%93min_sketch) — probabilistic counter с O(1) memory (vs O(N) для map). Точно top-K + approximate counts.

Используется в:
- Apache Spark
- Cloudflare (топ ресурсов)
- AWS (real-time analytics)

### 3. Heavy Hitters / SpaceSaving algorithm

Альтернатива CMS — гарантированно K самых частых с bounded error.

### 4. Concurrent top-K

Множество goroutines добавляет в один stream. Решения:
- mutex вокруг heap (low throughput)
- per-shard heap + merge в конце
- atomic counters + periodic top-K refresh

### 5. Distributed top-K

Несколько серверов — каждый локальный top-K, потом merge через coordinator. Aproximate. Применяется в Apache Storm, Flink.

### 6. With expiration / TTL

Каждый count имеет timestamp, expired не считается. Skip list по timestamp.

### 7. K-way merge для streaming sort

Можно использовать heap для merge'а отсортированных streams. Связанная задача — "find K smallest from K sorted lists".

---

## Что важно показать на собеседовании

1. **O(N log K)** для streaming — главное преимущество heap'а
2. **Min-heap для top-K maximum** — counter-intuitive момент, объясни
3. **Зная container/heap API** — Push/Pop/Fix
4. **Trade-off heap vs sort vs quickselect**:
   - Heap: O(N log K), best for streaming
   - Sort: O(N log N), best when K ≈ N
   - Quickselect: O(N) average, no streaming
5. **Count-Min Sketch** mention для extreme N — bonus points
6. **Memory consideration** — O(K) heap vs O(M) counts map
7. **Heap не отсортирован** — частая мелочь, не забудь

## Связки

- [Algorithms: heap / sorting](../../../16-algorithms-and-data-structures/07-sorting-and-heap.md)
- [Sliding window](./05-sliding-window-counter.md) — related — counting в окне
- [Cache analytics](../../../06-databases/caching/01-redis-as-cache.md) — top accessed keys
- [Count-Min Sketch (external)](https://en.wikipedia.org/wiki/Count%E2%80%93min_sketch)
