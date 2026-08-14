# Задача 4: Trie (Prefix Tree)

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы)
- [Базовая реализация](#базовое-решение-для-lowercase-ascii)
- [Unicode и переменный алфавит](#для-unicode--переменного-алфавита)
- [Autocomplete](#autocomplete-top-k-с-префиксом)
- [Тесты](#тесты)
- [Типичные ошибки](#подводные-камни)
- [Расширения](#возможные-расширения)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связки)

Trie, или префиксное дерево, хранит ключи по частям: путь от корня до узла
соответствует префиксу. В строковом trie ребро обычно обозначает байт или Unicode
code point, а отдельный флаг отличает полное слово от промежуточного префикса.

---

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

func (t *Trie) Insert(word string) bool {
    for i := 0; i < len(word); i++ {
        if _, ok := letterIndex(word[i]); !ok {
            return false
        }
    }

    n := t.root
    for i := 0; i < len(word); i++ {
        index, _ := letterIndex(word[i])
        if n.children[index] == nil {
            n.children[index] = &node{}
        }
        n = n.children[index]
    }
    n.isEnd = true
    return true
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
        index, ok := letterIndex(s[i])
        if !ok || n.children[index] == nil {
            return nil
        }
        n = n.children[index]
    }
    return n
}

func letterIndex(char byte) (int, bool) {
    if char < 'a' || char > 'z' {
        return 0, false
    }
    return int(char - 'a'), true
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

Время операций равно `O(L)`, где `L` — число байт. Если создано `P` узлов,
вариант с массивом использует `O(P * |alphabet|)` ссылок; в худшем случае
`P = O(N * L)` для `N` ключей без общих префиксов.

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

`map[rune]*node` хранит только существующие рёбра и поддерживает переменный
алфавит, но платит хешированием и дополнительными allocations. Массив обычно
компактнее только при маленьком плотном алфавите. Итерация по `rune` различает
code points, но не нормализует эквивалентные Unicode-последовательности.

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

func NewAutocomplete() *Autocomplete {
    return &Autocomplete{root: &autocompleteNode{}}
}

func (a *Autocomplete) AddWord(word string, count int) bool {
    for i := 0; i < len(word); i++ {
        if _, ok := letterIndex(word[i]); !ok {
            return false
        }
    }

    n := a.root
    for i := 0; i < len(word); i++ {
        index, _ := letterIndex(word[i])
        if n.children[index] == nil {
            n.children[index] = &autocompleteNode{}
        }
        n = n.children[index]
    }
    n.isEnd = true
    n.count += count
    return true
}

// Suggest возвращает топ-K самых частых слов с данным префиксом.
func (a *Autocomplete) Suggest(prefix string, k int) []string {
    if k <= 0 {
        return nil
    }

    // Найти node префикса
    n := a.root
    for i := 0; i < len(prefix); i++ {
        index, ok := letterIndex(prefix[i])
        if !ok || n.children[index] == nil {
            return nil
        }
        n = n.children[index]
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
ac := NewAutocomplete()
ac.AddWord("apple", 100)
ac.AddWord("application", 50)
ac.AddWord("app", 200)
ac.AddWord("apricot", 30)

suggestions := ac.Suggest("app", 3)
// ["app", "apple", "application"]
```

Для большого словаря можно хранить top-K в каждом узле заранее. Тогда чтение
занимает `O(L + K)`, но вставка обновляет кандидатов во всех `L` узлах пути, а
память может вырасти до `O(P * K)`.

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
    ac := NewAutocomplete()

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

На 64-битной архитектуре один массив из 26 указателей занимает 208 байт ещё до
учёта остальных полей и выравнивания узла. Для разреженного или большого
алфавита такой массив расходует память впустую.

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

`append` может использовать общий backing array. В приведённом последовательном
DFS это безопасно, потому что slice не сохраняется, а `string(current)` копирует
байты. Ошибка появится, если сохранять сами slices в результате или обходить
ветви параллельно. Тогда путь нужно явно копировать:

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

Большое дерево из узлов с указателями создаёт значительный overhead и нагрузку
на garbage collector. Сжатый radix tree уменьшает число узлов, но persistence
требует отдельного формата хранения, а не только замены типа дерева.

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

Сжатые пути уменьшают число узлов. Radix-подобные структуры часто применяются в
маршрутизаторах, но точная реализация зависит от конкретной библиотеки и версии.

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
- **Маршрутизация HTTP —** сопоставление сегментов пути и параметров.
- **Префиксная IP-маршрутизация —** longest-prefix match; конкретная структура
  зависит от реализации.
- **Словари и autocomplete —** поиск слов по введённому префиксу.
- **Поиск последовательностей —** работа с общими префиксами шаблонов.
- **Таблицы символов —** поиск имён внутри пространств имён.

---

## Interview-ready answer

**1. Как устроен trie и какова сложность операций?**

- Модель — путь от корня кодирует префикс, а `isEnd` отмечает полный ключ.
- Время — поиск и вставка занимают `O(L)` по длине ключа и не зависят напрямую
  от числа сохранённых ключей.
- Память — зависит от числа узлов и представления детей; массив выгоден для
  маленького плотного алфавита, `map` — для разреженного.

**2. Чем Contains отличается от StartsWith?**

- `StartsWith` — достаточно существования пути для всего префикса.
- `Contains` — конечный узел дополнительно должен иметь `isEnd = true`.
- Пример — после вставки `apple` префикс `app` существует, но отдельного слова
  `app` нет.

**3. Как построить autocomplete?**

- Базовый вариант — найти узел префикса, обойти его поддерево и выбрать top-K.
- Цена — полный DFS зависит от числа слов под популярным префиксом.
- Оптимизация — хранить кандидатов в узлах заранее, платя памятью и более дорогим
  обновлением.

**4. Какие ограничения важны в Go?**

- Ввод — индекс `word[i]-'a'` безопасен только после проверки диапазона.
- Unicode — обход по `rune` не заменяет нормализацию строк.
- Конкурентность — параллельная запись требует синхронизации или публикации
  неизменяемого дерева после построения.

---

## Связки

- [Algorithms: trees and graphs](../../../16-algorithms-and-data-structures/04-trees-and-graphs.md)
- [HTTP servers](../../../03-go-libraries-and-ecosystem/http-servers/) —
  практический контекст маршрутизации путей.
- [Оценка сложности](../../../16-algorithms-and-data-structures/01-time-and-space-complexity.md)
  — анализ времени и памяти.
