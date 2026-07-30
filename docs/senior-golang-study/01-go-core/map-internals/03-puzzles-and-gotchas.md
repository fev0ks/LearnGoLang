# Map: задачи и подводные камни

## Содержание

- [Задача 1: missing key и nil map](#задача-1-missing-key-и-nil-map)
- [Задача 2: значение map не addressable](#задача-2-значение-map-не-addressable)
- [Задача 3: range и модификация](#задача-3-range-и-модификация)
- [Задача 4: присваивание не клонирует map](#задача-4-присваивание-не-клонирует-map)
- [Задача 5: comparable и interface key](#задача-5-comparable-и-interface-key)
- [Задача 6: NaN как key](#задача-6-nan-как-key)
- [Задача 7: сравнение maps](#задача-7-сравнение-maps)
- [Задача 8: concurrent access](#задача-8-concurrent-access)
- [Задача 9: delete, clear и память](#задача-9-delete-clear-и-память)
- [Interview-ready answer](#interview-ready-answer)

Все задачи ниже проверяют семантику языка, а не раскладку Swiss Table: ответы одинаковы и для Go 1.23, и для Go 1.26. Формат — сначала попытка ответить самостоятельно, затем разбор под `<details>`.

---

## Задача 1: missing key и nil map

```go
var m map[string]int

fmt.Println(m["x"])
value, ok := m["x"]
fmt.Println(value, ok, len(m))

delete(m, "x")
clear(m)
m["x"] = 1
```

<details>
<summary>Ответ</summary>

```text
0
0 false 0
panic: assignment to entry in nil map
```

Lookup отсутствующего key возвращает zero value. Comma-ok отличает отсутствие от сохранённого нуля. Для nil map безопасны read, `len`, `range`, `delete` и `clear`; запись требует `make` или map literal.

Удобный idiom для non-nil counter map:

```go
counts[word]++ // missing key читается как 0, затем записывается 1
```

</details>

---

## Задача 2: значение map не addressable

```go
type Point struct{ X, Y int }

m := map[string]Point{"a": {X: 1, Y: 2}}
m["a"].X = 10
```

Что произойдёт и как изменить `X`?

<details>
<summary>Ответ</summary>

Код не компилируется: map index expression нельзя использовать как addressable struct value.

```go
p := m["a"]
p.X = 10
m["a"] = p
```

Либо хранить pointers, если нужны shared mutable objects:

```go
mp := map[string]*Point{"a": {X: 1, Y: 2}}
mp["a"].X = 10
```

Pointer-вариант меняет ownership, nil handling и allocation profile — это design choice, а не автоматический fix.

По той же причине нельзя вызвать pointer receiver method на `map[K]T` element, если для вызова compiler должен взять адрес элемента.

</details>

---

## Задача 3: range и модификация

```go
m := map[int]int{1: 10, 2: 20, 3: 30}

for key := range m {
	if key == 1 {
		delete(m, 2)
		m[4] = 40
	}
}
```

Гарантировано ли посещение keys 2 и 4?

<details>
<summary>Ответ</summary>

- Порядок обхода не specified и не обязан повторяться.
- Если key 2 ещё не был достигнут к моменту `delete`, он не будет произведён итератором.
- Добавленный key 4 может быть посещён в этом range, а может быть пропущен.

Это не undefined behavior: specification явно описывает варианты. Но код, зависящий от выбранного варианта, некорректен.

Изменение map в той же goroutine допустимо. Параллельная запись из другой goroutine — уже data race.

Если нужен стабильный порядок:

```go
keys := make([]int, 0, len(m))
for key := range m {
	keys = append(keys, key)
}
slices.Sort(keys)
```

</details>

---

## Задача 4: присваивание не клонирует map

```go
first := map[string]int{"x": 1}
second := first
second["x"] = 99

fmt.Println(first["x"])
```

<details>
<summary>Ответ</summary>

Будет напечатано `99`. Присваивание копирует небольшое map value, но обе переменные ссылаются на одни underlying entries. Такое поведение часто называют reference semantics, хотя specification не вводит отдельную категорию «reference type».

Независимая shallow copy:

```go
second := maps.Clone(first)
second["x"] = 99

fmt.Println(first["x"]) // 1
```

`maps.Clone` не делает deep copy значений: pointers, slices и вложенные maps внутри values продолжат разделять underlying data.

</details>

---

## Задача 5: comparable и interface key

Какие declarations корректны?

```go
_ = map[[2]int]string{}
_ = map[struct{ X, Y int }]bool{}
_ = map[[]int]string{}

dynamic := map[any]string{}
dynamic[[]int{1, 2}] = "value"
```

<details>
<summary>Ответ</summary>

- Array comparable, если comparable element type.
- Struct comparable, если comparable все fields.
- Slice, map и function не comparable, поэтому `map[[]int]...` не компилируется.
- `any` как static key type допустим, но dynamic slice внутри interface вызывает runtime panic при hashing.

```text
panic: runtime error: hash of unhashable type []int
```

Поэтому `map[any]V` переносит часть проверки из compile time в runtime. Для domain key лучше использовать конкретный comparable type.

</details>

---

## Задача 6: NaN как key

```go
nan := math.NaN()
m := map[float64]string{}

m[nan] = "first"
m[nan] = "second"

fmt.Println(len(m))
fmt.Println(m[nan])
delete(m, nan)
fmt.Println(len(m))
```

<details>
<summary>Ответ</summary>

Типичный результат:

```text
2          <- len(m): обе вставки создали отдельные entries
           <- m[nan]: пустая строка, zero value для string
2          <- len(m) после delete: ничего не удалилось
```

Вторая строка вывода действительно пустая — это не опечатка в примере, а zero value типа `string`, который возвращает промах по ключу.

IEEE-754 задаёт `NaN != NaN`. Поэтому повторная вставка не находит existing key, lookup возвращает zero value, а `delete` тоже не находит equality match.

Удалить такие entries по key нельзя, даже получив NaN key через `range`: он не равен самому себе. Очистить map можно через `clear(m)` или замену новым экземпляром.

Практическое правило: normalise/forbid NaN до использования float как key.

</details>

---

## Задача 7: сравнение maps

```go
left := map[string]int{"x": 1}
right := map[string]int{"x": 1}

fmt.Println(left == right)
```

<details>
<summary>Ответ</summary>

Не компилируется. Map можно сравнить только с `nil`.

Для content equality:

```go
equal := maps.Equal(left, right)
```

Если values требуют custom equality, подходит `maps.EqualFunc`. NaN и values с указателями внутри при этом подчиняются equality выбранной функции, а не `==`.

Следствие: map нельзя использовать как key другой map; struct с map field тоже не comparable.

</details>

---

## Задача 8: concurrent access

```go
m := map[int]int{}

go func() {
	for {
		m[1]++
	}
}()

go func() {
	for {
		_ = m[1]
	}
}()
```

Можно ли рассчитывать на runtime error?

<details>
<summary>Ответ</summary>

Нет. Программа содержит data race и её поведение нельзя использовать как synchronization contract. Runtime часто обнаруживает пересечение и завершает process сообщением вроде:

```text
fatal error: concurrent map read and map write
```

Это runtime throw, а не recoverable panic. Но встроенная проверка не обязана обнаружить каждую race.

Правильные варианты:

- map под `sync.Mutex`/`sync.RWMutex`;
- ownership одной goroutine и commands через channel;
- `sync.Map` для подходящего access pattern;
- immutable snapshot, публикуемый атомарно.

И обязательно:

```bash
go test -race ./...
```

</details>

---

## Задача 9: delete, clear и память

```go
m := make(map[int][]byte, 1_000_000)
for i := range 1_000_000 {
	m[i] = make([]byte, 64)
}

clear(m)
fmt.Println(len(m))
```

Гарантирует ли `len(m)==0`, что выделенная память вернулась ОС?

<details>
<summary>Ответ</summary>

Нет. `clear` удаляет entries и освобождает references на values для GC, но specification ничего не обещает о capacity или RSS. Текущая implementation сохраняет внутренние tables для повторного использования.

Если map пережила редкий большой peak и должна освободить storage:

```go
m = nil
// или
m = make(map[int][]byte)
```

После этого старая map становится eligible for GC, если других references нет. Даже после GC runtime может оставить освобождённые pages себе, поэтому решение проверяют по heap profile и RSS, а не по `len`.

</details>

---

## Interview-ready answer

**1. Что возвращает lookup отсутствующего key?**

- Zero value. Comma-ok нужен, чтобы отличить отсутствие от сохранённого zero. Nil map можно читать, но запись в неё паникует.

**2. Можно ли изменять map во время range?**

- В одной goroutine можно, но семантика специальная: ещё не достигнутый deleted key не появится, а новый key может появиться или нет. Порядок итерации не specified.

**3. Почему нельзя изменить поле `m[key].Field`?**

- Map element не addressable. Нужно получить value copy, изменить и записать обратно либо хранить pointer осознанно.

**4. Какие типы допустимы как keys?**

- Comparable types. Для interface key важен dynamic type: interface со slice/map/function внутри вызовет panic при hashing/comparison.

**5. Безопасна ли builtin map для concurrency?**

- Только если нет concurrent write либо доступ синхронизирован. Runtime fatal — best-effort detection, не механизм защиты; корректность дают happens-before и race-free program.

**6. Что происходит при `second := first` для map?**

- Копируется небольшое map value, а записи остаются общими — запись через `second` видна через `first`. Независимая копия — `maps.Clone`, но она shallow: вложенные slices, maps и объекты за pointer продолжают разделяться.

**7. Почему нельзя написать `left == right` для двух maps?**

- Map сравнивается только с `nil`, полноценного `==` у неё нет. Содержимое сравнивают через `maps.Equal` или `maps.EqualFunc`. Следствие — map нельзя использовать как ключ другой map, а структура с полем-map не comparable.

**8. Что не так с `NaN` в роли ключа?**

- По IEEE-754 `NaN != NaN`, поэтому каждая вставка создаёт новую запись, lookup по тому же значению её не находит, и `delete` тоже. Такие записи убирает только `clear` или замена map. Практическое правило — запрещать или нормализовать NaN до использования float как ключа.

**9. Освобождает ли `delete` или `clear` память map?**

- Записи исчезают и значения перестают удерживаться GC, но capacity и RSS ничего не обещают: текущая реализация сохраняет внутренние tables для переиспользования. После редкого большого пика map пересоздают, а эффект проверяют по heap profile и RSS, а не по `len`.

---

## Официальные источники

- [Go specification: map types](https://go.dev/ref/spec#Map_types)
- [Go specification: range over map](https://go.dev/ref/spec#For_range)
- [Go maps in action](https://go.dev/blog/maps)
- [maps package](https://pkg.go.dev/maps)
