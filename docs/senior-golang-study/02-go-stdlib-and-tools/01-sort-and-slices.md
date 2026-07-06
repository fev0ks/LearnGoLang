# Sort и Slices: сортировка и бинарный поиск

Сортировка и бинарный поиск — основа множества алгоритмических и продакшн-задач (топ-N, дедупликация, two-pointers, lower/upper bound). В Go два пути: классический пакет `sort` и современный `slices` + `cmp` (Go 1.21+). Знать оба полезно: `slices` — что писать сейчас, `sort` — что встретишь в существующем коде и на собесе.

## Содержание

- [Два пакета: sort и slices](#два-пакета-sort-и-slices)
- [slices: сортировка и поиск (Go 1.21+)](#slices-сортировка-и-поиск-go-121)
- [Пакет cmp](#пакет-cmp)
- [sort: классический API](#sort-классический-api)
- [Сортировка по нескольким ключам](#сортировка-по-нескольким-ключам)
- [Бинарный поиск: lower/upper bound](#бинарный-поиск-lowerupper-bound)
- [Stable vs unstable](#stable-vs-unstable)
- [Подводные камни](#подводные-камни)
- [Частые паттерны в алго-задачах](#частые-паттерны-в-алго-задачах)
- [Interview-ready answer](#interview-ready-answer)

---

## Два пакета: sort и slices

| | `sort` (старый) | `slices` + `cmp` (Go 1.21+) |
|---|---|---|
| Тип API | reflection (`sort.Slice`) или интерфейс | дженерики |
| Скорость | медленнее (reflection) | быстрее, без аллокаций |
| Компаратор | `less func(i,j int) bool` | `func(a,b T) int` (−/0/+) |
| Что использовать | legacy-код | **новый код** |

Оба сортируют **на месте** (in-place), ничего не возвращают (кроме поисковых функций).

> Алгоритм внутри — pattern-defeating quicksort (pdqsort) с Go 1.19: O(n log n), без квадратичных худших случаев на «плохих» входах.

---

## slices: сортировка и поиск (Go 1.21+)

```go
import (
    "cmp"
    "slices"
)

s := []int{3, 1, 2}
slices.Sort(s)                 // [1 2 3] — по возрастанию, для упорядочиваемых типов (cmp.Ordered)

// SortFunc с cmp.Compare(a, b) — РОВНО то же возрастание, что и Sort:
slices.SortFunc(s, func(a, b int) int { return cmp.Compare(a, b) }) // [1 2 3]

// Убывание = поменять местами аргументы (a ↔ b), а не «добавить функцию»:
slices.SortFunc(s, func(a, b int) int { return cmp.Compare(b, a) }) // [3 2 1]

// Или отсортировать по возрастанию и развернуть:
slices.Sort(s); slices.Reverse(s)  // [3 2 1]
```

**Направление задаёт компаратор, а не сам факт функции.** `slices.Sort(s)` — это короткая запись для `SortFunc` с `cmp.Compare(a, b)`. Компаратор возвращает **число**: `<0` если `a` должен идти **раньше** `b`, `0` если равны, `>0` если позже.

- `cmp.Compare(a, b) < 0` ⟺ `a < b` → меньшие раньше → **возрастание**;
- `cmp.Compare(b, a) < 0` ⟺ `a > b` → бóльшие раньше → **убывание**.

Полезный набор из `slices`:

```go
slices.IsSorted(s)                    // проверить отсортированность
slices.Min(s); slices.Max(s)          // экстремумы (паника на пустом)
slices.MinFunc(s, cmp); slices.MaxFunc(s, cmp)
slices.Index(s, v); slices.Contains(s, v)
slices.Compact(s)                     // убрать ПОДРЯД идущие дубли (после Sort = полная дедупликация)
slices.BinarySearch(s, target)        // (index, found) — s должен быть отсортирован
```

---

## Пакет cmp

`cmp` (Go 1.21+) — крошечный пакет-компаньон для `slices`: **один constraint и три функции**. Появился вместе с дженериками сортировки.

### `cmp.Ordered` — constraint для «<-сравнимых» типов

Это интерфейс-ограничение для дженериков: все типы, поддерживающие `< <= >= >` — целые, float, string (и их `~`-производные):

```go
// примерно так он определён:
type Ordered interface {
    ~int | ~int8 | ~int16 | ~int32 | ~int64 |
    ~uint | ~uint8 | ... | ~uintptr |
    ~float32 | ~float64 | ~string
}
```

Его используют `slices.Sort`, `slices.Min/Max`, `slices.BinarySearch` и т.п. — и в собственных дженерик-функциях, которым нужно сравнивать значения:

```go
func clamp[T cmp.Ordered](v, lo, hi T) T {
    if v < lo { return lo }
    if v > hi { return hi }
    return v
}
```

> ⚠️ `cmp.Ordered` ≠ `comparable`. `comparable` — это про `==`/`!=` (есть и у структур, указателей, интерфейсов). `cmp.Ordered` — про порядок `<`, только числа и строки.

### `cmp.Compare(x, y)` — трёхзначное сравнение

Возвращает `-1` / `0` / `+1`. Это **готовый компаратор** для `slices.SortFunc` и multi-key:

```go
cmp.Compare(3, 5)     // -1  (3 < 5)
cmp.Compare(5, 5)     //  0
cmp.Compare("b", "a") // +1

slices.SortFunc(s, func(a, b T) int { return cmp.Compare(a.Key, b.Key) })
```

**Бонус — корректный NaN:** для float `cmp.Compare` детерминирован (NaN считается меньше любого не-NaN, `NaN == NaN`, `-0.0 == 0.0`). Наивное `a < b` с NaN ломает сортировку — поэтому `slices.Sort([]float64)` внутри использует именно эту семантику.

### `cmp.Less(x, y)` — булево `x < y`

То же сравнение, но возвращает `bool` (с той же NaN-семантикой). Нужно, когда требуется булев предикат, а не `-1/0/+1`:

```go
if cmp.Less(a, b) { ... }
// эквивалент a < b, но работает в дженериках и корректно с NaN
```

### `cmp.Or(vals...)` — первое ненулевое (Go 1.22)

Возвращает **первый аргумент, не равный zero value**; если все нулевые — zero value. Два применения:

```go
// 1. Multi-key сортировка: первое НЕнулевое сравнение определяет порядок
slices.SortFunc(people, func(a, b Person) int {
    return cmp.Or(
        cmp.Compare(a.Age, b.Age),   // 0 при равном возрасте → смотрим дальше
        cmp.Compare(a.Name, b.Name),
    )
})

// 2. Coalescing — «значение по умолчанию», как SQL COALESCE / оператор ?? в других языках
port := cmp.Or(cfgPort, envPort, 8080)        // первый ненулевой int
name := cmp.Or(req.Name, user.Name, "anonymous") // первая непустая строка
```

Важно: «нулевое» — это zero value типа (`0`, `""`, `false`, `nil`-указатель). `cmp.Or` принимает `comparable`-типы, не только `Ordered`.

---

## sort: классический API

```go
import "sort"

// 1. Готовые функции для базовых типов
sort.Ints(a)        // []int по возрастанию
sort.Strings(ss)    // []string
sort.Float64s(fs)   // []float64

// 2. sort.Slice — компаратор по индексам (reflection, unstable)
sort.Slice(people, func(i, j int) bool {
    return people[i].Age < people[j].Age   // less: i раньше j?
})

// 3. sort.SliceStable — то же, но сохраняет порядок равных
sort.SliceStable(people, func(i, j int) bool { ... })
```

Компаратор `sort.Slice` — это **`less`** (возвращает `bool`: «`i` должен идти раньше `j`?»), в отличие от трёхзначного компаратора `slices.SortFunc`.

### sort.Interface — для своих коллекций

Полный контроль — реализовать три метода:

```go
type ByAge []Person
func (a ByAge) Len() int           { return len(a) }
func (a ByAge) Less(i, j int) bool { return a[i].Age < a[j].Age }
func (a ByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

sort.Sort(ByAge(people))            // unstable
sort.Stable(ByAge(people))          // stable
sort.Sort(sort.Reverse(ByAge(people))) // развернуть порядок
```

`sort.Reverse(data)` оборачивает `Interface` и инвертирует `Less` — сортировка по убыванию без отдельного компаратора.

---

## Сортировка по нескольким ключам

Самый частый «прод» сценарий: сортировать по полю A, при равенстве — по B.

```go
// slices + cmp.Or (Go 1.22+): возвращает первое НЕнулевое сравнение
slices.SortFunc(people, func(a, b Person) int {
    return cmp.Or(
        cmp.Compare(a.Age, b.Age),      // сначала по возрасту ↑
        cmp.Compare(b.Salary, a.Salary),// при равном возрасте — по зарплате ↓
        cmp.Compare(a.Name, b.Name),    // затем по имени ↑
    )
})
```

`cmp.Or` берёт первый ненулевой результат — читается как список приоритетов. До 1.22 (или на `sort.Slice`) — вручную через каскад `if`:

```go
sort.Slice(people, func(i, j int) bool {
    if people[i].Age != people[j].Age {
        return people[i].Age < people[j].Age
    }
    return people[i].Name < people[j].Name
})
```

---

## Бинарный поиск: lower/upper bound

Бинарный поиск работает **только по отсортированному** слайсу. Два инструмента:

### slices.BinarySearch — найти элемент

```go
a := []int{10, 20, 20, 30}
i, found := slices.BinarySearch(a, 20) // i=1, found=true (первое вхождение)
i, found = slices.BinarySearch(a, 25)  // i=3, found=false (точка вставки)
```

Возвращает позицию **и** флаг наличия. Если нет — `i` это место, куда вставить, сохранив порядок (lower bound).

### sort.Search — обобщённый бинпоиск по предикату

`sort.Search(n, f)` находит **наименьший** `i` в `[0, n)`, где `f(i) == true`. Предикат должен быть монотонным: `false…false true…true`.

```go
// lower bound: первый индекс, где a[i] >= x
lower := sort.Search(len(a), func(i int) bool { return a[i] >= x })

// upper bound: первый индекс, где a[i] > x
upper := sort.Search(len(a), func(i int) bool { return a[i] > x })

// количество элементов == x:  upper - lower
```

Если `f` нигде не `true` — вернёт `n`. Это «швейцарский нож» для задач на отсортированных данных (вставка, подсчёт диапазона, поиск границы).

---

## Stable vs unstable

- **Unstable** (`slices.Sort`, `slices.SortFunc`, `sort.Slice`, `sort.Sort`): порядок **равных** элементов не гарантирован. Быстрее, меньше памяти.
- **Stable** (`slices.SortStableFunc`, `sort.SliceStable`, `sort.Stable`): равные сохраняют исходный относительный порядок.

```go
// Нужна стабильность только когда равные элементы РАЗЛИЧИМЫ по другому признаку
// и этот порядок важен (например, уже отсортировано по дате, до-сортируем по статусу).
slices.SortStableFunc(orders, func(a, b Order) int {
    return cmp.Compare(a.Status, b.Status) // порядок по дате внутри статуса сохранится
})
```

Если сортируешь по **полному** ключу (все поля участвуют) — стабильность не нужна, бери unstable (быстрее).

---

## Подводные камни

1. **Компаратор должен задавать строгий порядок.** `less`/`SortFunc` обязаны быть консистентны: нельзя, чтобы и `a<b`, и `b<a` были true. Иначе результат мусорный, а `slices.SortFunc` может паниковать (`comparison function is not anti-symmetric`). Не пиши `return a.X <= b.X` в трёхзначном компараторе — используй `cmp.Compare`.

2. **`sort.Slice` использует reflection → медленнее.** На горячем пути / больших данных бери `slices.SortFunc` (дженерики, без reflection).

3. **NaN во float.** `NaN` не сравним (`NaN < x` всегда false). `slices.Sort`/`cmp.Compare` обрабатывают детерминированно (NaN считается меньше всего), а наивный `sort.Slice(f, func(i,j){return f[i]<f[j]})` с NaN даст неотсортированный результат. Чисти NaN заранее или используй `slices.Sort`.

4. **Бинпоиск по неотсортированному = мусор.** `BinarySearch`/`sort.Search` молча вернут неверный индекс. Отсортируй сначала.

5. **`slices.Compact` убирает только подряд идущие дубли.** Полная дедупликация = `slices.Sort` + `slices.Compact`.

6. **Сортировка меняет слайс на месте.** Если нужен оригинал — копируй: `c := slices.Clone(s); slices.Sort(c)`.

7. **`slices`/`sort` не принимают массив `[N]T` напрямую** — только слайс. Передавай `arr[:]` (слайс на весь массив, делит backing array → сортирует сам массив на месте):

   ```go
   arr := [3]int{3, 1, 2}
   slices.Sort(arr)      // ❌ cannot use arr ([3]int) as []int
   slices.Sort(arr[:])   // ✅ arr → [1 2 3]
   ```

   Нюанс — **адресуемость**: `arr[:]` требует адресуемый массив. На элементе map не сработает (он не адресуем, как и `&m["k"]`):

   ```go
   m := map[string][3]int{"a": {3, 1, 2}}
   slices.Sort(m["a"][:])   // ❌ cannot slice unaddressable value
   v := m["a"]; slices.Sort(v[:]); m["a"] = v   // достать → сортировать → положить
   ```

---

## Частые паттерны в алго-задачах

```go
// 1. Sort + two pointers (пары с заданной суммой, ближайшая сумма и т.п.)
slices.Sort(nums)
l, r := 0, len(nums)-1
for l < r { /* двигаем указатели */ }

// 2. Дедупликация
slices.Sort(s)
s = slices.Compact(s)

// 3. Топ-K: частичная сортировка не нужна — full sort + срез (или heap для больших n)
slices.SortFunc(items, byScoreDesc)
top := items[:k]

// 4. Сортировка ИНДЕКСОВ, а не значений (когда нужен исходный порядок)
idx := make([]int, len(scores))
for i := range idx { idx[i] = i }
slices.SortFunc(idx, func(a, b int) int { return cmp.Compare(scores[b], scores[a]) })

// 5. Подсчёт вхождений диапазона [lo, hi] в отсортированном
lo := sort.Search(len(a), func(i int) bool { return a[i] >= loVal })
hi := sort.Search(len(a), func(i int) bool { return a[i] > hiVal })
count := hi - lo

// 6. Сортировка по производному ключу через map (Schwartzian-ish)
slices.SortFunc(words, func(a, b string) int {
    return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b)) // по длине, затем лексикографически
})
```

Связанные темы — в [16-algorithms-and-data-structures](../16-algorithms-and-data-structures/README.md) и [12-interview-practice/coding-tasks](../12-interview-practice/coding-tasks/README.md).

---

## Interview-ready answer

**1. sort или slices — что использовать?**

- В новом коде — `slices` + `cmp` (Go 1.21+): дженерики, быстрее, без reflection. `sort.Slice`/`sort.Interface` — legacy и для случаев без 1.21. `sort.Slice` использует reflection и медленнее.

**2. Чем компаратор `slices.SortFunc` отличается от `sort.Slice`?**

- `SortFunc` принимает `func(a, b T) int` (−/0/+, как `cmp.Compare`), `sort.Slice` — `less func(i, j int) bool`. Трёхзначный компаратор удобнее для multi-key (`cmp.Or`).

**3. Как сортировать по нескольким ключам?**

- `cmp.Or(cmp.Compare(a.X, b.X), cmp.Compare(a.Y, b.Y), ...)` — первое ненулевое сравнение wins. До 1.22 — каскад `if` в `less`.

**4. Stable vs unstable — когда что?**

- Stable сохраняет порядок равных элементов. Нужен, только когда сортируешь по **части** ключа и исходный порядок равных важен. Иначе unstable (быстрее).

**5. Как сделать lower/upper bound?**

- `sort.Search(n, f)` — наименьший индекс, где монотонный предикат `true`. `a[i] >= x` → lower bound, `a[i] > x` → upper bound; их разность — число вхождений. Либо `slices.BinarySearch` (возвращает индекс + found).

**6. Главные грабли?**

- Компаратор должен задавать строгий порядок (иначе паника/мусор); бинпоиск требует отсортированного слайса; `NaN` ломает наивную сортировку float; сортировка меняет слайс на месте (копируй, если нужен оригинал).
