# Binary Search

Бинарный поиск — алгоритм поиска в отсортированном пространстве за `O(log n)`.
Работает не только по массиву индексов, но и по пространству ответов.

## Содержание

- [Шаблон](#шаблон)
- [Вариант 1: классический поиск](#вариант-1-классический-поиск)
- [Вариант 2: первая позиция (lower bound)](#вариант-2-первая-позиция-lower-bound)
- [Вариант 3: последняя позиция (upper bound)](#вариант-3-последняя-позиция-upper-bound)
- [Вариант 4: поиск в повёрнутом массиве](#вариант-4-поиск-в-повёрнутом-массиве)
- [Вариант 5: бинарный поиск на ответе](#вариант-5-бинарный-поиск-на-ответе)
- [Вариант 6: целочисленный sqrt(x)](#вариант-6-целочисленный-sqrtx)
- [Типичные ошибки](#типичные-ошибки)
- [Когда применять](#когда-применять)

---

## Шаблон

### Почему `left + (right-left)/2`, а не `(left+right)/2`

Если `left` и `right` — большие `int32` или `int64`, их сумма может переполнить тип:

```
left = 2_000_000_000, right = 2_000_000_001
(left + right) = 4_000_000_001 -> overflow для int32
```

Безопасная формула:

```go
mid := left + (right-left)/2
```

В Go тип `int` — 64-бит на большинстве платформ, но навык писать безопасно важен: он спасает в C/Java и показывает понимание арифметики на интервью.

### Два варианта границ

| Стиль | Инвариант | Условие цикла | Сдвиг right |
|---|---|---|---|
| Inclusive both ends | `[left, right]` | `left <= right` | `right = mid - 1` |
| Half-open | `[left, right)` | `left < right` | `right = mid` |

Inclusive — проще читается и используется по умолчанию ниже.

### Базовый шаблон Go

```go
func binarySearch(nums []int, target int) int {
    left, right := 0, len(nums)-1
    for left <= right {
        mid := left + (right-left)/2
        if nums[mid] == target {
            return mid
        } else if nums[mid] < target {
            left = mid + 1
        } else {
            right = mid - 1
        }
    }
    return -1
}
```

---

## Вариант 1: классический поиск

Найти индекс `target` в отсортированном массиве; вернуть `-1`, если не найден.

```go
// Search returns the index of target in sorted nums, or -1 if not found.
// Time: O(log n), Space: O(1)
func Search(nums []int, target int) int {
    left, right := 0, len(nums)-1

    for left <= right {
        mid := left + (right-left)/2

        switch {
        case nums[mid] == target:
            return mid
        case nums[mid] < target:
            left = mid + 1 // target правее
        default:
            right = mid - 1 // target левее
        }
    }

    return -1
}
```

---

## Вариант 2: первая позиция (lower bound)

Найти наименьший индекс, где `nums[i] >= target`.
Применяется для: первое вхождение, позиция вставки, "сколько элементов меньше target".

```go
// LowerBound returns the smallest index i such that nums[i] >= target.
// If all elements are less than target, returns len(nums).
// Time: O(log n), Space: O(1)
func LowerBound(nums []int, target int) int {
    left, right := 0, len(nums) // right = len, не len-1 — ответ может быть за концом

    for left < right { // строгое неравенство: когда left == right, ответ найден
        mid := left + (right-left)/2

        if nums[mid] < target {
            left = mid + 1 // mid точно не подходит, сдвигаем левую границу
        } else {
            right = mid // mid — кандидат, сужаем правую границу до него
        }
    }

    return left // left == right — позиция первого элемента >= target
}
```

Первое вхождение `target`:

```go
func FirstOccurrence(nums []int, target int) int {
    pos := LowerBound(nums, target)
    if pos < len(nums) && nums[pos] == target {
        return pos
    }
    return -1
}
```

---

## Вариант 3: последняя позиция (upper bound)

Найти наибольший индекс, где `nums[i] == target`; вернуть `-1`, если не найден.

```go
// UpperBound returns the largest index i such that nums[i] == target, or -1.
// Time: O(log n), Space: O(1)
func UpperBound(nums []int, target int) int {
    left, right := 0, len(nums)-1
    result := -1

    for left <= right {
        mid := left + (right-left)/2

        if nums[mid] == target {
            result = mid      // запоминаем кандидата
            left = mid + 1    // продолжаем искать правее
        } else if nums[mid] < target {
            left = mid + 1
        } else {
            right = mid - 1
        }
    }

    return result
}
```

Количество вхождений `target` через lower + upper bound:

```go
func CountOccurrences(nums []int, target int) int {
    first := LowerBound(nums, target)
    if first == len(nums) || nums[first] != target {
        return 0
    }
    last := UpperBound(nums, target)
    return last - first + 1
}
```

---

## Вариант 4: поиск в повёрнутом массиве

Отсортированный массив повёрнут в неизвестной точке: `[4,5,6,7,0,1,2]`.
Найти `target`; вернуть `-1`, если не найден.

Ключевая идея: один из двух отрезков `[left, mid]` или `[mid, right]` всегда отсортирован — определяем какой, сужаем диапазон.

```go
// SearchRotated finds target in a rotated sorted array without duplicates.
// Time: O(log n), Space: O(1)
func SearchRotated(nums []int, target int) int {
    left, right := 0, len(nums)-1

    for left <= right {
        mid := left + (right-left)/2

        if nums[mid] == target {
            return mid
        }

        // Левая половина [left, mid] отсортирована
        if nums[left] <= nums[mid] {
            // target попадает в отсортированную левую часть?
            if nums[left] <= target && target < nums[mid] {
                right = mid - 1 // ищем слева
            } else {
                left = mid + 1 // ищем справа
            }
        } else {
            // Правая половина [mid, right] отсортирована
            // target попадает в отсортированную правую часть?
            if nums[mid] < target && target <= nums[right] {
                left = mid + 1 // ищем справа
            } else {
                right = mid - 1 // ищем слева
            }
        }
    }

    return -1
}
```

---

## Вариант 5: бинарный поиск на ответе

Задача: за `D` дней доставить посылки весами `weights` — найти минимальную грузоподъёмность корабля.

Пространство поиска — не индексы, а значения `[max(weights), sum(weights)]`.
Предикат `canShip(capacity)` монотонен: если `capacity` достаточна, то любая большая тоже достаточна.

```go
// ShipWithinDays returns the minimum ship capacity to deliver all packages within days.
// Time: O(n * log(sum(weights))), Space: O(1)
func ShipWithinDays(weights []int, days int) int {
    // Нижняя граница: корабль должен вмещать хотя бы самый тяжёлый груз.
    // Верхняя граница: берём всё за один день.
    left, right := maxVal(weights), sumVal(weights)

    for left < right {
        mid := left + (right-left)/2

        if canShip(weights, days, mid) {
            right = mid // mid достаточен — ищем меньше
        } else {
            left = mid + 1 // mid недостаточен — нужно больше
        }
    }

    return left
}

// canShip checks whether capacity is enough to ship all weights within days.
func canShip(weights []int, days, capacity int) bool {
    currentLoad, daysNeeded := 0, 1

    for _, w := range weights {
        if currentLoad+w > capacity {
            daysNeeded++ // начинаем новый день
            currentLoad = 0
        }
        currentLoad += w
    }

    return daysNeeded <= days
}

func maxVal(nums []int) int {
    m := nums[0]
    for _, v := range nums[1:] {
        if v > m {
            m = v
        }
    }
    return m
}

func sumVal(nums []int) int {
    s := 0
    for _, v := range nums {
        s += v
    }
    return s
}
```

Паттерн "поиск на ответе" в общем виде:

```go
// BinarySearchOnAnswer находит минимальное значение x в [lo, hi],
// для которого predicate(x) == true (predicate монотонен: false...false, true...true).
func BinarySearchOnAnswer(lo, hi int, predicate func(int) bool) int {
    for lo < hi {
        mid := lo + (hi-lo)/2
        if predicate(mid) {
            hi = mid
        } else {
            lo = mid + 1
        }
    }
    return lo
}
```

---

## Вариант 6: целочисленный sqrt(x)

Найти наибольшее целое `k`, такое что `k*k <= x`.
Пространство поиска: `[0, x]`.

```go
// MySqrt returns the integer part of sqrt(x).
// Time: O(log x), Space: O(1)
func MySqrt(x int) int {
    if x < 2 {
        return x
    }

    left, right := 1, x/2 // sqrt(x) <= x/2 для x >= 4

    for left <= right {
        mid := left + (right-left)/2

        sq := mid * mid
        if sq == x {
            return mid
        } else if sq < x {
            left = mid + 1 // mid подходит, но ищем больше
        } else {
            right = mid - 1 // mid слишком большой
        }
    }

    // right — наибольшее k, при котором k*k <= x
    return right
}
```

Примечание: при больших `x` `mid*mid` может переполнить `int32`. В Go `int` — 64-бит на 64-битных платформах, поэтому для `x <= 2^31-1` переполнения нет. На других языках используют `int64` явно.

---

## Типичные ошибки

### Бесконечный цикл: `left = mid` вместо `left = mid + 1`

```go
// Неправильно: когда left == right == mid, left не продвигается — цикл не завершится
if nums[mid] < target {
    left = mid // BUG
}

// Правильно
if nums[mid] < target {
    left = mid + 1
}
```

Причина: если `left == right`, то `mid == left`. Присвоение `left = mid` не меняет состояние — цикл зависает.

### Выход за пределы: `right = len(nums)` в классическом варианте

```go
// Неправильно: right = len(nums) — обращение к nums[mid] на последнем шаге
// может дать panic: index out of range
left, right := 0, len(nums) // BUG для inclusive-варианта

// Правильно для inclusive [left, right]
left, right := 0, len(nums)-1

// right = len(nums) корректен только в half-open варианте,
// где условие цикла left < right и right никогда не разыменовывается
```

### Переполнение mid

```go
// Небезопасно в языках с фиксированной шириной int
mid := (left + right) / 2 // overflow если left + right > INT_MAX

// Всегда безопасно
mid := left + (right-left)/2
```

---

## Когда применять

| Признак задачи | Применить |
|---|---|
| Массив отсортирован, нужно найти элемент | Классический бинарный поиск `O(log n)` |
| Нужна позиция вставки или первое/последнее вхождение | Lower bound / upper bound |
| Массив отсортирован, но повёрнут | Бинарный поиск с проверкой отсортированной половины |
| Функция результата монотонна (false...true) | Бинарный поиск на ответе |
| Нужно `O(log n)` вместо `O(n)` по числовому диапазону | Бинарный поиск на ответе |
| Задача "минимум/максимум при ограничении" | Бинарный поиск на ответе + предикат-проверка |

Сигналы, что задача решается бинарным поиском на ответе:
- в условии есть "минимальный X при котором ...";
- есть "максимальный X при котором ...";
- можно написать функцию `feasible(x) bool`, которая монотонна.
