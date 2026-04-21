# samber/lo

`github.com/samber/lo` — generics-утилиты для коллекций. Убирает boilerplate `for` циклов для стандартных операций над slice и map.

Требует Go 1.18+.

## Зачем

До Go generics (до 1.18) каждый проект писал свои хелперы или копировал одно и то же:

```go
// Без lo — руками
emails := make([]string, 0, len(users))
for _, u := range users {
    emails = append(emails, u.Email)
}

// С lo
emails := lo.Map(users, func(u User, _ int) string {
    return u.Email
})
```

## Slice операции

```go
import "github.com/samber/lo"

users := []User{{ID: 1, Name: "Alice", Active: true}, {ID: 2, Name: "Bob", Active: false}}

// Map — трансформировать каждый элемент
names := lo.Map(users, func(u User, _ int) string {
    return u.Name
})
// → ["Alice", "Bob"]

// Filter — оставить подходящие
active := lo.Filter(users, func(u User, _ int) bool {
    return u.Active
})
// → [{ID:1 ...}]

// FilterMap — filter + map за один проход
emails := lo.FilterMap(users, func(u User, _ int) (string, bool) {
    if !u.Active {
        return "", false
    }
    return u.Email, true
})

// Reduce — свернуть в одно значение
total := lo.Reduce(orders, func(acc int, o Order, _ int) int {
    return acc + o.Amount
}, 0)

// Find — первый подходящий
user, found := lo.Find(users, func(u User) bool {
    return u.ID == 42
})

// Contains / ContainsBy
lo.Contains([]int{1, 2, 3}, 2)
lo.ContainsBy(users, func(u User) bool {
    return u.Name == "Alice"
})

// Uniq — убрать дубли (порядок сохраняется)
lo.Uniq([]int{1, 2, 2, 3, 1})      // [1, 2, 3]
lo.UniqBy(users, func(u User) int64 {
    return u.TeamID
})

// Flatten — [][]T → []T
lo.Flatten([][]int{{1, 2}, {3, 4}}) // [1, 2, 3, 4]

// Chunk — разбить на батчи по N
batches := lo.Chunk(users, 100)      // [][]User, каждый ≤ 100 элементов

// Reverse
lo.Reverse([]int{1, 2, 3})          // [3, 2, 1]

// First / Last
first, ok := lo.First(users)
last, ok := lo.Last(users)

// IndexOf
idx := lo.IndexOf([]string{"a", "b", "c"}, "b")  // 1
```

## Map операции

```go
m := map[string]int{"a": 1, "b": 2, "c": 3}

// Keys / Values (порядок не гарантирован)
lo.Keys(m)    // ["a", "b", "c"] в произвольном порядке
lo.Values(m)  // [1, 2, 3]

// Entries — пары ключ/значение
lo.Entries(m)  // [{Key:"a" Value:1}, ...]

// FromEntries — собрать map из пар
lo.FromEntries([]lo.Entry[string, int]{
    {Key: "x", Value: 10},
})

// MapKeys / MapValues — трансформировать
lo.MapKeys(m, func(v int, k string) string {
    return strings.ToUpper(k)
})
// → {"A":1, "B":2, "C":3}

// Invert — поменять ключи и значения
lo.Invert(m)  // {1:"a", 2:"b", 3:"c"}

// Pick / OmitBy — взять только нужные ключи
lo.PickBy(m, func(k string, v int) bool {
    return v > 1
})  // {"b":2, "c":3}
```

## GroupBy — частый паттерн

```go
// Сгруппировать по команде
byTeam := lo.GroupBy(users, func(u User) int64 {
    return u.TeamID
})
// → map[int64][]User

// Разделить на два slice по условию
active, inactive := lo.Partition(users, func(u User) bool {
    return u.Active
})
```

## Утилиты

```go
// Ternary — inline if/else
label := lo.Ternary(isAdmin, "admin", "user")

// TernaryF — ленивое вычисление (функции вызываются только когда нужны)
value := lo.TernaryF(condition,
    func() string { return expensiveTrue() },
    func() string { return expensiveFalse() },
)

// Must — panic если error (только для инициализации!)
id := lo.Must(uuid.Parse("550e8400-..."))
config := lo.Must(os.ReadFile("config.json"))

// Must1 / Must2 / Must3 — для функций с несколькими возвратами
// lo.Must аналогичен Must1

// Coalesce — первое не-zero значение
name := lo.CoalesceOrEmpty(user.NickName, user.Name, "Anonymous")

// Empty / IsEmpty
lo.IsEmpty("")         // true
lo.IsEmpty(0)          // true
lo.IsEmpty([]int{})    // true
lo.IsEmpty("hello")    // false
```

## Типичные ошибки

```go
// lo.Must в бизнес-логике — паника вместо ошибки
func getUser(id string) (User, error) {
    uid := lo.Must(uuid.Parse(id))  // плохо — паникует на невалидный id
    // ...
}

// Правильно: lo.Must только в main/init для обязательных ресурсов
func main() {
    templates := lo.Must(template.ParseGlob("templates/*.html"))
    // если шаблонов нет — приложение не должно стартовать
}

// lo.Map не возвращает ошибку — если нужна обработка ошибок, пиши цикл вручную
results, err := lo.MapErr(items, func(item Item, _ int) (Result, error) {
    return process(item)
})
// lo.MapErr остановится на первой ошибке
```

## Когда использовать

- трансформация и фильтрация коллекций без verbosity
- `GroupBy` для batch-обработки и группировки
- `Chunk` для pagination или rate-limited API calls
- `lo.Must` в main() для обязательных инициализаций
- `lo.Ternary` для коротких inline условий в присваиваниях

**Не использовать:**
- `lo.Must` в бизнес-логике и хендлерах
- как замену простого `for range` когда индекс нужен явно

## Interview-ready answer

`samber/lo` — generics-утилиты для коллекций: Map, Filter, GroupBy, Chunk, Uniq, Reduce и другие. До Go 1.18 каждый проект писал эти хелперы сам. `lo` даёт type-safe реализации без reflection. Особенно полезны `GroupBy` для группировки и `Chunk` для батч-обработки. `lo.Must` — только для инициализации, не в бизнес-логике.
