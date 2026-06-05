# Algorithms And Data Structures

Теория алгоритмов и структур данных — с акцентом на Go и на то, что реально встречается на backend-интервью и в production-коде. Не олимпиадная программа, а база которую должен знать senior.

## Материалы

- [01. Time And Space Complexity](./01-time-and-space-complexity.md) — O-нотация, таблица классов, ASCII-диаграммы роста, амортизированная сложность, Go-примеры
- [02. Patterns Overview](./02-patterns-overview.md) — таблица "признак задачи → паттерн", обзор всех паттернов, фреймворк для интервью, сигналы оптимизации
- [03. Two Pointers And Sliding Window](./03-two-pointers-and-sliding-window.md) — opposite ends, fast/slow, same direction, fixed/variable window с 9 задачами
- [04. Binary Search](./04-binary-search.md) — classic, lower/upper bound, rotated array, binary search on answer
- [05. Trees And Graphs](./05-trees-and-graphs.md) — обходы дерева, BFS/DFS на графе, топологическая сортировка, Union-Find
- [06. Dynamic Programming](./06-dynamic-programming.md) — memoization vs tabulation, 1D/2D DP, классические задачи
- [07. Sorting And Heap](./07-sorting-and-heap.md) — merge sort, quick sort, container/heap, top-K задачи
- [08. Backtracking And Linked List](./08-backtracking-and-linked-list.md) — шаблон backtracking, permutations/subsets, операции со связными списками

## Как читать

1. `01` — сначала, если не уверен в O-нотации и как её считать
2. `02` — обзор паттернов и таблица распознавания: с него удобно начинать перед задачей
3. `03–04` — самые частые паттерны: two pointers, sliding window, binary search
4. `05` — деревья и графы: BFS/DFS, топосортировка, Union-Find
5. `06` — DP: от climbing stairs до knapsack
6. `07` — сортировки и heap для top-K задач
7. `08` — backtracking и операции со связными списками

## Что важно уметь

- объяснить O(1) / O(log n) / O(n) / O(n log n) / O(n²) и назвать примеры для каждого
- не написать O(n²) там, где достаточно O(n) с хэш-мапой
- реализовать two pointers, sliding window, binary search без подсказок
- обойти дерево итеративно (BFS с очередью, DFS со стеком)
- написать backtracking-шаблон и применить pruning
- использовать `container/heap` для top-K задач
- объяснить разницу memoization vs tabulation и когда что выбрать
- назвать сложность стандартных операций Go: append, map lookup, sort
