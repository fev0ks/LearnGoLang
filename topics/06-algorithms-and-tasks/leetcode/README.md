# LeetCode

Решения настоящих задач LeetCode, разложенные по уровню сложности.

## Структура

```
leetcode/
├── easy/
├── medium/
└── hard/
```

Папка каждой задачи называется `<номер>-<kebab-имя>` (номер с ведущими нулями,
чтобы сортировка была по возрастанию). Уровень кодируется родительской папкой,
поэтому в имени не дублируется. Внутри — решение и (где есть) табличные тесты.

## Индекс задач

Колонка «Решение» — ссылка для готовых задач или 🔜 для запланированных (популярная
классика, которую планируем добавить).

### Easy

| № | Задача | Тема | Решение |
| --- | --- | --- | --- |
| 1 | Two Sum | Хеш-таблица | [0001-two-sum](easy/0001-two-sum) |
| 14 | Longest Common Prefix | Строки | 🔜 |
| 20 | Valid Parentheses | Стек | [0020-valid-parentheses](easy/0020-valid-parentheses) |
| 21 | Merge Two Sorted Lists | Связный список | [0021-merge-two-sorted-lists](easy/0021-merge-two-sorted-lists) |
| 70 | Climbing Stairs | Динамическое программирование | [0070-climbing-stairs](easy/0070-climbing-stairs) |
| 88 | Merge Sorted Array | Два указателя | 🔜 |
| 101 | Symmetric Tree | Деревья | [0101-symmetric-tree](easy/0101-symmetric-tree) |
| 104 | Maximum Depth of Binary Tree | Деревья (DFS) | [0104-maximum-depth-binary-tree](easy/0104-maximum-depth-binary-tree) |
| 121 | Best Time to Buy and Sell Stock | Один проход / DP | [0121-best-time-to-buy-sell-stock](easy/0121-best-time-to-buy-sell-stock) |
| 141 | Linked List Cycle | Два указателя (Флойд) | 🔜 |
| 169 | Majority Element | Голосование Бойера-Мура | 🔜 |
| 206 | Reverse Linked List | Связный список | [0206-reverse-linked-list](easy/0206-reverse-linked-list) |
| 226 | Invert Binary Tree | Деревья | 🔜 |
| 242 | Valid Anagram | Хеш-таблица | [0242-valid-anagram](easy/0242-valid-anagram) |
| 643 | Maximum Average Subarray I | Sliding window | [0643-maximum-average-subarray](easy/0643-maximum-average-subarray) |
| 844 | Backspace String Compare | Два указателя / стек | [0844-backspace-string-compare](easy/0844-backspace-string-compare) |
| 1365 | How Many Numbers Are Smaller Than the Current Number | Counting sort | [1365-how-many-numbers-smaller-than-current](easy/1365-how-many-numbers-smaller-than-current) |

### Medium

| № | Задача | Тема | Решение |
| --- | --- | --- | --- |
| 3 | Longest Substring Without Repeating | Sliding window | [0003-longest-substring-without-repeating](medium/0003-longest-substring-without-repeating) |
| 5 | Longest Palindromic Substring | Строки / DP | 🔜 |
| 11 | Container With Most Water | Два указателя | 🔜 |
| 15 | 3Sum | Два указателя | [0015-3sum](medium/0015-3sum) |
| 22 | Generate Parentheses | Backtracking | 🔜 |
| 33 | Search in Rotated Sorted Array | Бинарный поиск | [0033-search-in-rotated-sorted-array](medium/0033-search-in-rotated-sorted-array) |
| 46 | Permutations | Backtracking | 🔜 |
| 49 | Group Anagrams | Хеш-таблица | [0049-group-anagrams](medium/0049-group-anagrams) |
| 53 | Maximum Subarray | DP (Kadane) | [0053-maximum-subarray](medium/0053-maximum-subarray) |
| 56 | Merge Intervals | Интервалы | [0056-merge-intervals](medium/0056-merge-intervals) |
| 71 | Simplify Path | Стек | [0071-simplify-path](medium/0071-simplify-path) |
| 78 | Subsets | Backtracking | 🔜 |
| 79 | Word Search | Backtracking / DFS | 🔜 |
| 98 | Validate Binary Search Tree | Деревья (BST) | 🔜 |
| 102 | Binary Tree Level Order Traversal | Деревья (BFS) | [0102-binary-tree-level-order](medium/0102-binary-tree-level-order) |
| 105 | Construct Binary Tree (Preorder+Inorder) | Деревья | 🔜 |
| 128 | Longest Consecutive Sequence | Хеш-таблица | 🔜 |
| 133 | Clone Graph | Графы | 🔜 |
| 139 | Word Break | DP | 🔜 |
| 146 | LRU Cache | Дизайн (список + map), O(1) | [0146-lru-cache](medium/0146-lru-cache) |
| 198 | House Robber | DP | 🔜 |
| 200 | Number of Islands | Графы (DFS/BFS) | [0200-number-of-islands](medium/0200-number-of-islands) |
| 207 | Course Schedule | Графы (топосорт) | [0207-course-schedule](medium/0207-course-schedule) |
| 208 | Implement Trie | Trie / дизайн | 🔜 |
| 215 | Kth Largest Element | Куча | [0215-kth-largest-element](medium/0215-kth-largest-element) |
| 236 | Lowest Common Ancestor | Деревья | 🔜 |
| 238 | Product of Array Except Self | Префиксные произведения | 🔜 |
| 322 | Coin Change | DP | 🔜 |
| 347 | Top K Frequent Elements | Куча / bucket sort | 🔜 |
| 904 | Fruit Into Baskets | Sliding window | [0904-fruit-into-baskets](medium/0904-fruit-into-baskets) |
| 994 | Rotting Oranges | Графы (BFS по матрице) | 🔜 |
| 1143 | Longest Common Subsequence | DP | 🔜 |

### Hard

| № | Задача | Тема | Решение |
| --- | --- | --- | --- |
| 4 | Median of Two Sorted Arrays | Бинарный поиск | 🔜 |
| 23 | Merge k Sorted Lists | Куча | [0023-merge-k-sorted-lists](hard/0023-merge-k-sorted-lists) |
| 42 | Trapping Rain Water | Два указателя / стек | [0042-trapping-rain-water](hard/0042-trapping-rain-water) |
| 72 | Edit Distance | DP | 🔜 |
| 76 | Minimum Window Substring | Sliding window | [0076-minimum-window-substring](hard/0076-minimum-window-substring) |
| 84 | Largest Rectangle in Histogram | Монотонный стек | 🔜 |
| 124 | Binary Tree Maximum Path Sum | Деревья | 🔜 |
| 127 | Word Ladder | Графы (BFS) | 🔜 |
| 239 | Sliding Window Maximum | Монотонная очередь | 🔜 |
| 295 | Find Median from Data Stream | Две кучи | [0295-find-median-from-data-stream](hard/0295-find-median-from-data-stream) |
| 297 | Serialize and Deserialize Binary Tree | Деревья (обход) | [0297-serialize-deserialize-binary-tree](hard/0297-serialize-deserialize-binary-tree) |

## Как запускать

Задачи с тестами (`*_test.go`):

```bash
go test ./topics/06-algorithms-and-tasks/leetcode/easy/0101-symmetric-tree/
```

Задачи с `main()` (демо-прогон в консоль):

```bash
go run ./topics/06-algorithms-and-tasks/leetcode/medium/0146-lru-cache/
```

## Что относится сюда, а что нет

Сюда кладём только задачи, у которых есть официальный номер LeetCode. Кастомные
собесные вариации «по мотивам» (скобочные задачи, обход матрицы, восстановление
маршрута и т.п.), concurrency-задачи и структуры данных живут в
[../code_examples](../code_examples).
