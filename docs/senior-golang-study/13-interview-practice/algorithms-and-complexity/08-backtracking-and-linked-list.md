# Backtracking и Linked List

Два классических раздела алгоритмических интервью. Backtracking — систематический перебор с возвратом,
используется для задач на перестановки, подмножества, поиск путей. Linked List — набор техник работы
со связными списками: указатели, dummy-голова, обнаружение цикла.

---

## Backtracking

### Идея

Backtracking — это рекурсивный обход **дерева состояний** (state space tree). На каждом узле дерева
мы делаем выбор из доступных вариантов, уходим вглубь, а после возврата из рекурсии отменяем выбор
(undo). Таким образом каждая ветка дерева соответствует одному пути от корня до листа.

Ключевой приём — **pruning** (отсечение): если текущее состояние уже не может привести к корректному
решению, мы обрываем ветку досрочно, не спускаясь глубже. Это превращает полный перебор 2^N или N!
в практически приемлемое время для небольших N.

### Универсальный шаблон

```go
func backtrack(path []int, choices []int, result *[][]int) {
    if /* base case — путь полный */ {
        tmp := make([]int, len(path))
        copy(tmp, path)
        *result = append(*result, tmp)
        return
    }
    for _, choice := range choices {
        // 1. сделать выбор
        path = append(path, choice)
        // 2. уйти глубже с уменьшенным пространством выборов
        backtrack(path, /* reduced choices */, result)
        // 3. отменить выбор (undo)
        path = path[:len(path)-1]
    }
}
```

Три шага — **make choice / recurse / undo choice** — повторяются в каждой задаче на backtracking.
`path` — текущий путь от корня до текущего узла. `result` передаётся по указателю, чтобы не копировать
накопленные решения на каждом уровне рекурсии.

Важно: при сохранении `path` в результат всегда делать `copy`, иначе все элементы `result` будут
указывать на один и тот же backing array слайса.

---

### Задача 1. Permutations — все перестановки

Дан массив уникальных целых чисел. Вернуть все возможные перестановки.

```go
func permute(nums []int) [][]int {
    result := [][]int{}
    used := make([]bool, len(nums))

    var backtrack func(path []int)
    backtrack = func(path []int) {
        if len(path) == len(nums) {
            tmp := make([]int, len(path))
            copy(tmp, path)
            result = append(result, tmp)
            return
        }
        for i, num := range nums {
            if used[i] {
                continue
            }
            used[i] = true
            path = append(path, num)
            backtrack(path)
            path = path[:len(path)-1]
            used[i] = false
        }
    }

    backtrack([]int{})
    return result
}
```

Слайс `used` длиной N отслеживает, какой элемент уже стоит в текущем `path`. На каждом уровне
рекурсии перебираем все N элементов, пропуская уже использованные — это и формирует дерево
перестановок. Отмена выбора включает как усечение `path`, так и сброс `used[i] = false`.

**Сложность:** O(N! * N) по времени (N! перестановок, каждую копируем за O(N)); O(N) доп. памяти
на стек рекурсии и `used`.

---

### Задача 2. Subsets (Power Set) — все подмножества

Дан массив уникальных целых чисел. Вернуть все подмножества (включая пустое и само множество).

```go
func subsets(nums []int) [][]int {
    result := [][]int{}

    var backtrack func(start int, path []int)
    backtrack = func(start int, path []int) {
        // добавляем текущий path на каждом уровне, не только на листьях
        tmp := make([]int, len(path))
        copy(tmp, path)
        result = append(result, tmp)

        for i := start; i < len(nums); i++ {
            path = append(path, nums[i])
            backtrack(i+1, path)
            path = path[:len(path)-1]
        }
    }

    backtrack(0, []int{})
    return result
}
```

Параметр `start` гарантирует, что мы берём только элементы правее текущего — это исключает дубликаты
подмножеств. В отличие от перестановок, результат пишется не только на листьях, а на каждом узле
дерева, потому что каждый префикс пути является допустимым подмножеством.

**Сложность:** O(2^N * N) — 2^N подмножеств, каждое копируется за O(N).

---

### Задача 3. Combination Sum — сумма с повторным использованием

Дан массив уникальных натуральных чисел `candidates` и целевая сумма `target`. Найти все комбинации,
чьи элементы в сумме дают `target`. Один и тот же элемент можно использовать несколько раз.

```go
func combinationSum(candidates []int, target int) [][]int {
    sort.Ints(candidates) // сортировка для раннего выхода
    result := [][]int{}

    var backtrack func(start, remaining int, path []int)
    backtrack = func(start, remaining int, path []int) {
        if remaining == 0 {
            tmp := make([]int, len(path))
            copy(tmp, path)
            result = append(result, tmp)
            return
        }
        for i := start; i < len(candidates); i++ {
            if candidates[i] > remaining {
                break // pruning: отсортированный массив — дальше только больше
            }
            path = append(path, candidates[i])
            backtrack(i, remaining-candidates[i], path) // i, не i+1: элемент можно повторять
            path = path[:len(path)-1]
        }
    }

    backtrack(0, target, []int{})
    return result
}
```

Два ключевых отличия от `subsets`: рекурсия уходит с тем же `i` (не `i+1`), позволяя повторное
использование элемента; `break` вместо `continue` — после сортировки все последующие кандидаты
тоже превысят `remaining`, продолжать бессмысленно.

**Сложность:** O(N^(T/M)) в среднем, где T — target, M — минимальный кандидат. В худшем случае
экспоненциальная, pruning существенно сокращает реальное время.

---

### Задача 4. Word Search в сетке

Дана 2D-сетка символов и строка `word`. Определить, существует ли путь в сетке, спеллующий `word`.
Из каждой клетки можно двигаться в 4 стороны, одну клетку нельзя использовать дважды в одном пути.

```go
func exist(board [][]byte, word string) bool {
    rows, cols := len(board), len(board[0])
    dirs := [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}

    var dfs func(r, c, idx int) bool
    dfs = func(r, c, idx int) bool {
        if idx == len(word) {
            return true
        }
        if r < 0 || r >= rows || c < 0 || c >= cols {
            return false
        }
        if board[r][c] != word[idx] {
            return false
        }

        // пометить клетку как посещённую — in-place modification
        tmp := board[r][c]
        board[r][c] = '#'

        for _, d := range dirs {
            if dfs(r+d[0], c+d[1], idx+1) {
                board[r][c] = tmp // восстановить перед возвратом
                return true
            }
        }

        // undo: восстановить клетку
        board[r][c] = tmp
        return false
    }

    for r := 0; r < rows; r++ {
        for c := 0; c < cols; c++ {
            if dfs(r, c, 0) {
                return true
            }
        }
    }
    return false
}
```

Посещённость отмечается прямо в `board` символом `'#'` — это экономит O(M*N) памяти на отдельный
`visited`. Обязательно восстанавливать `board[r][c] = tmp` при возврате — иначе следующие пути
от других стартовых клеток получат испорченную сетку.

**Сложность:** O(M * N * 4^L), где L — длина слова; в каждой клетке до 4 направлений на L уровней.

---

### Задача 5. N-Queens

Расставить N ферзей на доске N x N так, чтобы ни один не бил другого. Вернуть количество решений.

```go
func totalNQueens(n int) int {
    count := 0
    // используем три множества для O(1) проверки атак
    cols := make([]bool, n)
    diag1 := make([]bool, 2*n-1) // главные диагонали: r-c+n-1
    diag2 := make([]bool, 2*n-1) // побочные диагонали: r+c

    var backtrack func(row int)
    backtrack = func(row int) {
        if row == n {
            count++
            return
        }
        for col := 0; col < n; col++ {
            d1 := row - col + n - 1
            d2 := row + col
            if cols[col] || diag1[d1] || diag2[d2] {
                continue // клетка под атакой — pruning
            }
            // разместить ферзя
            cols[col] = true
            diag1[d1] = true
            diag2[d2] = true

            backtrack(row + 1)

            // убрать ферзя
            cols[col] = false
            diag1[d1] = false
            diag2[d2] = false
        }
    }

    backtrack(0)
    return count
}
```

Ферзи расставляются по одному на строку (row), поэтому атаку по строке проверять не нужно.
Три булевых массива кодируют занятость столбца и двух диагоналей за O(1) без итерации по доске.
Формула `r - c + n - 1` отображает главную диагональ (r - c = const) в неотрицательный индекс;
`r + c` — для побочной.

**Сложность:** O(N!) в худшем случае, на практике значительно меньше благодаря pruning. Пространство O(N).

---

### Pruning: ключевые приёмы

**1. Сортировка + ранний break**

Если массив отсортирован и нас интересуют суммы, как только текущий элемент превышает остаток —
все последующие тоже превысят. Используем `break`, не `continue`.

```go
sort.Ints(nums)
for i := start; i < len(nums); i++ {
    if nums[i] > remaining {
        break
    }
    // ...
}
```

**2. Пропуск дубликатов при отсортированном входе**

Если входной массив содержит повторяющиеся элементы и нужны уникальные комбинации, пропускаем
элемент, если он совпадает с предыдущим на том же уровне рекурсии:

```go
sort.Ints(nums)
for i := start; i < len(nums); i++ {
    if i > start && nums[i] == nums[i-1] {
        continue // пропускаем дубликат на этом уровне
    }
    // ...
}
```

Условие `i > start` (а не `i > 0`) важно: мы разрешаем использовать одно и то же значение
на разных уровнях рекурсии, запрещаем лишь второй выбор одного значения на том же уровне.

---

## Linked List

### Определение узла

```go
type ListNode struct {
    Val  int
    Next *ListNode
}
```

Все задачи ниже используют эту структуру.

---

### Техники

**Dummy head node**

Добавление фиктивного узла перед головой списка убирает особые случаи при удалении или вставке
у начала: код для первого узла становится таким же, как для всех остальных.

```go
dummy := &ListNode{Next: head}
curr := dummy
// ... операции ...
return dummy.Next
```

**Two pointers on list**

Два указателя с разными скоростями или смещением решают целый класс задач:
- fast (шаг 2) + slow (шаг 1) → нахождение середины списка
- fast на N шагов вперёд + slow → N-й с конца
- fast + slow одновременно → обнаружение цикла (Floyd's algorithm)

---

### Задача 1. Reverse Linked List — развернуть список

Дан односвязный список. Вернуть его развёрнутую версию.

```go
// Итеративный вариант — O(N) время, O(1) память
func reverseList(head *ListNode) *ListNode {
    var prev *ListNode
    curr := head
    for curr != nil {
        next := curr.Next // сохранить следующий перед перезаписью
        curr.Next = prev  // развернуть указатель
        prev = curr
        curr = next
    }
    return prev // prev — новая голова
}

// Рекурсивный вариант — O(N) время, O(N) стек
func reverseListRec(head *ListNode) *ListNode {
    if head == nil || head.Next == nil {
        return head
    }
    newHead := reverseListRec(head.Next)
    head.Next.Next = head // узел, стоявший за head, теперь смотрит на head
    head.Next = nil       // head становится хвостом
    return newHead
}
```

Итеративный вариант предпочтителен на интервью: нет риска переполнения стека для длинных списков.
Три переменных `prev / curr / next` — достаточное состояние для разворота одного шага.

**Сложность:** O(N) время, O(1) доп. память (итеративный).

---

### Задача 2. Find Middle — найти середину списка

Дан односвязный список. Вернуть узел, являющийся серединой. При чётном числе узлов вернуть
второй из двух средних (LeetCode 876 по умолчанию).

```go
func middleNode(head *ListNode) *ListNode {
    slow, fast := head, head
    for fast != nil && fast.Next != nil {
        slow = slow.Next
        fast = fast.Next.Next
    }
    return slow
}
```

Инвариант: когда `fast` достигает конца (nil или последний узел), `slow` находится ровно
посередине. При нечётном N (например, 5 узлов) fast дойдёт до последнего узла, slow будет
на 3-м. При чётном N (например, 4 узла) fast станет nil, slow будет на 3-м из 4 — второй средний.

**Сложность:** O(N) время, O(1) память.

---

### Задача 3. Detect and Find Cycle Start — цикл Флойда

Определить, есть ли цикл в списке. Если есть — вернуть узел, с которого цикл начинается.

```go
func detectCycle(head *ListNode) *ListNode {
    slow, fast := head, head

    // Фаза 1: обнаружение цикла
    for fast != nil && fast.Next != nil {
        slow = slow.Next
        fast = fast.Next.Next
        if slow == fast {
            // Фаза 2: поиск входа в цикл
            // Один указатель — с начала списка, второй — с точки встречи,
            // оба движутся по одному шагу; встретятся на входе в цикл.
            entry := head
            for entry != slow {
                entry = entry.Next
                slow = slow.Next
            }
            return entry
        }
    }
    return nil // цикла нет
}
```

Математическое обоснование фазы 2: пусть расстояние от head до входа в цикл — F,
от входа до точки встречи — a, длина цикла — C. В момент встречи slow прошёл F + a,
fast — F + a + k*C (k >= 1 полных оборотов). Так как fast = 2 * slow:
2(F + a) = F + a + k*C → F = k*C - a. Указатель с начала пройдёт F шагов,
slow внутри цикла также пройдёт F = k*C - a шагов и окажется ровно у входа.

**Сложность:** O(N) время, O(1) память.

---

### Задача 4. Merge Two Sorted Lists — слияние отсортированных списков

Даны два отсортированных связных списка. Вернуть слитый отсортированный список.

```go
func mergeTwoLists(list1 *ListNode, list2 *ListNode) *ListNode {
    dummy := &ListNode{}
    curr := dummy

    for list1 != nil && list2 != nil {
        if list1.Val <= list2.Val {
            curr.Next = list1
            list1 = list1.Next
        } else {
            curr.Next = list2
            list2 = list2.Next
        }
        curr = curr.Next
    }

    // Один из списков закончился — присоединить остаток другого
    if list1 != nil {
        curr.Next = list1
    } else {
        curr.Next = list2
    }

    return dummy.Next
}
```

Dummy head убирает необходимость обрабатывать "первый узел результата" отдельно. `curr` всегда
указывает на последний добавленный узел, и мы просто выбираем меньший из текущих голов двух списков.
Финальная проверка обрабатывает хвост непустого списка одним присваиванием.

**Сложность:** O(N + M) время, O(1) доп. память (без учёта выходного списка).

---

### Задача 5. Remove Nth Node From End — удалить N-й с конца

Дан связный список и число N. Удалить N-й узел с конца списка и вернуть голову.

```go
func removeNthFromEnd(head *ListNode, n int) *ListNode {
    dummy := &ListNode{Next: head}
    fast, slow := dummy, dummy

    // fast уходит вперёд на n+1 шагов (не n, потому что slow должен
    // остановиться перед удаляемым узлом, а не на нём)
    for i := 0; i <= n; i++ {
        fast = fast.Next
    }

    // Двигать оба, пока fast не достигнет конца
    for fast != nil {
        fast = fast.Next
        slow = slow.Next
    }

    // slow стоит перед N-м с конца — удалить следующий узел
    slow.Next = slow.Next.Next
    return dummy.Next
}
```

Расстояние между `fast` и `slow` поддерживается равным N+1. Когда `fast` становится nil (прошёл
весь список), `slow` находится ровно перед удаляемым узлом. Dummy head обрабатывает удаление
первого узла без специального кода.

**Сложность:** O(N) время, O(1) память; один проход по списку.

---

### Задача 6. Palindrome Linked List — палиндром

Определить, является ли связный список палиндромом. Решить за O(N) время и O(1) память.

```go
func isPalindrome(head *ListNode) bool {
    if head == nil || head.Next == nil {
        return true
    }

    // Шаг 1: найти середину
    slow, fast := head, head
    for fast != nil && fast.Next != nil {
        slow = slow.Next
        fast = fast.Next.Next
    }
    // slow теперь указывает на начало второй половины

    // Шаг 2: развернуть вторую половину
    secondHalf := reverseList(slow)
    secondHalfCopy := secondHalf // сохранить для восстановления

    // Шаг 3: сравнить первую и вторую половины
    p1, p2 := head, secondHalf
    result := true
    for p2 != nil {
        if p1.Val != p2.Val {
            result = false
            break
        }
        p1 = p1.Next
        p2 = p2.Next
    }

    // Шаг 4: восстановить список (good practice)
    reverseList(secondHalfCopy)
    return result
}

// повторно использует reverseList из Задачи 1
func reverseList(head *ListNode) *ListNode {
    var prev *ListNode
    curr := head
    for curr != nil {
        next := curr.Next
        curr.Next = prev
        prev = curr
        curr = next
    }
    return prev
}
```

Алгоритм состоит из четырёх шагов: найти середину (fast/slow), развернуть вторую половину на месте,
сравнить посимвольно обе половины, восстановить список в исходное состояние. Восстановление
технически необязательно для прохождения теста, но хорошая практика — вызывающий код не должен
получать испорченную структуру данных.

Для нечётного числа узлов (например, 1→2→3→2→1) после поиска середины `slow` будет на узле 3.
Развёрнутая вторая половина — 1→2, а первая — 1→2→3. Сравнение остановится, когда `p2` исчерпается,
узел 3 в середине в сравнении не участвует.

**Сложность:** O(N) время, O(1) доп. память.

---

## Сводная таблица сложности

| Задача                       | Время         | Память   |
|------------------------------|---------------|----------|
| Permutations                 | O(N! * N)     | O(N)     |
| Subsets                      | O(2^N * N)    | O(N)     |
| Combination Sum              | O(N^(T/M))    | O(T/M)   |
| Word Search                  | O(M*N * 4^L)  | O(L)     |
| N-Queens                     | O(N!)         | O(N)     |
| Reverse Linked List          | O(N)          | O(1)     |
| Find Middle                  | O(N)          | O(1)     |
| Detect Cycle Start           | O(N)          | O(1)     |
| Merge Two Sorted Lists       | O(N + M)      | O(1)     |
| Remove Nth From End          | O(N)          | O(1)     |
| Palindrome Linked List       | O(N)          | O(1)     |

---

## Частые ошибки на интервью

**Backtracking:**
- Забыть `copy` при сохранении `path` — все результаты будут указывать на один слайс.
- Передавать `path` по значению в Go без понимания того, что append может создать новый backing array;
  безопаснее явно передавать длину и обрезать вручную (`path = path[:len(path)-1`).
- Не восстанавливать изменённые данные (например, `board` в Word Search).
- Использовать `continue` вместо `break` при pruning по отсортированному массиву.

**Linked List:**
- Обращение к `node.Next` без проверки `node != nil` — паника в рантайме.
- Не использовать dummy head и городить ветки `if head == nil`.
- В задаче "Remove Nth from End" сдвигать fast на N вместо N+1 шагов, из-за чего slow останавливается
  на удаляемом узле, а не перед ним.
- Забыть восстановить список после `isPalindrome` — структура остаётся частично развёрнутой.
