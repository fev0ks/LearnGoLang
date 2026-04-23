# Two Pointers And Sliding Window

Два классических паттерна, которые встречаются почти на каждом интервью. Суть обоих — избежать вложенных циклов за счет умного движения по данным.

## Содержание

- [Two Pointers](#two-pointers)
  - [Opposite ends](#opposite-ends)
  - [Fast and slow](#fast-and-slow)
  - [Same direction](#same-direction)
- [Sliding Window](#sliding-window)
  - [Fixed window](#fixed-window)
  - [Variable window](#variable-window)
- [Когда что применять](#когда-что-применять)

---

## Two Pointers

Паттерн: два индекса (или два указателя на узлы), которые движутся по структуре данных — каждый по своим правилам.

Три основных варианта:

- **Opposite ends** — `left` и `right` стартуют с двух концов и сближаются к центру.
- **Fast and slow** — два указателя движутся с разной скоростью.
- **Same direction** — оба указателя движутся вправо, но с разным шагом.

---

### Opposite ends

Признак задачи:
- отсортированный массив или строка;
- нужно найти пару / тройку элементов, удовлетворяющих условию;
- нужно сравнивать элементы с двух сторон.

Общий шаблон:

```go
left, right := 0, len(nums)-1
for left < right {
    // принять решение: двигать left, right, или оба
}
```

#### Two Sum II — Input Array Is Sorted

Дан отсортированный массив и цель. Найти два индекса, сумма элементов по которым равна target.

```go
// TwoSumSorted возвращает 1-based индексы пары, сумма которой равна target.
func TwoSumSorted(numbers []int, target int) [2]int {
    left, right := 0, len(numbers)-1
    for left < right {
        sum := numbers[left] + numbers[right]
        switch {
        case sum == target:
            return [2]int{left + 1, right + 1}
        case sum < target:
            left++
        default:
            right--
        }
    }
    return [2]int{-1, -1}
}
```

// O(n) time, O(1) space

Почему это работает: массив отсортирован, поэтому если сумма мала — нужно увеличить `left`, если велика — уменьшить `right`. Каждый шаг отбрасывает один элемент, итого не более `n` шагов.

#### Container With Most Water

Дан массив высот. Найти два столбца, которые вместе с осью X образуют контейнер максимального объема.

```go
// MaxArea возвращает максимальную площадь контейнера.
func MaxArea(height []int) int {
    left, right := 0, len(height)-1
    best := 0
    for left < right {
        h := min(height[left], height[right])
        area := h * (right - left)
        if area > best {
            best = area
        }
        // двигаем меньший столбец — больший уже не выгоднее оставлять
        if height[left] < height[right] {
            left++
        } else {
            right--
        }
    }
    return best
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

// O(n) time, O(1) space

Ключевая идея: ширина при каждом шаге уменьшается на единицу. Чтобы площадь могла вырасти, нужно попытаться увеличить высоту — поэтому всегда двигаем указатель с меньшим столбцом.

---

### Fast and slow

Признак задачи:
- связный список без явной длины;
- нужно найти середину или обнаружить цикл;
- задача звучит как "найти N-е с конца".

Два указателя: `slow` делает 1 шаг, `fast` — 2 шага за итерацию.

#### Linked List Cycle Detection — алгоритм Флойда

Определить, есть ли цикл в односвязном списке.

```go
type ListNode struct {
    Val  int
    Next *ListNode
}

// HasCycle возвращает true, если в списке есть цикл.
func HasCycle(head *ListNode) bool {
    slow, fast := head, head
    for fast != nil && fast.Next != nil {
        slow = slow.Next
        fast = fast.Next.Next
        if slow == fast {
            return true
        }
    }
    return false
}
```

// O(n) time, O(1) space

Почему работает: если цикл есть, `fast` в итоге догонит `slow` внутри цикла — разрыв уменьшается на 1 за каждую итерацию. Если цикла нет, `fast` упрется в `nil`.

#### Find Middle of Linked List

Найти средний узел односвязного списка. При четной длине вернуть второй из двух средних.

```go
// MiddleNode возвращает средний узел списка.
func MiddleNode(head *ListNode) *ListNode {
    slow, fast := head, head
    for fast != nil && fast.Next != nil {
        slow = slow.Next
        fast = fast.Next.Next
    }
    return slow
}
```

// O(n) time, O(1) space

Когда `fast` достигает конца, `slow` находится ровно посередине — `fast` прошел вдвое больше шагов.

---

### Same direction

Признак задачи:
- нужно убрать или переставить элементы in-place;
- условие сравнивает соседние или несмежные элементы одного массива;
- задача требует "переписать" слайс без доп. памяти.

`read` (fast) — сканирует весь массив, `write` (slow) — позиция следующей записи.

#### Remove Duplicates from Sorted Slice

Удалить дубликаты из отсортированного слайса in-place. Вернуть количество уникальных элементов.

```go
// RemoveDuplicates возвращает длину уникального префикса и модифицирует nums in-place.
func RemoveDuplicates(nums []int) int {
    if len(nums) == 0 {
        return 0
    }
    write := 1
    for read := 1; read < len(nums); read++ {
        if nums[read] != nums[write-1] {
            nums[write] = nums[read]
            write++
        }
    }
    return write
}
```

// O(n) time, O(1) space

`write` всегда указывает на позицию следующего уникального элемента. `read` проходит весь массив; если текущий элемент отличается от последнего записанного, записываем его.

#### Move Zeros to End

Переместить все нули в конец слайса, сохранив порядок ненулевых элементов. In-place.

```go
// MoveZeros переставляет все нули в конец nums.
func MoveZeros(nums []int) {
    write := 0
    for read := 0; read < len(nums); read++ {
        if nums[read] != 0 {
            nums[write] = nums[read]
            write++
        }
    }
    // заполнить оставшиеся позиции нулями
    for write < len(nums) {
        nums[write] = 0
        write++
    }
}
```

// O(n) time, O(1) space

Первый проход собирает все ненулевые элементы подряд начиная с индекса 0. Второй проход проставляет нули хвостом.

---

## Sliding Window

Паттерн: поддерживать "окно" — непрерывный подмассив или подстроку — и двигать его по данным без полного пересчета.

Два варианта:
- **Fixed window** — размер окна задан заранее (`k`), оба края двигаются синхронно.
- **Variable window** — правый край растет свободно, левый сдвигается когда окно становится невалидным.

---

### Fixed window

Признак задачи:
- "максимальная / минимальная сумма подмассива длиной k";
- "среднее по k последним элементам".

#### Maximum Sum Subarray of Size K

Найти максимальную сумму непрерывного подмассива ровно из `k` элементов.

```go
// MaxSumSubarray возвращает максимальную сумму подмассива длиной k.
func MaxSumSubarray(nums []int, k int) int {
    if len(nums) < k {
        return 0
    }
    // сумма первого окна
    windowSum := 0
    for i := 0; i < k; i++ {
        windowSum += nums[i]
    }
    best := windowSum
    // сдвигаем окно: добавляем правый элемент, убираем левый
    for right := k; right < len(nums); right++ {
        windowSum += nums[right] - nums[right-k]
        if windowSum > best {
            best = windowSum
        }
    }
    return best
}
```

// O(n) time, O(1) space

Вместо пересчета суммы с нуля каждый раз вычитаем выбывший элемент и прибавляем вошедший.

---

### Variable window

Признак задачи:
- "найти наименьшее/наибольшее окно, удовлетворяющее условию";
- условие на содержимое окна (уникальность, наличие символов, сумма >= K).

Шаблон:

```go
left := 0
for right := 0; right < len(s); right++ {
    // расширить окно: добавить s[right]
    for /* окно невалидно */ {
        // сузить: убрать s[left], left++
    }
    // обновить ответ
}
```

Левый указатель сдвигается только когда окно нарушает условие — суммарно не более `n` раз за всё время работы.

#### Longest Substring Without Repeating Characters

Найти длину наибольшей подстроки без повторяющихся символов.

```go
// LengthOfLongestSubstring возвращает длину наибольшей подстроки без повторов.
func LengthOfLongestSubstring(s string) int {
    lastSeen := make(map[byte]int) // символ -> последний индекс
    best := 0
    left := 0
    for right := 0; right < len(s); right++ {
        ch := s[right]
        if idx, ok := lastSeen[ch]; ok && idx >= left {
            // символ уже в окне — сдвигаем left вправо за его предыдущую позицию
            left = idx + 1
        }
        lastSeen[ch] = right
        if width := right - left + 1; width > best {
            best = width
        }
    }
    return best
}
```

// O(n) time, O(min(n, alphabet)) space

Карта хранит последнюю позицию каждого символа. При повторе `left` прыгает сразу за прошлое вхождение — не сдвигается по одному шагу, что ускоряет работу.

#### Minimum Window Substring

Дана строка `s` и строка `t`. Найти минимальную подстроку в `s`, содержащую все символы из `t` (с учетом кратности).

```go
// MinWindow возвращает минимальное окно в s, содержащее все символы t.
// Если такого окна нет, возвращает "".
func MinWindow(s, t string) string {
    if len(s) == 0 || len(t) == 0 {
        return ""
    }

    need := make(map[byte]int) // сколько раз каждый символ нужен
    for i := 0; i < len(t); i++ {
        need[t[i]]++
    }

    window := make(map[byte]int) // сколько раз встречается в текущем окне
    have, total := 0, len(need)  // have — символов с нужной кратностью, total — сколько нужно

    left := 0
    bestLen := len(s) + 1
    bestLeft := 0

    for right := 0; right < len(s); right++ {
        ch := s[right]
        window[ch]++
        if cnt, ok := need[ch]; ok && window[ch] == cnt {
            have++
        }

        // пока окно валидно, пробуем его уменьшить
        for have == total {
            if width := right - left + 1; width < bestLen {
                bestLen = width
                bestLeft = left
            }
            lch := s[left]
            window[lch]--
            if cnt, ok := need[lch]; ok && window[lch] < cnt {
                have--
            }
            left++
        }
    }

    if bestLen > len(s) {
        return ""
    }
    return s[bestLeft : bestLeft+bestLen]
}
```

// O(|s| + |t|) time, O(|t|) space

`have` отслеживает, сколько символов из `t` уже покрыты с нужной кратностью. Окно сжимается слева пока оно содержит все нужные символы — это гарантирует минимальность.

---

## Когда что применять

| Паттерн                  | Признак задачи                                              | Типичная сложность     |
|--------------------------|-------------------------------------------------------------|------------------------|
| Opposite ends            | Отсортированный массив, поиск пары/тройки                  | O(n) time, O(1) space  |
| Opposite ends            | Задача на максимизацию площади / длины                     | O(n) time, O(1) space  |
| Fast + slow              | Обнаружение цикла в linked list                            | O(n) time, O(1) space  |
| Fast + slow              | Нахождение середины или N-го с конца                       | O(n) time, O(1) space  |
| Same direction           | Удаление дубликатов / фильтрация in-place                  | O(n) time, O(1) space  |
| Same direction           | Перестановка элементов (zeros, partition)                  | O(n) time, O(1) space  |
| Fixed sliding window     | Агрегат (сумма, среднее) по окну фиксированного размера    | O(n) time, O(1) space  |
| Variable sliding window  | Наибольшая/наименьшая подстрока с условием на содержимое   | O(n) time, O(k) space  |

Общее правило: если видишь задачу на непрерывный подмассив или подстроку — думай сначала про sliding window. Если данные отсортированы и нужна пара — two pointers opposite ends. Если linked list без длины — fast/slow.
