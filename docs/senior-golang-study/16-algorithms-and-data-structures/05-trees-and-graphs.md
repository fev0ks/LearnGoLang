# Trees And Graphs

Деревья и графы — одна из самых частых тем на алгоритмических интервью. Задачи на них проверяют понимание рекурсии, обходов, работы с очередями и стеками, а также умение анализировать сложность.

## Содержание

- [Определения](#определения)
- [Обходы дерева](#обходы-дерева)
  - [Inorder](#inorder-left--root--right)
  - [Preorder](#preorder-root--left--right)
  - [Postorder](#postorder-left--right--root)
  - [Level order BFS](#level-order-bfs)
- [Основные задачи на деревьях](#основные-задачи-на-деревьях)
  - [Maximum depth](#maximum-depth)
  - [Same tree](#same-tree)
  - [Invert binary tree](#invert-binary-tree)
  - [Lowest Common Ancestor of BST](#lowest-common-ancestor-of-bst)
  - [Validate BST](#validate-bst)
  - [Diameter of binary tree](#diameter-of-binary-tree)
- [BFS на графе](#bfs-на-графе)
- [DFS на графе](#dfs-на-графе)
  - [Connected components](#connected-components)
  - [Cycle detection в ориентированном графе](#cycle-detection-в-ориентированном-графе)
- [Топологическая сортировка — алгоритм Кана](#топологическая-сортировка--алгоритм-кана)
- [Union-Find (Disjoint Set Union)](#union-find-disjoint-set-union)
- [Когда что применять](#когда-что-применять)

---

## Определения

Базовые структуры, которые используются во всех примерах ниже.

```go
// TreeNode — узел бинарного дерева.
type TreeNode struct {
    Val   int
    Left  *TreeNode
    Right *TreeNode
}

// Graph — граф в виде списка смежности.
// Ключ — вершина, значение — список соседей.
type Graph map[int][]int
```

---

## Обходы дерева

### Inorder (left -> root -> right)

Inorder обход BST возвращает элементы в отсортированном порядке. Это ключевое свойство, которое часто используется для валидации и поиска k-го наименьшего элемента.

**Рекурсивный вариант:**

```go
func inorderRecursive(root *TreeNode) []int {
    if root == nil {
        return nil
    }
    result := inorderRecursive(root.Left)
    result = append(result, root.Val)
    result = append(result, inorderRecursive(root.Right)...)
    return result
}
```

**Итеративный вариант (явный стек):**

```go
func inorderIterative(root *TreeNode) []int {
    var result []int
    var stack []*TreeNode
    current := root

    for current != nil || len(stack) > 0 {
        for current != nil {
            stack = append(stack, current)
            current = current.Left
        }
        current = stack[len(stack)-1]
        stack = stack[:len(stack)-1]
        result = append(result, current.Val)
        current = current.Right
    }
    return result
}
```

Сложность: time `O(n)`, space `O(h)`, где `h` — высота дерева.

---

### Preorder (root -> left -> right)

Применяется при копировании дерева и сериализации: корень записывается до потомков, что позволяет восстановить структуру при десериализации.

**Рекурсивный вариант:**

```go
func preorderRecursive(root *TreeNode) []int {
    if root == nil {
        return nil
    }
    result := []int{root.Val}
    result = append(result, preorderRecursive(root.Left)...)
    result = append(result, preorderRecursive(root.Right)...)
    return result
}
```

**Итеративный вариант:**

```go
func preorderIterative(root *TreeNode) []int {
    if root == nil {
        return nil
    }
    var result []int
    stack := []*TreeNode{root}

    for len(stack) > 0 {
        node := stack[len(stack)-1]
        stack = stack[:len(stack)-1]
        result = append(result, node.Val)
        // Правый потомок кладется первым, чтобы левый обрабатывался раньше.
        if node.Right != nil {
            stack = append(stack, node.Right)
        }
        if node.Left != nil {
            stack = append(stack, node.Left)
        }
    }
    return result
}
```

Сложность: time `O(n)`, space `O(h)`.

---

### Postorder (left -> right -> root)

Постордер используется при удалении дерева (нужно удалить потомков до родителя) и при вычислении выражений в expression tree.

**Рекурсивный вариант:**

```go
func postorderRecursive(root *TreeNode) []int {
    if root == nil {
        return nil
    }
    result := postorderRecursive(root.Left)
    result = append(result, postorderRecursive(root.Right)...)
    result = append(result, root.Val)
    return result
}
```

**Итеративный вариант (два стека):**

```go
func postorderIterative(root *TreeNode) []int {
    if root == nil {
        return nil
    }
    var output []int
    stack := []*TreeNode{root}

    for len(stack) > 0 {
        node := stack[len(stack)-1]
        stack = stack[:len(stack)-1]
        output = append(output, node.Val)
        if node.Left != nil {
            stack = append(stack, node.Left)
        }
        if node.Right != nil {
            stack = append(stack, node.Right)
        }
    }
    // Результат собран в обратном порядке — разворачиваем.
    for i, j := 0, len(output)-1; i < j; i, j = i+1, j-1 {
        output[i], output[j] = output[j], output[i]
    }
    return output
}
```

Сложность: time `O(n)`, space `O(n)`.

---

### Level order BFS

Обход уровень за уровнем. Используется для поиска минимальной глубины, вывода дерева по уровням, поиска правого крайнего узла.

```go
func levelOrder(root *TreeNode) [][]int {
    if root == nil {
        return nil
    }
    var result [][]int
    queue := []*TreeNode{root}

    for len(queue) > 0 {
        levelSize := len(queue)
        var level []int

        for i := 0; i < levelSize; i++ {
            node := queue[0]
            queue = queue[1:]
            level = append(level, node.Val)
            if node.Left != nil {
                queue = append(queue, node.Left)
            }
            if node.Right != nil {
                queue = append(queue, node.Right)
            }
        }
        result = append(result, level)
    }
    return result
}
```

Сложность: time `O(n)`, space `O(w)`, где `w` — максимальная ширина уровня. В худшем случае `O(n)`.

---

## Основные задачи на деревьях

### Maximum depth

Максимальная глубина бинарного дерева. Глубина — это количество узлов вдоль самого длинного пути от корня до листа.

```go
func maxDepth(root *TreeNode) int {
    if root == nil {
        return 0
    }
    leftDepth := maxDepth(root.Left)
    rightDepth := maxDepth(root.Right)
    if leftDepth > rightDepth {
        return leftDepth + 1
    }
    return rightDepth + 1
}
```

Сложность: time `O(n)`, space `O(h)`.

---

### Same tree

Два дерева считаются одинаковыми, если они структурно идентичны и все узлы имеют одинаковые значения.

```go
func isSameTree(p *TreeNode, q *TreeNode) bool {
    if p == nil && q == nil {
        return true
    }
    if p == nil || q == nil {
        return false
    }
    if p.Val != q.Val {
        return false
    }
    return isSameTree(p.Left, q.Left) && isSameTree(p.Right, q.Right)
}
```

Сложность: time `O(n)`, space `O(h)`.

---

### Invert binary tree

Зеркальное отражение дерева — поменять местами левое и правое поддерево для каждого узла.

```go
func invertTree(root *TreeNode) *TreeNode {
    if root == nil {
        return nil
    }
    root.Left, root.Right = invertTree(root.Right), invertTree(root.Left)
    return root
}
```

Сложность: time `O(n)`, space `O(h)`.

---

### Lowest Common Ancestor of BST

В BST LCA можно найти без рекурсивного обхода всего дерева: достаточно использовать свойство упорядоченности.

```go
// lowestCommonAncestor находит LCA двух узлов p и q в BST.
func lowestCommonAncestor(root, p, q *TreeNode) *TreeNode {
    if root == nil {
        return nil
    }
    if p.Val < root.Val && q.Val < root.Val {
        return lowestCommonAncestor(root.Left, p, q)
    }
    if p.Val > root.Val && q.Val > root.Val {
        return lowestCommonAncestor(root.Right, p, q)
    }
    // Узлы расположены по разные стороны или один из них — текущий корень.
    return root
}
```

Сложность: time `O(h)`, space `O(h)`.

---

### Validate BST

Частая ошибка: проверять только `node.Left.Val < node.Val < node.Right.Val`. Это не достаточно — нужно передавать допустимые границы вниз по рекурсии.

```go
func isValidBST(root *TreeNode) bool {
    return validateBST(root, nil, nil)
}

// validateBST проверяет, что все узлы поддерева находятся в диапазоне (min, max).
// nil означает отсутствие ограничения с соответствующей стороны.
func validateBST(node *TreeNode, min, max *int) bool {
    if node == nil {
        return true
    }
    if min != nil && node.Val <= *min {
        return false
    }
    if max != nil && node.Val >= *max {
        return false
    }
    return validateBST(node.Left, min, &node.Val) &&
        validateBST(node.Right, &node.Val, max)
}
```

Почему передача границ обязательна: узел в левом поддереве должен быть меньше не только своего непосредственного родителя, но и всех предков выше по дереву.

Сложность: time `O(n)`, space `O(h)`.

---

### Diameter of binary tree

Диаметр — длина самого длинного пути между двумя узлами (количество рёбер). Путь необязательно проходит через корень.

```go
func diameterOfBinaryTree(root *TreeNode) int {
    maxDiameter := 0

    var depth func(node *TreeNode) int
    depth = func(node *TreeNode) int {
        if node == nil {
            return 0
        }
        left := depth(node.Left)
        right := depth(node.Right)
        // Диаметр через текущий узел = левая глубина + правая глубина.
        if left+right > maxDiameter {
            maxDiameter = left + right
        }
        if left > right {
            return left + 1
        }
        return right + 1
    }

    depth(root)
    return maxDiameter
}
```

Сложность: time `O(n)`, space `O(h)`.

---

## BFS на графе

BFS находит кратчайший путь в невзвешенном графе, потому что обходит вершины строго по возрастанию расстояния от источника.

```go
// shortestPath возвращает длину кратчайшего пути от start до end.
// Возвращает -1, если путь не существует.
func shortestPath(graph Graph, start, end int) int {
    if start == end {
        return 0
    }

    visited := map[int]bool{start: true}
    queue := []int{start}
    distance := 0

    for len(queue) > 0 {
        distance++
        nextQueue := make([]int, 0)

        for _, node := range queue {
            for _, neighbor := range graph[node] {
                if neighbor == end {
                    return distance
                }
                if !visited[neighbor] {
                    visited[neighbor] = true
                    nextQueue = append(nextQueue, neighbor)
                }
            }
        }
        queue = nextQueue
    }
    return -1
}
```

Ключевые моменты:
- `visited` map помечает вершины при добавлении в очередь, а не при извлечении — иначе одна вершина может быть добавлена несколько раз.
- Срез используется как очередь: `queue[0]` — голова, `append` — добавление в хвост.

Сложность: time `O(V + E)`, space `O(V)`, где `V` — число вершин, `E` — число рёбер.

---

## DFS на графе

### Connected components

Подсчёт числа связных компонент в неориентированном графе. Запускаем DFS из каждой непосещённой вершины.

```go
// countComponents возвращает количество связных компонент.
func countComponents(n int, graph Graph) int {
    visited := make(map[int]bool)
    count := 0

    var dfs func(node int)
    dfs = func(node int) {
        visited[node] = true
        for _, neighbor := range graph[node] {
            if !visited[neighbor] {
                dfs(neighbor)
            }
        }
    }

    for i := 0; i < n; i++ {
        if !visited[i] {
            dfs(i)
            count++
        }
    }
    return count
}
```

Сложность: time `O(V + E)`, space `O(V)`.

---

### Cycle detection в ориентированном графе

Для обнаружения цикла в ориентированном графе используется трёхцветная маркировка:
- `white` (0) — вершина не посещена;
- `gray` (1) — вершина находится в текущем пути рекурсии (стек вызовов);
- `black` (2) — вершина полностью обработана.

Если при DFS мы приходим в `gray` вершину — найден цикл (back edge).

```go
const (
    white = 0
    gray  = 1
    black = 2
)

// hasCycle возвращает true, если в ориентированном графе есть цикл.
func hasCycle(n int, graph Graph) bool {
    color := make([]int, n)

    var dfs func(node int) bool
    dfs = func(node int) bool {
        color[node] = gray
        for _, neighbor := range graph[node] {
            if color[neighbor] == gray {
                return true // back edge — цикл найден
            }
            if color[neighbor] == white {
                if dfs(neighbor) {
                    return true
                }
            }
        }
        color[node] = black
        return false
    }

    for i := 0; i < n; i++ {
        if color[i] == white {
            if dfs(i) {
                return true
            }
        }
    }
    return false
}
```

Важно: простой `visited bool` не работает для ориентированного графа, потому что вершина может быть посещена по другому пути без образования цикла.

Сложность: time `O(V + E)`, space `O(V)`.

---

## Топологическая сортировка — алгоритм Кана

Топологическая сортировка применяется, когда нужно выстроить порядок выполнения задач с зависимостями: сборка проекта, порядок установки пакетов, расписание курсов.

Алгоритм Кана использует понятие in-degree (число входящих рёбер). Вершины с in-degree = 0 не имеют зависимостей и могут быть выполнены первыми.

Пример задачи: можно ли пройти все курсы, если некоторые из них требуют предварительного прохождения других?

```go
// canFinish возвращает true, если можно завершить все numCourses курсов.
// prerequisites[i] = [a, b] означает: чтобы взять курс a, нужно сначала пройти b.
func canFinish(numCourses int, prerequisites [][]int) bool {
    inDegree := make([]int, numCourses)
    graph := make(Graph)

    for _, pre := range prerequisites {
        course, dep := pre[0], pre[1]
        graph[dep] = append(graph[dep], course)
        inDegree[course]++
    }

    // Добавляем все вершины с нулевым in-degree в очередь.
    var queue []int
    for i := 0; i < numCourses; i++ {
        if inDegree[i] == 0 {
            queue = append(queue, i)
        }
    }

    processed := 0
    for len(queue) > 0 {
        node := queue[0]
        queue = queue[1:]
        processed++

        for _, neighbor := range graph[node] {
            inDegree[neighbor]--
            if inDegree[neighbor] == 0 {
                queue = append(queue, neighbor)
            }
        }
    }

    // Если обработали все вершины — цикла нет, порядок существует.
    return processed == numCourses
}

// findOrder возвращает порядок прохождения курсов или nil если это невозможно.
func findOrder(numCourses int, prerequisites [][]int) []int {
    inDegree := make([]int, numCourses)
    graph := make(Graph)

    for _, pre := range prerequisites {
        course, dep := pre[0], pre[1]
        graph[dep] = append(graph[dep], course)
        inDegree[course]++
    }

    var queue []int
    for i := 0; i < numCourses; i++ {
        if inDegree[i] == 0 {
            queue = append(queue, i)
        }
    }

    var order []int
    for len(queue) > 0 {
        node := queue[0]
        queue = queue[1:]
        order = append(order, node)

        for _, neighbor := range graph[node] {
            inDegree[neighbor]--
            if inDegree[neighbor] == 0 {
                queue = append(queue, neighbor)
            }
        }
    }

    if len(order) != numCourses {
        return nil
    }
    return order
}
```

Сложность: time `O(V + E)`, space `O(V + E)`.

---

## Union-Find (Disjoint Set Union)

Union-Find — структура данных для эффективного объединения множеств и проверки принадлежности элементов одному множеству. Используется для:
- подсчёта числа связных компонент;
- задачи Kruskal MST;
- определения, создаёт ли новое ребро цикл.

Две оптимизации делают операции почти `O(1)` амортизированно:
- **path compression** — при нахождении корня сжимаем путь, напрямую привязывая узлы к корню;
- **union by rank** — всегда присоединяем меньшее дерево к большему.

```go
type UnionFind struct {
    parent []int
    rank   []int
    count  int // число связных компонент
}

func newUnionFind(n int) *UnionFind {
    parent := make([]int, n)
    rank := make([]int, n)
    for i := range parent {
        parent[i] = i
    }
    return &UnionFind{parent: parent, rank: rank, count: n}
}

// find возвращает корень множества с path compression.
func (uf *UnionFind) find(x int) int {
    if uf.parent[x] != x {
        uf.parent[x] = uf.find(uf.parent[x]) // path compression
    }
    return uf.parent[x]
}

// union объединяет два множества. Возвращает true, если они были разными.
func (uf *UnionFind) union(x, y int) bool {
    rootX := uf.find(x)
    rootY := uf.find(y)
    if rootX == rootY {
        return false
    }
    // Union by rank: присоединяем меньшее дерево к большему.
    switch {
    case uf.rank[rootX] < uf.rank[rootY]:
        uf.parent[rootX] = rootY
    case uf.rank[rootX] > uf.rank[rootY]:
        uf.parent[rootY] = rootX
    default:
        uf.parent[rootY] = rootX
        uf.rank[rootX]++
    }
    uf.count--
    return true
}

// connected проверяет принадлежность x и y одному множеству.
func (uf *UnionFind) connected(x, y int) bool {
    return uf.find(x) == uf.find(y)
}
```

Пример задачи: число связных компонент в неориентированном графе.

```go
// countComponentsUF считает связные компоненты через Union-Find.
func countComponentsUF(n int, edges [][]int) int {
    uf := newUnionFind(n)
    for _, edge := range edges {
        uf.union(edge[0], edge[1])
    }
    return uf.count
}
```

Сложность: `find` и `union` — `O(alpha(n))` амортизированно, где `alpha` — обратная функция Аккермана, практически константа.

---

## Когда что применять

| Задача | Алгоритм | Сложность |
|---|---|---|
| Обход дерева | Inorder / Preorder / Postorder | O(n) time, O(h) space |
| Обход по уровням | Level order BFS | O(n) time, O(w) space |
| Кратчайший путь (невзвешенный граф) | BFS | O(V+E) time, O(V) space |
| Достижимость, связность | DFS или BFS | O(V+E) time, O(V) space |
| Число связных компонент | DFS / Union-Find | O(V+E) / O(n * alpha(n)) |
| Цикл в ориентированном графе | DFS с трёхцветной маркировкой | O(V+E) time, O(V) space |
| Цикл в неориентированном графе | Union-Find | O(E * alpha(n)) |
| Порядок задач с зависимостями | Топологическая сортировка (Kahn) | O(V+E) time, O(V+E) space |
| Динамическое объединение множеств | Union-Find (path compression + rank) | O(alpha(n)) per op |
| Валидация BST | DFS с границами min/max | O(n) time, O(h) space |
| Диаметр дерева | DFS с глобальным максимумом | O(n) time, O(h) space |
| LCA в BST | Использование свойства BST | O(h) time, O(h) space |
| Минимальное остовное дерево | Kruskal (Union-Find) / Prim (heap) | O(E log E) |
