# Сортировки и куча

## Содержание

- [Сортировки: сравнительная таблица](#сортировки-сравнительная-таблица)
- [Реализации в Go](#реализации-в-go)
  - [Сортировка слиянием](#сортировка-слиянием)
  - [Быстрая сортировка](#быстрая-сортировка)
  - [Сортировка подсчётом](#сортировка-подсчётом)
  - [Стандартная библиотека](#стандартная-библиотека)
- [Куча и очередь с приоритетом](#куча-и-очередь-с-приоритетом)
- [Задачи на кучу](#задачи-на-кучу)
- [Когда куча, когда сортировка](#когда-куча-когда-сортировка)
- [Interview-ready answer](#interview-ready-answer)

Сортировка почти никогда не пишется руками в рабочем коде, но её свойства — устойчивость, память, поведение в худшем случае — определяют выбор инструмента. Куча же решает класс задач, где полная сортировка избыточна: достаточно нескольких крайних элементов.

---

## Сортировки: сравнительная таблица

| Алгоритм       | Лучший     | Средний    | Худший     | Память  | Устойчива | Когда применять                                        |
|----------------|------------|------------|------------|---------|--------|-----------------------------------------------------------|
| Bubble Sort    | O(n)       | O(n²)      | O(n²)      | O(1)    | да     | учебные цели, почти отсортированные данные                |
| Selection Sort | O(n²)      | O(n²)      | O(n²)      | O(1)    | нет    | минимизация числа свапов, маленькие массивы               |
| Insertion Sort | O(n)       | O(n²)      | O(n²)      | O(1)    | да     | почти отсортированный ввод, онлайн-сортировка             |
| Merge Sort     | O(n log n) | O(n log n) | O(n log n) | O(n)    | да     | связные списки, внешняя сортировка, гарантия worst-case   |
| Quick Sort     | O(n log n) | O(n log n) | O(n²)      | O(log n)| нет    | in-place, кэш-дружелюбен, лучший практический выбор      |
| Heap Sort      | O(n log n) | O(n log n) | O(n log n) | O(1)    | нет    | in-place с гарантией worst-case, нет рекурсии             |
| Counting Sort  | O(n+k)     | O(n+k)     | O(n+k)     | O(k)    | да     | целые числа в небольшом диапазоне k                       |
| Radix Sort     | O(nk)      | O(nk)      | O(nk)      | O(n+k)  | да     | строки, числа фиксированной длины, k — число разрядов     |

Пояснение к колонкам: память — дополнительная, помимо входного массива; устойчивость — сохраняется ли относительный порядок равных элементов. Устойчивость важна, когда данные сортируются по нескольким ключам последовательно: неустойчивый алгоритм разрушит результат предыдущей сортировки.

---

## Реализации в Go

### Сортировка слиянием

Сортировка слиянием (merge sort) — рекурсивный алгоритм: массив делится пополам, каждая половина сортируется отдельно, затем отсортированные половины сливаются. Время — всегда O(n log n), дополнительная память O(n) на временный слайс при слиянии.

Почему стабилен: при слиянии двух половин, если элементы равны, мы всегда берём сначала элемент из левой половины. Это сохраняет исходный порядок.

Почему хорош для связных списков: не требует случайного доступа, слияние двух отсортированных списков — O(n) без дополнительной памяти.

Почему хорош для внешней сортировки: данные не помещаются в RAM, чтение происходит блоками. Каждый блок сортируется отдельно, затем блоки сливаются — именно паттерн Merge Sort.

```go
func mergeSort(arr []int) []int {
    if len(arr) <= 1 {
        return arr
    }

    mid := len(arr) / 2
    left := mergeSort(arr[:mid])
    right := mergeSort(arr[mid:])

    return merge(left, right)
}

func merge(left, right []int) []int {
    result := make([]int, 0, len(left)+len(right))
    i, j := 0, 0

    for i < len(left) && j < len(right) {
        // <= обеспечивает стабильность: левый идёт первым при равенстве
        if left[i] <= right[j] {
            result = append(result, left[i])
            i++
        } else {
            result = append(result, right[j])
            j++
        }
    }

    result = append(result, left[i:]...)
    result = append(result, right[j:]...)
    return result
}
```

---

### Быстрая сортировка

Quick Sort выбирает опорный элемент (pivot), переставляет элементы так, чтобы все меньшие оказались слева, все большие — справа, затем рекурсивно сортирует обе части.

Worst case O(n²) возникает, когда pivot каждый раз оказывается минимальным или максимальным элементом — тогда одна часть имеет n-1 элементов, другая — 0. Это случается, например, при сортировке уже отсортированного массива с фиксированным pivot = первый элемент.

Как избежать: выбирать pivot случайно или использовать median-of-three (медиана первого, среднего и последнего элемента). На практике Go runtime использует pdqsort — гибрид Quick Sort, Heap Sort и Insertion Sort.

```go
import "math/rand"

func quickSort(arr []int, lo, hi int) {
    if lo >= hi {
        return
    }

    pivotIdx := partition(arr, lo, hi)
    quickSort(arr, lo, pivotIdx-1)
    quickSort(arr, pivotIdx+1, hi)
}

func partition(arr []int, lo, hi int) int {
    // случайный pivot снижает вероятность worst case до O(1/n!)
    randIdx := lo + rand.Intn(hi-lo+1)
    arr[randIdx], arr[hi] = arr[hi], arr[randIdx]

    pivot := arr[hi]
    i := lo - 1

    for j := lo; j < hi; j++ {
        if arr[j] <= pivot {
            i++
            arr[i], arr[j] = arr[j], arr[i]
        }
    }

    arr[i+1], arr[hi] = arr[hi], arr[i+1]
    return i + 1
}

// Использование:
// quickSort(arr, 0, len(arr)-1)
```

---

### Сортировка подсчётом

Counting Sort применим только к целым числам (или данным, отображаемым в целые числа) в заранее известном диапазоне [min, max]. Сложность O(n+k), где k = max - min + 1. Если k >> n, алгоритм неэффективен по памяти.

Типичный сценарий: сортировка оценок (0–100), возрастов, символов ASCII.

```go
func countingSort(arr []int) []int {
    if len(arr) == 0 {
        return arr
    }

    // находим диапазон
    minVal, maxVal := arr[0], arr[0]
    for _, v := range arr {
        if v < minVal {
            minVal = v
        }
        if v > maxVal {
            maxVal = v
        }
    }

    k := maxVal - minVal + 1
    count := make([]int, k)

    // считаем вхождения
    for _, v := range arr {
        count[v-minVal]++
    }

    // строим результат
    result := make([]int, 0, len(arr))
    for i, c := range count {
        for j := 0; j < c; j++ {
            result = append(result, i+minVal)
        }
    }

    return result
}
```

---

### Стандартная библиотека

Go предоставляет два основных способа сортировки:

`sort.Slice` — классический вариант, работает с любым слайсом через замыкание-компаратор:

```go
import "sort"

people := []struct {
    Name string
    Age  int
}{
    {"Alice", 30},
    {"Bob", 25},
    {"Charlie", 30},
}

// сортировка по возрасту, при равном возрасте — по имени
sort.Slice(people, func(i, j int) bool {
    if people[i].Age != people[j].Age {
        return people[i].Age < people[j].Age
    }
    return people[i].Name < people[j].Name
})
```

`slices.Sort` и `slices.SortFunc` — дженерик-вариант из Go 1.21, типобезопасен:

```go
import (
    "cmp"
    "slices"
)

nums := []int{5, 2, 8, 1, 9}
slices.Sort(nums) // для типов с порядком

// кастомный компаратор
slices.SortFunc(people, func(a, b struct{ Name string; Age int }) int {
    return cmp.Compare(a.Age, b.Age)
})
```

Разница между ними не только в типобезопасности. `sort.Slice` переставляет элементы через рефлексию, а `slices.Sort` специализируется под конкретный тип на этапе компиляции. Замер на срезе из 100 000 случайных `int`, Go 1.26:

```text
sort.Slice     8.63 мс/операция
slices.Sort    4.67 мс/операция   ← почти вдвое быстрее
```

Поэтому для новых мест по умолчанию берётся `slices.Sort`, а `sort.Slice` остаётся там, где сортируется не срез или где так уже написано.

Когда предпочесть стандартную библиотеку вместо ручной реализации:

- почти всегда: Go использует pdqsort — практически оптимальный алгоритм;
- исключения: нужна стабильная сортировка → `sort.SliceStable` или `slices.SortStableFunc`;
- исключения: сортировка связного списка — стандартная библиотека не поддерживает;
- исключения: внешняя сортировка или специфический алгоритм под данные (counting, radix).

---

## Куча и очередь с приоритетом

Куча (heap) — полное бинарное дерево, хранимое в массиве, с инвариантом:

- min-heap: каждый узел меньше или равен своим потомкам. Корень — минимум.
- max-heap: каждый узел больше или равен своим потомкам. Корень — максимум.

Операции: вставка O(log n), извлечение минимума/максимума O(log n), peek O(1).

В Go стандартная библиотека `container/heap` предоставляет интерфейс, который нужно реализовать:

```go
import "container/heap"

type MinHeap []int

func (h MinHeap) Len() int           { return len(h) }
func (h MinHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h MinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MinHeap) Push(x any) {
    *h = append(*h, x.(int))
}

func (h *MinHeap) Pop() any {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[:n-1]
    return x
}
```

Для max-heap достаточно инвертировать Less:

```go
func (h MaxHeap) Less(i, j int) bool { return h[i] > h[j] }
```

Базовое использование:

```go
h := &MinHeap{5, 2, 8}
heap.Init(h)
heap.Push(h, 1)
min := heap.Pop(h).(int) // 1
```

---

## Задачи на кучу

### 1. K-й по величине элемент

Найти k-й по величине элемент в несортированном массиве.

Подход: min-heap размером k. Для каждого нового элемента: если он больше вершины кучи — выталкиваем вершину и добавляем новый элемент. В конце вершина кучи — k-й наибольший.

Сложность: O(n log k) по времени, O(k) по памяти. Лучше O(n log n) при k << n.

```go
import "container/heap"

func findKthLargest(nums []int, k int) int {
    h := &MinHeap{}
    heap.Init(h)

    for _, num := range nums {
        heap.Push(h, num)
        if h.Len() > k {
            heap.Pop(h)
        }
    }

    return (*h)[0]
}
```

---

### 2. K самых частых элементов

Найти k наиболее часто встречающихся элементов.

Подход: сначала строим карту частот за O(n), затем используем min-heap по частоте размером k.

```go
import "container/heap"

type freqItem struct {
    val, freq int
}

type FreqHeap []freqItem

func (h FreqHeap) Len() int           { return len(h) }
func (h FreqHeap) Less(i, j int) bool { return h[i].freq < h[j].freq }
func (h FreqHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *FreqHeap) Push(x any)        { *h = append(*h, x.(freqItem)) }
func (h *FreqHeap) Pop() any {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[:n-1]
    return x
}

func topKFrequent(nums []int, k int) []int {
    freq := make(map[int]int)
    for _, n := range nums {
        freq[n]++
    }

    h := &FreqHeap{}
    heap.Init(h)

    for val, f := range freq {
        heap.Push(h, freqItem{val, f})
        if h.Len() > k {
            heap.Pop(h)
        }
    }

    result := make([]int, k)
    for i := k - 1; i >= 0; i-- {
        result[i] = heap.Pop(h).(freqItem).val
    }
    return result
}
```

---

### 3. Слияние K отсортированных списков

Слияние k отсортированных массивов в один отсортированный массив.

Подход: помещаем в min-heap первый элемент каждого списка вместе с индексами (значение, индекс_списка, индекс_элемента). Каждый раз извлекаем минимум, добавляем в результат и вставляем следующий элемент из того же списка. Сложность O(n log k), где n — общее число элементов.

```go
import "container/heap"

type listItem struct {
    val, listIdx, elemIdx int
}

type ListHeap []listItem

func (h ListHeap) Len() int           { return len(h) }
func (h ListHeap) Less(i, j int) bool { return h[i].val < h[j].val }
func (h ListHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *ListHeap) Push(x any)        { *h = append(*h, x.(listItem)) }
func (h *ListHeap) Pop() any {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[:n-1]
    return x
}

func mergeKSortedLists(lists [][]int) []int {
    h := &ListHeap{}
    heap.Init(h)

    // инициализируем кучу первыми элементами каждого списка
    for i, list := range lists {
        if len(list) > 0 {
            heap.Push(h, listItem{list[0], i, 0})
        }
    }

    var result []int
    for h.Len() > 0 {
        item := heap.Pop(h).(listItem)
        result = append(result, item.val)

        // добавляем следующий элемент из того же списка
        nextIdx := item.elemIdx + 1
        if nextIdx < len(lists[item.listIdx]) {
            heap.Push(h, listItem{
                val:     lists[item.listIdx][nextIdx],
                listIdx: item.listIdx,
                elemIdx: nextIdx,
            })
        }
    }

    return result
}
```

---

### 4. Медиана потока

Нахождение медианы в потоке данных после каждого добавления.

Идея: делим все числа на две равные половины. Левая половина — max-heap (хранит меньшие числа, быстро отдаёт максимум левой части). Правая половина — min-heap (хранит большие числа, быстро отдаёт минимум правой части).

Инвариант: размеры куч отличаются не более чем на 1.

- Если суммарное количество элементов нечётное — медиана это вершина большей кучи.
- Если чётное — среднее арифметическое двух вершин.

Сложность: добавление O(log n), получение медианы O(1).

```go
import "container/heap"

// MaxHeap для левой половины (меньших чисел)
type MaxHeap []int

func (h MaxHeap) Len() int           { return len(h) }
func (h MaxHeap) Less(i, j int) bool { return h[i] > h[j] } // инвертировано
func (h MaxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *MaxHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *MaxHeap) Pop() any {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[:n-1]
    return x
}

type MedianFinder struct {
    lower *MaxHeap // левая половина
    upper *MinHeap // правая половина
}

func newMedianFinder() *MedianFinder {
    lo, up := &MaxHeap{}, &MinHeap{}
    heap.Init(lo)
    heap.Init(up)
    return &MedianFinder{lo, up}
}

func (mf *MedianFinder) addNum(num int) {
    // добавляем в левую кучу
    heap.Push(mf.lower, num)

    // балансируем: максимум левой должен быть <= минимуму правой
    if mf.upper.Len() > 0 && (*mf.lower)[0] > (*mf.upper)[0] {
        heap.Push(mf.upper, heap.Pop(mf.lower))
    }

    // выравниваем размеры: левая может быть на 1 больше правой
    if mf.lower.Len() > mf.upper.Len()+1 {
        heap.Push(mf.upper, heap.Pop(mf.lower))
    } else if mf.upper.Len() > mf.lower.Len() {
        heap.Push(mf.lower, heap.Pop(mf.upper))
    }
}

func (mf *MedianFinder) findMedian() float64 {
    if mf.lower.Len() > mf.upper.Len() {
        return float64((*mf.lower)[0])
    }
    return float64((*mf.lower)[0]+(*mf.upper)[0]) / 2.0
}
```

---

## Когда куча, когда сортировка

**Полная сортировка** уместна, если:

- все данные доступны заранее;
- нужен полностью отсортированный результат;
- сложность O(n log n), память O(1) (in-place) или O(n) (merge sort).

**Куча** уместна, если:

- данные приходят потоком, нельзя ждать конца ввода;
- нужен только top-K — незачем сортировать всё, достаточно поддерживать кучу размером k: O(n log k);
- нужна медиана потока, k ближайших точек или аналогичная задача с динамическим ранжированием;
- нужен быстрый доступ к минимуму/максимуму при частых вставках и удалениях.

Практическое правило распознавания: полная сортировка, после которой берутся только первые k элементов, — это сигнал, что задача решается кучей за O(n log k) вместо O(n log n).

---

## Interview-ready answer

**1. Какая сортировка используется в Go и почему именно она?**

- `slices.Sort` и `sort.Slice` применяют pdqsort — гибрид быстрой сортировки, сортировки кучей и вставками.
- Быстрая сортировка даёт лучшую практическую скорость за счёт локальности данных, но на неудачных входных данных вырождается в O(n²).
- Pdqsort распознаёт такое вырождение и переключается на сортировку кучей, что даёт гарантию O(n log n) в худшем случае.
- Ни один из этих вариантов не устойчив: для устойчивой сортировки нужны `slices.SortStableFunc` или `sort.SliceStable`.

**2. Что такое устойчивость сортировки и когда она важна?**

- Устойчивая сортировка сохраняет относительный порядок элементов с равными ключами.
- Это важно при последовательной сортировке по нескольким ключам: сначала по имени, затем по возрасту — и внутри одного возраста имена остаются упорядоченными.
- Неустойчивый алгоритм в этом сценарии разрушит результат первой сортировки.
- Альтернатива устойчивости — один компаратор, сравнивающий сразу все ключи по очереди.

**3. Почему для top-K не нужно сортировать весь массив?**

- Полная сортировка стоит O(n log n), а достаточно поддерживать кучу размером k и получить O(n log k).
- При k много меньше n разница принципиальна: для миллиона элементов и k равном десяти логарифм падает с двадцати до трёх.
- Для K наибольших используется min-куча: её вершина — наименьший из отобранных, и любой элемент больше неё вытесняет вершину.
- Для K наименьших всё зеркально — max-куча, и в Go она получается инверсией знака в методе `Less`.

**4. Как устроен `container/heap` в Go?**

- Это не структура данных, а набор алгоритмов поверх интерфейса: реализовать нужно `Len`, `Less`, `Swap`, `Push` и `Pop`.
- `Push` и `Pop` в интерфейсе работают с концом среза, а балансировку выполняют функции пакета `heap.Push` и `heap.Pop` — вызывать методы напрямую нельзя.
- Стоимость: вставка и извлечение O(log n), доступ к вершине O(1), построение кучи из готового среза `heap.Init` — O(n).
- Направление кучи задаётся только методом `Less`, поэтому max-куча получается сменой знака сравнения.

**5. Как найти медиану потока чисел?**

- Двумя кучами: max-куча хранит меньшую половину, min-куча — большую.
- После каждой вставки размеры выравниваются так, чтобы отличались не больше чем на единицу.
- Медиана — вершина большей кучи либо среднее двух вершин при равных размерах, и берётся она за O(1).
- Вставка стоит O(log n), поэтому подход выдерживает поток, тогда как пересортировка на каждом элементе не выдержала бы.
