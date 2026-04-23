# Algorithms And Complexity

Этот подпакет не про олимпиадщину, а про базу, которую часто спрашивают на backend собеседованиях. Каждый файл — паттерн с полными Go-реализациями и разбором сложности.

## Материалы

- [01. Time And Space Complexity](./01-time-and-space-complexity.md) — O-нотация, анализ сложности, типичные примеры
- [02. Common Algorithm Patterns](./02-common-algorithm-patterns-and-examples-in-go.md) — обзор паттернов
- [03. Two Pointers And Sliding Window](./03-two-pointers-and-sliding-window.md) — opposite ends, fast/slow, same direction, fixed/variable window
- [04. Binary Search](./04-binary-search.md) — classic, lower/upper bound, rotated array, binary search on answer
- [05. Trees And Graphs](./05-trees-and-graphs.md) — обходы дерева, BFS/DFS на графе, топологическая сортировка, Union-Find
- [06. Dynamic Programming](./06-dynamic-programming.md) — memoization vs tabulation, 1D/2D DP, классические задачи
- [07. Sorting And Heap](./07-sorting-and-heap.md) — merge sort, quick sort, container/heap, top-K задачи
- [08. Backtracking And Linked List](./08-backtracking-and-linked-list.md) — шаблон backtracking, permutations/subsets, операции со списками

## Как читать

1. `01–02` — сначала, если не уверен в O-нотации
2. `03–04` — самые частые паттерны на массивах и строках
3. `05` — деревья и графы: BFS/DFS, топосортировка, DSU
4. `06` — DP: climbing stairs → knapsack
5. `07` — сортировки и heap для top-K задач
6. `08` — backtracking и операции со связными списками

## Что важно уметь

- объяснить `O(1)`, `O(log n)`, `O(n)`, `O(n log n)`, `O(n²)` и назвать примеры
- не написать `O(n²)` там, где достаточно `O(n)` с хэш-мапой
- реализовать two pointers, sliding window, binary search без подсказок
- обойти дерево итеративно (BFS с очередью, DFS со стеком)
- написать backtracking шаблон и применить pruning
- использовать `container/heap` для top-K задач
- объяснить разницу memoization vs tabulation и когда что выбрать
