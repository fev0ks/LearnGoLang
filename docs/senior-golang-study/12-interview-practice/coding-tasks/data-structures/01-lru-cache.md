# Задача 1: LRU Cache

## Содержание

- [Формулировка и контракт](#формулировка-и-контракт)
- [Ментальная модель](#ментальная-модель)
- [Почему нужны две структуры данных](#почему-нужны-две-структуры-данных)
- [Пошаговый пример](#пошаговый-пример)
- [Реализация на Go](#реализация-на-go)
- [Самописный двусвязный список](#самописный-двусвязный-список)
- [Конкурентный доступ](#конкурентный-доступ)
- [Тесты](#тесты)
- [Сложность и память](#сложность-и-память)
- [TTL и эксплуатационные расширения](#ttl-и-эксплуатационные-расширения)
- [Реализация LRU с TTL на запись](#реализация-lru-с-ttl-на-запись)
- [LRU и альтернативы](#lru-и-альтернативы)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

LRU (`Least Recently Used`) — cache фиксированной ёмкости, который при
переполнении удаляет запись без недавних обращений. Задача проверяет, умеет ли
разработчик совместить быстрый поиск по ключу с быстрым изменением порядка.

---

## Формулировка и контракт

> Реализуйте LRU cache фиксированной ёмкости. `Get` и `Put` должны работать за
> ожидаемое `O(1)`. Когда места не остаётся, вытесняется наименее недавно
> использованный ключ.

До реализации нужно договориться о семантике:

1. успешный `Get` делает запись наиболее недавно использованной;
2. `Put` существующего ключа обновляет значение и тоже освежает запись;
3. `Put` нового ключа при заполненном cache вытесняет ровно одну запись;
4. `Get` отсутствующего ключа возвращает zero value и `false`;
5. `Delete` удаляет ключ, но не считается вытеснением;
6. неположительная ёмкость является ошибкой конструктора;
7. реализация ниже поддерживает конкурентные вызовы.

TTL не входит в основной контракт. Он меняет смысл `Len`, правила вытеснения и
очистки, поэтому его лучше обсуждать как отдельное расширение — см.
[Реализация LRU с TTL на запись](#реализация-lru-с-ttl-на-запись).

---

## Ментальная модель

У cache есть два представления одних и тех же записей:

```text
items: key -> элемент списка

order:
front                                             back
[C:3] <-> [A:1] <-> [B:2]
  MRU                         LRU, кандидат на удаление
```

Начало списка хранит наиболее недавно использованную запись (`Most Recently
Used`, MRU), конец — наименее недавно использованную (`Least Recently Used`,
LRU).

Хеш-таблица отвечает на вопрос «где запись с таким ключом?». Двусвязный список
отвечает на вопрос «какую запись использовали раньше остальных?».

---

## Почему нужны две структуры данных

Одна хеш-таблица даёт ожидаемое `O(1)` для поиска, но не хранит порядок
обращений. Чтобы найти старейшую запись, пришлось бы сканировать весь cache за
`O(C)`, где `C` — ёмкость.

Один список хранит порядок, но поиск произвольного ключа занимает `O(C)`.

Комбинация устраняет оба линейных прохода:

| Операция | Что используется |
| --- | --- |
| найти ключ | `map[string]*list.Element` |
| сделать запись MRU | `MoveToFront` |
| добавить запись | `PushFront` |
| выбрать LRU | `Back` |
| удалить запись | `Remove` и `delete` |

Двусвязность важна: имея указатель на узел, список удаляет его без поиска
предыдущего элемента. `container/list` уже реализует эту механику, поэтому в
прикладном Go-коде ручные `prev` и `next` обычно не нужны — но у этого списка
поле значения объявлено как `any`, и разбор такого варианта есть в разделе
[Самописный двусвязный список](#самописный-двусвязный-список).

---

## Пошаговый пример

Пусть ёмкость равна двум:

```text
Put(A, 1)
order: [A]

Put(B, 2)
order: [B] <-> [A]

Get(A)
order: [A] <-> [B]

Put(C, 3)
до вставки:        [A] <-> [B]
после PushFront:   [C] <-> [A] <-> [B]
жертва:                              [B]
итог:              [C] <-> [A]
```

Хотя `A` добавили раньше `B`, успешный `Get(A)` обновил порядок. Поэтому при
вставке `C` удаляется `B`.

---

## Реализация на Go

Код ниже обходится без generics: ключ — `string`, значение — структура
`Profile`. Конкретные типы убирают `[K comparable, V any]` из каждой сигнатуры,
поэтому на доске остаётся только логика cache. Обобщение до
`Cache[K comparable, V any]` добавляется заменой двух типов на параметры и
механику не меняет. Type assertion `element.Value.(*entry)` при этом не исчезает:
поле `list.Element.Value` объявлено как `any`.

```go
package lru

import (
    "container/list"
    "errors"
    "sync"
)

var ErrInvalidCapacity = errors.New("lru: capacity must be positive")

// Profile — значение, которое хранит cache.
type Profile struct {
    Name  string
    Email string
}

type entry struct {
    key   string
    value Profile
}

type Cache struct {
    mu       sync.Mutex
    capacity int
    items    map[string]*list.Element
    order    *list.List
}

func New(capacity int) (*Cache, error) {
    if capacity <= 0 {
        return nil, ErrInvalidCapacity
    }

    return &Cache{
        capacity: capacity,
        items:    make(map[string]*list.Element, capacity),
        order:    list.New(),
    }, nil
}

func (c *Cache) Get(key string) (Profile, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()

    element, ok := c.items[key]
    if !ok {
        return Profile{}, false
    }

    c.order.MoveToFront(element)
    item := element.Value.(*entry)
    return item.value, true
}

func (c *Cache) Put(key string, value Profile) {
    c.mu.Lock()
    defer c.mu.Unlock()

    if element, ok := c.items[key]; ok {
        item := element.Value.(*entry)
        item.value = value
        c.order.MoveToFront(element)
        return
    }

    item := &entry{key: key, value: value}
    element := c.order.PushFront(item)
    c.items[key] = element

    if c.order.Len() > c.capacity {
        c.removeElement(c.order.Back())
    }
}

func (c *Cache) Delete(key string) bool {
    c.mu.Lock()
    defer c.mu.Unlock()

    element, ok := c.items[key]
    if !ok {
        return false
    }

    c.removeElement(element)
    return true
}

func (c *Cache) Len() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.order.Len()
}

func (c *Cache) removeElement(element *list.Element) {
    item := element.Value.(*entry)
    c.order.Remove(element)
    delete(c.items, item.key)
}
```

В `items` хранится `*list.Element`, а не копия `entry`: один и тот же объект
должен одновременно участвовать в поиске и порядке. При удалении запись нужно
убрать из обеих структур, иначе `map` продолжит удерживать значение и будет
возвращать элемент, которого уже нет в списке.

`removeElement` не блокирует mutex самостоятельно. Это внутренний метод, который
вызывается только из уже синхронизированных публичных операций. Повторный lock
обычного `sync.Mutex` привёл бы к deadlock.

---

## Самописный двусвязный список

`container/list` хранит полезную нагрузку в поле `Element.Value` типа `any`,
поэтому в каждой операции появляется `element.Value.(*entry)`. Отсюда три
неудобства:

- компилятор не проверяет, что в списке лежат именно `*entry` — несоответствие
  типа станет паникой в рантайме;
- на каждую запись приходится две аллокации: `list.Element` и сама `entry`;
- чтение значения идёт через лишнюю косвенность и мысленную распаковку `any`.

Свой список убирает `any` целиком: узел хранит ключ и значение в типизированных
полях, а `prev` и `next` указывают на такой же `*node`.

### Стражи вместо nil-проверок

Ключевой приём — два фиктивных узла-стража, `head` и `tail`. Они не хранят
данных и никогда не удаляются:

```text
пустой cache:
head[·] <-> [·]tail

после Put(A), Put(B), затем Get(A):
head[·] <-> [A] <-> [B] <-> [·]tail
             MRU     LRU
```

Поскольку стражи есть всегда, у любого реального узла `prev` и `next` заведомо
не `nil`. Это убирает четыре особых случая, на которых обычно и ломается ручной
список: пустой список, единственный узел, удаление первого узла, удаление
последнего. Вставка и удаление превращаются в несколько безусловных присваиваний.

`head.next` — это MRU, `tail.prev` — кандидат на вытеснение. Пустота списка
проверяется как `head.next == tail`.

### Реализация

```go
package lru

import "sync"

// node — узел двусвязного списка с типизированными полями.
// Ни any, ни type assertion: типы проверяет компилятор.
type node struct {
    key   string
    value Profile
    prev  *node
    next  *node
}

// LinkedCache — тот же LRU, но на собственном списке.
type LinkedCache struct {
    mu       sync.Mutex
    capacity int
    items    map[string]*node
    head     *node // страж: head.next — MRU
    tail     *node // страж: tail.prev — LRU
    length   int
}

func NewLinked(capacity int) (*LinkedCache, error) {
    if capacity <= 0 {
        return nil, ErrInvalidCapacity
    }

    head := &node{}
    tail := &node{}
    head.next = tail
    tail.prev = head

    return &LinkedCache{
        capacity: capacity,
        items:    make(map[string]*node, capacity),
        head:     head,
        tail:     tail,
    }, nil
}

func (c *LinkedCache) Get(key string) (Profile, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()

    current, ok := c.items[key]
    if !ok {
        return Profile{}, false
    }

    c.moveToFront(current)
    return current.value, true
}

func (c *LinkedCache) Put(key string, value Profile) {
    c.mu.Lock()
    defer c.mu.Unlock()

    if current, ok := c.items[key]; ok {
        current.value = value
        c.moveToFront(current)
        return
    }

    current := &node{key: key, value: value}
    c.items[key] = current
    c.pushFront(current)

    if c.length > c.capacity {
        c.removeNode(c.tail.prev)
    }
}

func (c *LinkedCache) Delete(key string) bool {
    c.mu.Lock()
    defer c.mu.Unlock()

    current, ok := c.items[key]
    if !ok {
        return false
    }

    c.removeNode(current)
    return true
}

func (c *LinkedCache) Len() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.length
}

// pushFront вставляет узел сразу после стража head.
func (c *LinkedCache) pushFront(current *node) {
    current.prev = c.head
    current.next = c.head.next
    current.prev.next = current
    current.next.prev = current
    c.length++
}

// unlink вырезает узел из списка, не трогая items.
func (c *LinkedCache) unlink(current *node) {
    current.prev.next = current.next
    current.next.prev = current.prev
    current.prev = nil
    current.next = nil
    c.length--
}

func (c *LinkedCache) moveToFront(current *node) {
    c.unlink(current)
    c.pushFront(current)
}

// removeNode удаляет узел из списка и из items.
func (c *LinkedCache) removeNode(current *node) {
    c.unlink(current)
    delete(c.items, current.key)
}
```

### Ссылки, которые легко забыть

Вся ручная часть сосредоточена в двух функциях, и обе переписывают ссылки
парами — в двусвязном списке каждая связь существует в двух направлениях:

```text
unlink(X) для участка A <-> X <-> B:
1. A.next = B      (current.prev.next = current.next)
2. B.prev = A      (current.next.prev = current.prev)

pushFront(X) для участка head <-> F:
1. X.prev = head
2. X.next = F
3. head.next = X   (current.prev.next = current)
4. F.prev = X      (current.next.prev = current)
```

Порядок в `pushFront` важен: сначала узел узнаёт соседей, и только потом соседи
узнают о нём. Если сперва выполнить `head.next = X`, ссылка на прежний первый
узел `F` будет потеряна и её уже негде взять.

Обнуление `current.prev` и `current.next` в `unlink` не требуется для
корректности — список на вырезанный узел уже не ссылается. Оно даёт две
эксплуатационные вещи: удалённый узел не удерживает соседей достижимыми, если
ссылку на него где-то сохранили, а повторный `unlink` того же узла падает с
nil dereference вместо тихого разрушения списка.

`moveToFront` использует ровно те же две функции, поэтому отдельного пути
перемещения, где можно рассинхронизировать связи, не существует.

### Длину приходится считать самому

У `container/list` есть `Len()`, здесь его заменяет поле `length`. Оно меняется
только в `pushFront` и `unlink` — тех же двух функциях, что владеют связями.
Инвариант «`length` равен числу узлов между стражами и равен `len(items)`»
держится в двух местах, а не размазан по `Put`, `Get` и `Delete`.

### Одна проверка на обе реализации

Публичный API совпадает, поэтому обе версии проверяются одним table-driven
тестом через интерфейс:

```go
package lru

import "testing"

// Cacher — общий контракт обеих реализаций.
type Cacher interface {
    Get(key string) (Profile, bool)
    Put(key string, value Profile)
    Delete(key string) bool
    Len() int
}

func TestBothImplementationsEvictEqually(t *testing.T) {
    constructors := map[string]func(int) (Cacher, error){
        "container/list": func(capacity int) (Cacher, error) { return New(capacity) },
        "linked":         func(capacity int) (Cacher, error) { return NewLinked(capacity) },
    }

    for name, newCache := range constructors {
        t.Run(name, func(t *testing.T) {
            cache, err := newCache(2)
            if err != nil {
                t.Fatal(err)
            }

            cache.Put("ann", profile("ann"))
            cache.Put("bob", profile("bob"))
            if _, ok := cache.Get("ann"); !ok {
                t.Fatal("expected ann to exist")
            }

            cache.Put("cid", profile("cid"))

            if _, ok := cache.Get("bob"); ok {
                t.Fatal("bob must be evicted")
            }
            if value, ok := cache.Get("ann"); !ok || value.Name != "ann" {
                t.Fatalf("Get(ann) = (%+v, %t), want ann and true", value, ok)
            }
            if cache.Len() != 2 {
                t.Fatalf("Len() = %d, want 2", cache.Len())
            }
            if !cache.Delete("ann") || cache.Len() != 1 {
                t.Fatalf("Delete(ann) must drop the entry, Len() = %d", cache.Len())
            }
        })
    }
}
```

Такой тест заодно фиксирует, что переход на свой список не менял контракт: если
самописная версия начнёт вытеснять другой ключ, упадёт ровно один subtest.

### Цена и выигрыш

```go
package lru

import (
    "strconv"
    "testing"
)

var benchKeys = func() []string {
    keys := make([]string, 1024)
    for i := range keys {
        keys[i] = strconv.Itoa(i)
    }
    return keys
}()

func BenchmarkContainerList(b *testing.B) {
    cache, err := New(512)
    if err != nil {
        b.Fatal(err)
    }

    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        key := benchKeys[i%len(benchKeys)]
        cache.Put(key, Profile{Name: key})
        cache.Get(key)
    }
}

func BenchmarkLinkedList(b *testing.B) {
    cache, err := NewLinked(512)
    if err != nil {
        b.Fatal(err)
    }

    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        key := benchKeys[i%len(benchKeys)]
        cache.Put(key, Profile{Name: key})
        cache.Get(key)
    }
}
```

```bash
go test -bench List -benchmem -benchtime=3s -count=5
```

Apple M3 Max, go1.26.5, ёмкость 512 при 1024 ключах, то есть почти каждый `Put`
вставляет запись и вытесняет другую:

| Реализация | ns/op (5 прогонов) | B/op | allocs/op |
| --- | --- | --- | --- |
| `container/list` | 86.7–89.7 | 96 | 2 |
| самописный список | 74.1–83.4 | 64 | 1 |

Аллокация ровно одна вместо двух — это и есть исчезнувший `list.Element`. По
времени разница около 10%, и на фоне сетевого вызова, ради которого cache и
существует, эти наносекунды не видны.

| Вопрос | `container/list` | самописный список |
| --- | --- | --- |
| доступ к значению | `element.Value.(*entry)` | `current.value` |
| ошибка типа | паника в рантайме | ошибка компиляции |
| аллокаций на запись | 2 (`Element` и `entry`) | 1 (`node`) |
| длина | `order.Len()` | своё поле `length` |
| граничные случаи | скрыты в stdlib | стражи, но код свой |
| риск | связи ломать нечем | четыре ссылки на операцию |

Практический вывод: `container/list` разумен, когда список — деталь, а не тема
разговора. Свой список стоит писать, когда мешает именно `any`: постоянные type
assertion, отсутствие проверки типов и лишняя аллокация. Смысл выбора в
типизации, а не в наносекундах.

---

## Конкурентный доступ

Успешный `Get` меняет список, поэтому для него нужен эксклюзивный lock. Замена
`sync.Mutex` на `sync.RWMutex` и использование `RLock` в `Get` создаст data race,
несмотря на название метода.

Один mutex задаёт простой и проверяемый контракт, но все операции одного cache
сериализуются. Это становится заметно при большом числе goroutine и горячем
cache. Варианты оптимизации:

- **Sharding —** ключ хешируется в один из независимых LRU; contention
  уменьшается, но вытеснение становится локальным для shard, а не глобальным.
- **Не обновлять порядок на каждом hit —** снижает число записей в список, но
  политика становится приближённой LRU.
- **Отдельный `Peek` —** читает значение без продвижения; семантика отличается
  от `Get` и должна быть видна в API.
- **Готовая библиотека —** уменьшает риск ошибок, но её контракт по TTL,
  callbacks и конкурентности всё равно нужно проверить.

`sync.Map` сам по себе не решает задачу: кроме хеш-таблицы остаётся общий порядок,
который нужно синхронизировать.

---

## Тесты

```go
package lru

import (
    "errors"
    "strconv"
    "sync"
    "testing"
)

func profile(name string) Profile {
    return Profile{Name: name, Email: name + "@example.com"}
}

func TestLRUEvictsLeastRecentlyUsed(t *testing.T) {
    cache, err := New(2)
    if err != nil {
        t.Fatal(err)
    }

    cache.Put("ann", profile("ann"))
    cache.Put("bob", profile("bob"))
    if _, ok := cache.Get("ann"); !ok {
        t.Fatal("expected ann to exist")
    }

    cache.Put("cid", profile("cid"))

    if _, ok := cache.Get("bob"); ok {
        t.Fatal("bob must be evicted")
    }
    if value, ok := cache.Get("ann"); !ok || value.Name != "ann" {
        t.Fatalf("Get(ann) = (%+v, %t), want ann and true", value, ok)
    }
}

func TestLRUUpdateRefreshesEntry(t *testing.T) {
    cache, err := New(2)
    if err != nil {
        t.Fatal(err)
    }

    cache.Put("ann", profile("ann"))
    cache.Put("bob", profile("bob"))
    cache.Put("ann", Profile{Name: "ann", Email: "ann@new.example.com"})
    cache.Put("cid", profile("cid"))

    if _, ok := cache.Get("bob"); ok {
        t.Fatal("bob must be evicted")
    }
    if value, ok := cache.Get("ann"); !ok || value.Email != "ann@new.example.com" {
        t.Fatalf("Get(ann) = (%+v, %t), want the updated email and true", value, ok)
    }
}

func TestLRUDelete(t *testing.T) {
    cache, err := New(2)
    if err != nil {
        t.Fatal(err)
    }

    cache.Put("ann", profile("ann"))
    if !cache.Delete("ann") {
        t.Fatal("Delete(ann) = false, want true")
    }
    if cache.Delete("ann") {
        t.Fatal("second Delete(ann) = true, want false")
    }
    if cache.Len() != 0 {
        t.Fatalf("Len() = %d, want 0", cache.Len())
    }
}

func TestLRURejectsInvalidCapacity(t *testing.T) {
    _, err := New(0)
    if !errors.Is(err, ErrInvalidCapacity) {
        t.Fatalf("New() error = %v, want ErrInvalidCapacity", err)
    }
}

func TestLRUConcurrentAccess(t *testing.T) {
    cache, err := New(32)
    if err != nil {
        t.Fatal(err)
    }

    var workers sync.WaitGroup
    for worker := 0; worker < 8; worker++ {
        workers.Add(1)
        go func() {
            defer workers.Done()
            for key := 0; key < 1_000; key++ {
                name := strconv.Itoa(key % 64)
                cache.Put(name, profile(name))
                cache.Get(name)
            }
        }()
    }
    workers.Wait()

    if cache.Len() > 32 {
        t.Fatalf("Len() = %d, want at most 32", cache.Len())
    }
}
```

Последний тест нужно запускать с `go test -race`. Он проверяет отсутствие data
race, но не доказывает корректность порядка. Поэтому сценарии вытеснения остаются
отдельными детерминированными тестами.

---

## Сложность и память

| Операция | Ожидаемое время | Причина |
| --- | --- | --- |
| `Get` | `O(1)` | lookup в `map` и `MoveToFront` |
| `Put` | `O(1)` | lookup, `PushFront` и не более одного удаления |
| `Delete` | `O(1)` | lookup и удаление известного элемента |
| `Len` | `O(1)` | длина списка |

`O(1)` является ожидаемой оценкой из-за хеш-таблицы Go. Дополнительная память
равна `O(C)`: для каждой логической записи хранятся элемент списка и ссылка из
`map`.

Cache удерживает ключи и значения до удаления или вытеснения. Если значение
содержит большой буфер или граф объектов, весь достижимый граф остаётся живым
для garbage collector. Это не утечка само по себе, а следствие политики
удержания; размер cache в записях не ограничивает его размер в байтах.

---

## TTL и эксплуатационные расширения

TTL (`time to live`) задаёт срок актуальности записи, а LRU — порядок
вытеснения при нехватке места. Эти механизмы дополняют друг друга, но не заменяют.

При добавлении TTL нужно определить:

- считается ли запись истёкшей в момент `now >= expiresAt`;
- обновляет ли `Get` TTL или только LRU-порядок;
- удаляются ли записи лениво при обращении или фоновой goroutine;
- возвращает ли `Len` физическое число записей или только неистёкшие;
- удаляются ли сначала истёкшие записи перед обычным LRU-вытеснением;
- как останавливается фоновая очистка.

Тест с реальным `time.Sleep` медленный и может быть нестабильным. Удобнее
передать cache функцию часов `now func() time.Time`, а в тесте вручную сдвигать
контролируемое время.

Метрики hit, miss, eviction и expiration следует считать раздельно. Callback
вытеснения лучше вызывать после освобождения mutex: медленный пользовательский
код под lock увеличивает latency всех операций, а повторный вызов cache из
callback способен вызвать deadlock.

---

## Реализация LRU с TTL на запись

Вариант ниже фиксирует один конкретный набор ответов на вопросы предыдущего
раздела. В интервью важно не столько выбрать «правильный» вариант, сколько
проговорить выбор вслух.

| Решение | Выбор в коде ниже | Альтернатива |
| --- | --- | --- |
| момент истечения | `now >= expiresAt` | строгое `>` сдвигает границу на наносекунду |
| срок хранится | у каждой записи | один общий TTL на весь cache |
| `Put` существующего ключа | продлевает срок | сохраняет прежний `expiresAt` |
| `Get` (hit) | не продлевает срок | sliding expiration, как в session store |
| истёкшая запись при `Get` | удаляется, `Get` возвращает miss | остаётся до фоновой очистки |
| очистка | ленивая на чтении и хвосте плюс опциональный janitor | только ленивая или только фоновая |
| `Len` | физическое число записей | только неистёкшие, ценой обхода |
| часы | инъекция `Now func() time.Time` | прямой вызов `time.Now` |

Ключевое отличие от базового LRU: причин удаления становится две. `Evictions`
означает нехватку места, `Expirations` — истечение срока, и смешивать их в одной
метрике нельзя: рост первой требует увеличить ёмкость, рост второй — пересмотреть
TTL.

`TTLCache` не расширяет `Cache`, а является отдельным типом: TTL меняет смысл
`Get`, `Len` и вытеснения, поэтому встраивание срока в базовую структуру усложнило
бы её без пользы для случаев, где TTL не нужен. Типы `Profile` и
`ErrInvalidCapacity` берутся из базовой реализации — это тот же пакет.

```go
package lru

import (
    "container/list"
    "sync"
    "time"
)

// ttlEntry дополняет запись моментом истечения.
// Нулевое время означает запись без срока.
type ttlEntry struct {
    key       string
    value     Profile
    expiresAt time.Time
}

func (e *ttlEntry) expired(now time.Time) bool {
    return !e.expiresAt.IsZero() && !now.Before(e.expiresAt)
}

// Config собирает параметры вместо длинного списка аргументов.
type Config struct {
    Capacity        int
    TTL             time.Duration // срок по умолчанию; <= 0 — без срока
    Now             func() time.Time
    CleanupInterval time.Duration // > 0 запускает фоновую очистку
}

type Stats struct {
    Hits        uint64
    Misses      uint64
    Evictions   uint64
    Expirations uint64
}

type TTLCache struct {
    mu       sync.Mutex
    capacity int
    ttl      time.Duration
    now      func() time.Time
    items    map[string]*list.Element
    order    *list.List
    stats    Stats

    stop      chan struct{}
    closeOnce sync.Once
}

func NewTTL(cfg Config) (*TTLCache, error) {
    if cfg.Capacity <= 0 {
        return nil, ErrInvalidCapacity
    }

    now := cfg.Now
    if now == nil {
        now = time.Now
    }

    cache := &TTLCache{
        capacity: cfg.Capacity,
        ttl:      cfg.TTL,
        now:      now,
        items:    make(map[string]*list.Element, cfg.Capacity),
        order:    list.New(),
        stop:     make(chan struct{}),
    }

    if cfg.CleanupInterval > 0 {
        go cache.runCleaner(cfg.CleanupInterval)
    }

    return cache, nil
}

// Put использует TTL по умолчанию из Config.
func (c *TTLCache) Put(key string, value Profile) {
    c.PutTTL(key, value, c.ttl)
}

// PutTTL задаёт срок для конкретной записи.
// ttl <= 0 сохраняет запись без срока, оставляя только LRU-вытеснение.
func (c *TTLCache) PutTTL(key string, value Profile, ttl time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()

    now := c.now()

    var expiresAt time.Time
    if ttl > 0 {
        expiresAt = now.Add(ttl)
    }

    if element, ok := c.items[key]; ok {
        item := element.Value.(*ttlEntry)
        item.value = value
        item.expiresAt = expiresAt
        c.order.MoveToFront(element)
        return
    }

    item := &ttlEntry{key: key, value: value, expiresAt: expiresAt}
    c.items[key] = c.order.PushFront(item)

    c.dropExpiredTail(now)

    if c.order.Len() > c.capacity {
        c.removeElement(c.order.Back())
        c.stats.Evictions++
    }
}

func (c *TTLCache) Get(key string) (Profile, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()

    element, ok := c.items[key]
    if !ok {
        c.stats.Misses++
        return Profile{}, false
    }

    item := element.Value.(*ttlEntry)
    if item.expired(c.now()) {
        c.removeElement(element)
        c.stats.Expirations++
        c.stats.Misses++
        return Profile{}, false
    }

    c.order.MoveToFront(element)
    c.stats.Hits++
    return item.value, true
}

// Len возвращает физическое число записей, включая истёкшие,
// но ещё не удалённые.
func (c *TTLCache) Len() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.order.Len()
}

func (c *TTLCache) Stats() Stats {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.stats
}

// Cleanup удаляет все истёкшие записи за O(C) и возвращает их число.
func (c *TTLCache) Cleanup() int {
    c.mu.Lock()
    defer c.mu.Unlock()

    now := c.now()
    removed := 0

    for element := c.order.Back(); element != nil; {
        prev := element.Prev() // Remove обнуляет связи, поэтому prev нужен заранее

        if element.Value.(*ttlEntry).expired(now) {
            c.removeElement(element)
            c.stats.Expirations++
            removed++
        }

        element = prev
    }

    return removed
}

// Close останавливает фоновую очистку и безопасен при повторном вызове.
func (c *TTLCache) Close() {
    c.closeOnce.Do(func() { close(c.stop) })
}

func (c *TTLCache) runCleaner(interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            c.Cleanup()
        case <-c.stop:
            return
        }
    }
}

// dropExpiredTail снимает истёкшие записи с конца списка.
// Каждая итерация удаляет запись, поэтому стоимость амортизированно
// пропорциональна числу вставок, а не ёмкости.
func (c *TTLCache) dropExpiredTail(now time.Time) {
    for {
        element := c.order.Back()
        if element == nil {
            return
        }

        if !element.Value.(*ttlEntry).expired(now) {
            return
        }

        c.removeElement(element)
        c.stats.Expirations++
    }
}

func (c *TTLCache) removeElement(element *list.Element) {
    item := element.Value.(*ttlEntry)
    c.order.Remove(element)
    delete(c.items, item.key)
}
```

### Почему одной ленивой очистки мало

`dropExpiredTail` смотрит только на конец списка, а порядок задаёт недавность
обращений, а не близость истечения. Запись с TTL в одну секунду, положенная
последней, лежит в начале списка и не попадёт под проверку хвоста. Поэтому
ленивая очистка гарантирует лишь корректность чтения: истёкшее значение никогда
не возвращается наружу. Освобождение памяти она не гарантирует.

```text
случай 1: истёкшие записи стоят в конце
order: [D:live] <-> [C:dead] <-> [B:dead] <-> [A:dead]
                                    dropExpiredTail снимает C, B, A

случай 2: истёкшая запись стоит в середине
order: [D:live] <-> [X:dead] <-> [B:live] <-> [A:live]
                       ^ хвост живой, обход останавливается сразу;
                         X ждёт Cleanup или обращения по ключу
```

| Стратегия | Что даёт | Чего стоит |
| --- | --- | --- |
| проверка на `Get` | наружу не уходит истёкшее значение | память не освобождается без обращений |
| снятие истёкшего хвоста | вытеснение живых записей откладывается | видит только конец списка |
| `Cleanup` по таймеру | освобождает память без обращений | goroutine, `Close` и обход `O(C)` под lock |
| heap по `expiresAt` | точная очистка ближайших истекающих | вторая структура и `O(log C)` на запись |

Выбор между таймером и heap определяется масштабом: при ёмкости в тысячи записей
периодический обход дешевле дополнительной структуры, при десятках тысяч записей
и секундных TTL обход под mutex сам становится источником latency.

### Тесты с управляемыми часами

```go
package lru

import (
    "sync"
    "testing"
    "time"
)

type fakeClock struct {
    mu      sync.Mutex
    current time.Time
}

func (c *fakeClock) Now() time.Time {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.current
}

func (c *fakeClock) Advance(d time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.current = c.current.Add(d)
}

func newTestCache(t *testing.T, capacity int, ttl time.Duration) (*TTLCache, *fakeClock) {
    t.Helper()

    clock := &fakeClock{current: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

    cache, err := NewTTL(Config{
        Capacity: capacity,
        TTL:      ttl,
        Now:      clock.Now,
    })
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(cache.Close)

    return cache, clock
}

func TestTTLExpiresExactlyAtDeadline(t *testing.T) {
    cache, clock := newTestCache(t, 2, time.Minute)

    cache.Put("ann", profile("ann"))

    clock.Advance(time.Minute - time.Nanosecond)
    if value, ok := cache.Get("ann"); !ok || value.Name != "ann" {
        t.Fatalf("Get(ann) = (%+v, %t), want ann and true", value, ok)
    }

    clock.Advance(time.Nanosecond)
    if _, ok := cache.Get("ann"); ok {
        t.Fatal("ann must expire at now >= expiresAt")
    }

    if stats := cache.Stats(); stats.Expirations != 1 || stats.Evictions != 0 {
        t.Fatalf("Stats() = %+v, want 1 expiration and 0 evictions", stats)
    }
}

func TestTTLPerEntryOverridesDefault(t *testing.T) {
    cache, clock := newTestCache(t, 3, time.Minute)

    cache.Put("default", profile("default"))
    cache.PutTTL("short", profile("short"), time.Second)
    cache.PutTTL("forever", profile("forever"), 0)

    clock.Advance(2 * time.Second)

    if _, ok := cache.Get("short"); ok {
        t.Fatal("short must expire after its own ttl")
    }
    if _, ok := cache.Get("default"); !ok {
        t.Fatal("default must survive its ttl window")
    }

    clock.Advance(time.Hour)

    if _, ok := cache.Get("default"); ok {
        t.Fatal("default must expire after the default ttl")
    }
    if value, ok := cache.Get("forever"); !ok || value.Name != "forever" {
        t.Fatalf("Get(forever) = (%+v, %t), want forever and true", value, ok)
    }
}

func TestTTLGetDoesNotExtendDeadline(t *testing.T) {
    cache, clock := newTestCache(t, 2, time.Minute)

    cache.Put("ann", profile("ann"))

    clock.Advance(30 * time.Second)
    if _, ok := cache.Get("ann"); !ok {
        t.Fatal("expected ann to exist")
    }

    clock.Advance(30 * time.Second)
    if _, ok := cache.Get("ann"); ok {
        t.Fatal("hit must refresh LRU order, not the deadline")
    }
}

func TestTTLExpiredTailFreesSpaceBeforeEviction(t *testing.T) {
    cache, clock := newTestCache(t, 2, 0)

    cache.PutTTL("stale", profile("stale"), time.Second)
    cache.PutTTL("live", profile("live"), 0)

    clock.Advance(time.Second)
    cache.PutTTL("new", profile("new"), 0)

    if value, ok := cache.Get("live"); !ok || value.Name != "live" {
        t.Fatalf("Get(live) = (%+v, %t), want live and true: expired tail must be freed first", value, ok)
    }

    stats := cache.Stats()
    if stats.Expirations != 1 || stats.Evictions != 0 {
        t.Fatalf("Stats() = %+v, want 1 expiration and 0 evictions", stats)
    }
}

func TestTTLCleanupRemovesUnreachableEntries(t *testing.T) {
    cache, clock := newTestCache(t, 4, time.Minute)

    cache.Put("ann", profile("ann"))
    cache.Put("bob", profile("bob"))
    cache.PutTTL("cid", profile("cid"), 0)

    clock.Advance(time.Minute)

    if removed := cache.Cleanup(); removed != 2 {
        t.Fatalf("Cleanup() = %d, want 2", removed)
    }
    if cache.Len() != 1 {
        t.Fatalf("Len() = %d, want 1", cache.Len())
    }
    if removed := cache.Cleanup(); removed != 0 {
        t.Fatalf("second Cleanup() = %d, want 0", removed)
    }

    cache.Close()
    cache.Close() // Close идемпотентен
}
```

Тесты не вызывают `time.Sleep`: `fakeClock` двигает время вручную, поэтому
проверка границы `now >= expiresAt` детерминирована. `Advance` берёт mutex,
потому что часы читаются из goroutine фоновой очистки.

Фоновый janitor остаётся исключением: `time.Ticker` работает по реальным часам,
и подменить его тем же `fakeClock` нельзя. Такой код тестируется либо прямым
вызовом `Cleanup`, либо инъекцией интерфейса тикера.

---

## LRU и альтернативы

| Политика | Сигнал | Сильная сторона | Слабая сторона |
| --- | --- | --- | --- |
| LRU | недавность | быстро адаптируется к смене рабочего набора | scan уникальных ключей вытесняет полезные записи |
| LFU | частота | сохраняет стабильно горячие ключи | старая популярность мешает адаптации |
| FIFO | время вставки | простая и дешёвая политика | обращения не защищают запись |
| Random | случайный выбор | мало метаданных | менее предсказуемое качество |
| 2Q / ARC-подобные | несколько историй обращений | лучше разделяют одноразовые и повторные доступы | сложнее контракт и реализация |

Sharded LRU уменьшает конкуренцию за mutex, но меняет результат: глобально
старейшая запись может находиться в другом shard. Это осознанный обмен точности
политики на пропускную способность.

---

## Типичные ошибки

### Get не обновляет порядок

Тогда структура ведёт себя как порядок вставки, а не LRU.

### Удаление только из списка

Запись остаётся достижимой через `map`, удерживает память и может быть найдена
после логического вытеснения.

### Удаление только из map

Узел остаётся в списке. Размер и порядок расходятся с содержимым `items`, а
следующее вытеснение может обратиться к уже несуществующему ключу.

### Ручной список без стражей

Без `head` и `tail` появляются четыре ветки: пустой список, единственный узел,
удаление первого, удаление последнего. Именно в них теряется одна из парных
ссылок, после чего список расходится с `items` и обход в одну сторону перестаёт
совпадать с обходом в другую.

### RLock в Get

Успешный `Get` вызывает `MoveToFront` и является операцией записи во внутреннее
состояние.

### Неопределённая нулевая ёмкость

Молчаливое превращение `0` в `1`, отключённый cache и ошибка — три разных
контракта. Конструктор должен выбрать один и зафиксировать его.

### TTL без очистки и lifecycle

Ленивая проверка не освобождает записи, к которым больше не обращаются. Фоновая
очистка освобождает, но добавляет goroutine, таймер и необходимость `Close`.

### Обещание строгого O1

Операции списка имеют строгую константную стоимость, но lookup в хеш-таблице
даёт ожидаемое `O(1)`. Это различие полезно проговорить.

---

## Interview-ready answer

**1. Как реализовать LRU Cache с ожидаемыми O(1) Get и Put?**

- Структуры — `map` находит запись по ключу, а двусвязный список хранит порядок
  от MRU к LRU.
- Доступ — успешный `Get` перемещает известный узел в начало списка.
- Вставка — новый ключ добавляется в начало, а при переполнении удаляется хвост.
- Согласованность — при удалении запись убирается и из списка, и из `map`.

**2. Почему недостаточно одной map или одного списка?**

- Только `map` — не хранит порядок, поэтому выбор жертвы требует обхода.
- Только список — не даёт быстрого поиска произвольного ключа.
- Комбинация — прямой указатель из `map` на узел списка устраняет оба обхода.

**3. Почему обычному Get нужен эксклюзивный mutex?**

- Причина — hit изменяет порядок через `MoveToFront`.
- Следствие — параллельные hits под `RLock` создают data race.
- Trade-off — один mutex проще и точнее; sharding быстрее, но даёт локальный,
  а не глобальный LRU.

**4. Что меняется при добавлении TTL?**

- Семантика — нужно определить момент expiration и обновление срока.
- Память — ленивая очистка оставляет неиспользуемые истёкшие записи внутри
  cache.
- Lifecycle — фоновая очистка требует остановки и тестируемых часов.
- Метрики — expiration нужно отличать от обычного eviction.

**5. Почему ленивой проверки TTL недостаточно?**

- Гарантия — проверка на `Get` обеспечивает только корректность чтения:
  истёкшее значение не уходит наружу.
- Пробел — порядок списка задаёт недавность, а не близость истечения, поэтому
  истёкшая запись в середине не освобождает память без обращения.
- Дополнение — снятие истёкшего хвоста перед вытеснением сохраняет живые записи,
  периодический `Cleanup` освобождает остальные, heap по `expiresAt` даёт
  точность за счёт второй структуры.
- Проверяемость — TTL тестируется инъекцией `Now func() time.Time`, а не
  `time.Sleep`.

---

## Связанные материалы

- [LFU Cache](./07-lfu-cache.md) — политика на основе частоты обращений.
- [Redis как cache](../../../06-databases/caching/01-redis-as-cache.md) — TTL и
  эксплуатационные политики вытеснения.
- [`container/list`](https://pkg.go.dev/container/list) — двусвязный список
  стандартной библиотеки.
- [Оценка сложности](../../../16-algorithms-and-data-structures/01-time-and-space-complexity.md)
  — ожидаемые и худшие оценки операций.
