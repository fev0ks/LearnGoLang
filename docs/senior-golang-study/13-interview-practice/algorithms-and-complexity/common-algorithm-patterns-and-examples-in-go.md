# Common Algorithm Patterns And Examples In Go

Здесь собраны не "редкие tricky задачи", а то, что реально часто всплывает на интервью и в коде.

## Hash map lookup

Типичный use case:
- deduplication;
- frequency count;
- membership check.

Пример:

```go
seen := make(map[string]struct{})
for _, v := range values {
    if _, ok := seen[v]; ok {
        continue
    }
    seen[v] = struct{}{}
}
```

Обычно:
- average time `O(1)` на операцию;
- память `O(n)`.

## Linear scan

Пример:

```go
for _, v := range values {
    if v == target {
        return true
    }
}
```

Сложность:
- time `O(n)`
- space `O(1)`

## Sorting

В Go обычно используют стандартную сортировку:

```go
slices.Sort(nums)
```

или:

```go
slices.SortFunc(items, cmp)
```

Обычно нужно помнить:
- sorting быстрее, чем `O(n^2)` naive подход;
- после сортировки можно делать binary search.

## Binary search

Подходит:
- когда данные уже отсортированы;
- нужен быстрый поиск.

Типичная сложность:
- `O(log n)`

## Two pointers

Часто полезно:
- на отсортированных массивах;
- при поиске пары;
- при работе со строками и окнами.

## Sliding window

Полезно для:
- longest substring;
- rate limiting logic;
- moving aggregates.

## BFS and DFS

Полезны для:
- графов;
- dependency resolution;
- traversal деревьев и связных структур.

На обычных backend интервью достаточно понимать:
- когда нужен обход в ширину;
- когда подходит обход в глубину;
- что у графов легко получить `O(V+E)`.

## Что чаще всего бывает в Go-коде

На практике в Go чаще всплывают:
- map for lookup and dedup;
- slices plus sorting;
- heap or priority queue в специальных задачах;
- queue and graph traversal в задачах зависимостей;
- string processing и window patterns.

## Что важно на интервью

Обычно не ждут, что ты вспомнишь экзотику.

Ждут, что ты:
- не напишешь `O(n^2)` там, где нужен `O(n)`;
- умеешь использовать map;
- понимаешь цену сортировки;
- можешь объяснить trade-off между временем и памятью.
