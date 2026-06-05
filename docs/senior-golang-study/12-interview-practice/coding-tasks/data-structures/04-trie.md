# Задача 4: Trie (Prefix Tree)

Trie — дерево, где каждый node = одна буква, путь от root до node = строка. Используется для **autocomplete**, **spell check**, **routing** (HTTP routers как chi/gin внутри), **IP prefix matching**.

## Формулировка

> "Реализуй структуру для быстрого поиска по префиксу. Insert, Contains, и StartsWith за O(L), где L — длина строки."

Вариации:
- "Autocomplete: дай топ-K слов с данным префиксом"
- "Spell-check: найди все слова на расстоянии edit distance ≤ 2"
- "HTTP router: match /users/:id/posts/:postId"

---

## Уточняющие вопросы

1. **Алфавит — ASCII (26 букв) или Unicode?**
   "26 букв — array[26]. Unicode — map[rune]node, медленнее но универсально."

2. **Case sensitive?**
   "Если нет — lowercase на insert и search."

3. **Только префикс или полные строки?**
   "Differentiate через `isEnd` флаг — node может быть и префиксом, и concrete word'ом."

4. **Counting (сколько слов с префиксом)?**
   "Хранить count в каждом node — увеличивать на insert."

5. **Удаление поддерживается?**
   "Возможно через ref counting в каждом node."

6. **Concurrent?**
   "Обычно build один раз, потом read-only. Если concurrent build — mutex."

---

## Базовое решение: для lowercase ASCII

```go
package trie

type node struct {
    children [26]*node
    isEnd    bool
}

type Trie struct {
    root *node
}

func New() *Trie {
    return &Trie{root: &node{}}
}

func (t *Trie) Insert(word string) {
    n := t.root
    for i := 0; i < len(word); i++ {
        c := word[i] - 'a'
        if n.children[c] == nil {
            n.children[c] = &node{}
        }
        n = n.children[c]
    }
    n.isEnd = true
}

func (t *Trie) Contains(word string) bool {
    n := t.find(word)
    return n != nil && n.isEnd
}

func (t *Trie) StartsWith(prefix string) bool {
    return t.find(prefix) != nil
}

func (t *Trie) find(s string) *node {
    n := t.root
    for i := 0; i < len(s); i++ {
        c := s[i] - 'a'
        if n.children[c] == nil {
            return nil
        }
        n = n.children[c]
    }
    return n
}
```

**Использование:**

```go
t := trie.New()
t.Insert("apple")
t.Insert("app")
t.Insert("application")

t.Contains("app")        // true
t.Contains("appl")       // false (нет такого word'а, только prefix)
t.StartsWith("appl")     // true
t.Contains("apple")      // true
```

**Сложность:**
- Time: O(L) для всех операций (L = длина слова)
- Memory: O(N * L * |alphabet|) worst case. С общими префиксами — лучше.

---

## Для Unicode / переменного алфавита

```go
type node struct {
    children map[rune]*node
    isEnd    bool
}

func (t *Trie) Insert(word string) {
    n := t.root
    for _, r := range word {  // итерация по runes (Unicode-aware)
        if n.children == nil {
            n.children = make(map[rune]*node)
        }
        if _, ok := n.children[r]; !ok {
            n.children[r] = &node{}
        }
        n = n.children[r]
    }
    n.isEnd = true
}
```

**Trade-off:** Unicode поддерживается, но map медленнее array (~3-5x slower per access). Для autocomplete с английским — array быстрее.

---

## Autocomplete: top-K с префиксом

Расширение Trie — хранить **частоты** (count). Autocomplete делает DFS из node префикса, собирает все слова, сортирует по count.

```go
type autocompleteNode struct {
    children [26]*autocompleteNode
    isEnd    bool
    count    int  // частота этого слова в корпусе
}

type Autocomplete struct {
    root *autocompleteNode
}

func (a *Autocomplete) AddWord(word string, count int) {
    n := a.root
    for i := 0; i < len(word); i++ {
        c := word[i] - 'a'
        if n.children[c] == nil {
            n.children[c] = &autocompleteNode{}
        }
        n = n.children[c]
    }
    n.isEnd = true
    n.count += count
}

// Suggest возвращает топ-K самых частых слов с данным префиксом.
func (a *Autocomplete) Suggest(prefix string, k int) []string {
    // Найти node префикса
    n := a.root
    for i := 0; i < len(prefix); i++ {
        c := prefix[i] - 'a'
        if n.children[c] == nil {
            return nil
        }
        n = n.children[c]
    }

    // DFS — собрать все слова из этого поддерева
    type wordCount struct {
        word  string
        count int
    }
    var results []wordCount

    var dfs func(n *autocompleteNode, current []byte)
    dfs = func(n *autocompleteNode, current []byte) {
        if n.isEnd {
            results = append(results, wordCount{
                word:  string(current),
                count: n.count,
            })
        }
        for i := 0; i < 26; i++ {
            if n.children[i] != nil {
                dfs(n.children[i], append(current, byte('a'+i)))
            }
        }
    }
    dfs(n, []byte(prefix))

    // Top-K по count'у
    sort.Slice(results, func(i, j int) bool {
        return results[i].count > results[j].count
    })
    if k > len(results) {
        k = len(results)
    }

    out := make([]string, k)
    for i := 0; i < k; i++ {
        out[i] = results[i].word
    }
    return out
}
```

**Использование:**

```go
ac := &Autocomplete{root: &autocompleteNode{}}
ac.AddWord("apple", 100)
ac.AddWord("application", 50)
ac.AddWord("app", 200)
ac.AddWord("apricot", 30)

suggestions := ac.Suggest("app", 3)
// ["app", "apple", "application"]
```

**Оптимизация:** для очень больших словарей хранить **top-K в каждом node** заранее. На insert — обновлять topK по пути. Тогда Suggest = O(L), не O(всех слов с префиксом).

---

## Тесты

```go
func TestTrie_Basic(t *testing.T) {
    tr := New()

    tr.Insert("apple")

    if !tr.Contains("apple") {
        t.Error("apple should be contained")
    }
    if tr.Contains("app") {
        t.Error("app shouldn't be contained (only prefix)")
    }
    if !tr.StartsWith("app") {
        t.Error("app should be a prefix")
    }
    if !tr.StartsWith("apple") {
        t.Error("apple should be both word and prefix")
    }
}

func TestTrie_MultipleWords(t *testing.T) {
    tr := New()

    words := []string{"cat", "car", "card", "care", "careful", "cars"}
    for _, w := range words {
        tr.Insert(w)
    }

    for _, w := range words {
        if !tr.Contains(w) {
            t.Errorf("missing: %s", w)
        }
    }

    if tr.Contains("ca") {
        t.Error("ca shouldn't be word")
    }
}

func TestAutocomplete(t *testing.T) {
    ac := &Autocomplete{root: &autocompleteNode{}}

    ac.AddWord("apple", 100)
    ac.AddWord("application", 50)
    ac.AddWord("app", 200)
    ac.AddWord("banana", 1000)  // другой prefix

    suggestions := ac.Suggest("app", 2)

    expected := []string{"app", "apple"}
    if !reflect.DeepEqual(suggestions, expected) {
        t.Errorf("got %v, want %v", suggestions, expected)
    }
}
```

---

## Подводные камни

### 1. Memory blowup для широкого алфавита

`children [26]*node` — 26 указателей по 8 байт = 208 байт на каждый node. Для Unicode (millions of code points) — array невозможен.

Решения:
- Использовать map для редких символов
- Compressed trie / Patricia trie
- Trie с использованием bitwise для дочерних slot'ов

### 2. Деление по `'a'`

```go
c := word[i] - 'a'  // ← assumes только lowercase a-z
```

Если придёт uppercase или digit — out-of-bounds. Защита: lowercase + проверка.

```go
if word[i] < 'a' || word[i] > 'z' {
    // Skip, error, или нормализовать
}
```

### 3. isEnd vs только terminator символ

Альтернатива isEnd — class terminator символ ($). Не используй — `$` может быть в input'е.

### 4. DFS без depth limit

Если в trie есть очень длинные слова — DFS stack overflow. Maxим depth — typically OK для slovesных задач (десятки символов).

### 5. Slice mutation в DFS

```go
dfs(n.children[i], append(current, byte('a'+i)))
```

`append` может share underlying array. Если slice grows — может corrupt'нуть parent'ов state.

Безопаснее — explicit copy:
```go
next := make([]byte, len(current)+1)
copy(next, current)
next[len(current)] = byte('a'+i)
dfs(n.children[i], next)
```

### 6. Concurrent read + write

Trie обычно build один раз → read many. Если concurrent insert — нужен mutex. Альтернатива — immutable persistent trie (но это сложно в Go).

### 7. Forget what's been inserted

```go
t.Insert("apple")
// Удалить "apple" — нужно явно. Trie не tracks insertion order.
```

Delete: пройти от corner case (один word с этим path) и удалять nodes до встречи branching point. Сложнее чем insert.

### 8. Trie на disk — radix tree

In-memory trie с N=1B нодов = 100+ GB. Для persistent — Radix tree (compressed paths). См. BadgerDB, etcd.

### 9. Autocomplete без top-K cache

Каждый Suggest("app", 10) — DFS по всему поддереву "app". Для популярных префиксов медленно. Решение: cache top-K per node.

---

## Возможные расширения

### 1. Radix Tree (compressed trie)

Объединяет paths где node имеет один child. Например:
```
Trie:    a → p → p → l → e
Radix:   "appl" → e
```

Меньше памяти, быстрее. Используется в HTTP routers (chi, gin, fasthttp).

### 2. Suffix Tree

Для всех суффиксов строки. Используется в bioinformatics (DNA matching), full-text search.

### 3. Aho-Corasick

Trie + finite automaton для multi-pattern matching. Поиск всех вхождений N patterns в text за O(|text| + |patterns| + matches).

### 4. Persistent Trie

После каждого insert — новая версия. Immutable. Используется в functional programming.

### 5. Fuzzy search через Levenshtein automaton

Найти все слова на расстоянии ≤ k от query. Используется в spell check, search engines.

### 6. Distributed Trie

Шардить по first character: a-h → node1, i-p → node2, q-z → node3. Простой, но disbalanced.

---

## Реальные применения

- **Autocomplete** — Google search bar, ChatGPT prompt suggestions
- **HTTP routing** — chi/gin/fasthttp matching `/users/:id/posts/:postId`
- **IP routing** — kernel routing table, prefix matching CIDR
- **Spell checkers** — Hunspell, browser spell-check
- **DNA matching** — bioinformatics
- **Compiler symbol tables** — namespace lookup
- **PDF text search** — Adobe Reader

---

## Что важно показать на собеседовании

1. **O(L)** для всех операций — не O(N где N = слов).
2. **`children [26]*node` vs map** — trade-off speed vs flexibility.
3. **isEnd флаг** — чтобы различать word vs only prefix.
4. **DFS для autocomplete** — стандартный паттерн обхода.
5. **Top-K cache** для production autocomplete — оптимизация.
6. **Radix tree** в production routers — связь с реальностью.
7. **Memory considerations** — 26 pointers per node bloat'ит.

## Связки

- [Algorithms: trees and graphs](../../../16-algorithms-and-data-structures/05-trees-and-graphs.md)
- [HTTP routers](../../../03-go-libraries-and-ecosystem/http-servers/) — chi/gin internal trie
- [BadgerDB / Radix tree](https://github.com/dgraph-io/badger) — persistent trie variant
