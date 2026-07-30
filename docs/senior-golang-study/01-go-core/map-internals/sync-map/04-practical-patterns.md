# `sync.Map`: практические приёмы и benchmark

## Содержание

- [Типизированная оболочка](#типизированная-оболочка)
- [Счётчик на ключ](#счётчик-на-ключ)
- [LoadOrStore и дорогая инициализация](#loadorstore-и-дорогая-инициализация)
- [Ленивая инициализация на ключ через sync.Once](#ленивая-инициализация-на-ключ-через-synconce)
- [Когда использовать singleflight](#когда-использовать-singleflight)
- [CAS как переход состояния одного ключа](#cas-как-переход-состояния-одного-ключа)
- [Как сравнивать с map и Mutex](#как-сравнивать-с-map-и-mutex)
- [Минимальный параллельный benchmark](#минимальный-параллельный-benchmark)
- [Какие профили нагрузки проверить отдельно](#какие-профили-нагрузки-проверить-отдельно)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

`sync.Map` полезна не сама по себе, а когда её атомарные операции совпадают с моделью задачи. Эта глава показывает приёмы, в которых разделены два уровня синхронизации:

```text
sync.Map                 -> конкурентный набор ключей
value отдельного ключа   -> собственный atomic, lock, Once или immutable state
```

Публичные гарантии методов разобраны в [первой главе](./01-public-contract.md), а стоимость текущего hash-trie — в [главе про внутреннее устройство](./02-hash-trie-since-1.24.md).

---

## Типизированная оболочка

Generic wrapper возвращает типы, проверяемые при компиляции, хотя внутри всё равно хранится `any`:

```go
type ConcurrentMap[K comparable, V any] struct {
	inner sync.Map
}

func (m *ConcurrentMap[K, V]) Load(key K) (V, bool) {
	value, ok := m.inner.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	return value.(V), true
}

func (m *ConcurrentMap[K, V]) Store(key K, value V) {
	m.inner.Store(key, value)
}

func (m *ConcurrentMap[K, V]) Delete(key K) {
	m.inner.Delete(key)
}
```

Оболочка предотвращает смешивание типов через собственный публичный API, но:

- не добавляет snapshot, `Len` или транзакции над несколькими keys;
- не делает объект за сохранённым указателем потокобезопасным;
- сам не должен копироваться после первого использования;
- теряет защиту типов, если внутреннюю `sync.Map` дополнительно раскрыть наружу.

Если нужен богатый типизированный API с несколькими согласованными операциями, обычная `map[K]V` под блокировкой обычно честнее выражает модель.

---

## Счётчик на ключ

`sync.Map` отвечает за конкурентный набор счётчиков, а каждый `atomic.Int64` — за состояние отдельного счётчика:

```go
var counters sync.Map // map[string]*atomic.Int64

func increment(name string) int64 {
	candidate := &atomic.Int64{}
	actual, _ := counters.LoadOrStore(name, candidate)
	counter := actual.(*atomic.Int64)

	return counter.Add(1)
}
```

Если этот ключ не удаляют и не заменяют другими методами, только один `candidate` становится сохранённым значением. Несколько goroutines могут создать лишние кандидаты, но изменяют `actual`, возвращённый `LoadOrStore`.

Роли разделены:

```text
LoadOrStore(name) -> выбрать единственный counter для key
counter.Add(1)    -> атомарно изменить значение counter
```

Для фиксированного заранее известного набора счётчиков обычная структура с отдельными atomic fields проще. Приём полезен для динамического набора ключей.

Если счётчики удаляются конкурентно, старый указатель может оставаться у goroutine после `Delete`. Тогда нужно определить бизнес-семантику: допустимо ли позднее изменение уже удалённого счётчика и может ли тот же ключ получить новый объект.

---

## LoadOrStore и дорогая инициализация

Такой код не дедуплицирует работу:

```go
candidate := buildExpensiveValue()
actual, loaded := cache.LoadOrStore(key, candidate)
```

Несколько goroutines могут одновременно выполнить `buildExpensiveValue`. `LoadOrStore` атомарно выбирает сохранённое значение, но функция создания выполняется до этой операции.

Если для ключа одновременно выполняются только `LoadOrStore` и чтения, все завершившиеся вызовы получают одно сохранённое значение. `Store`, `Swap`, успешный CAS, `Delete` или `Clear` могут изменить это состояние; глобальной гарантии «один объект навсегда» нет.

---

## Ленивая инициализация на ключ через sync.Once

Дорогую работу можно перенести внутрь дешёвого объекта, который сначала выбирается через `LoadOrStore`:

```go
type lazyValue[T any] struct {
	once  sync.Once
	value T
	err   error
}

var cache sync.Map // map[string]*lazyValue[Config]

func loadConfig(key string) (Config, error) {
	actual, _ := cache.LoadOrStore(key, &lazyValue[Config]{})
	lazy := actual.(*lazyValue[Config])

	lazy.once.Do(func() {
		lazy.value, lazy.err = fetchConfig(key)
	})
	return lazy.value, lazy.err
}
```

Теперь возможны несколько дешёвых выделений `lazyValue`, но `fetchConfig` выполняется один раз у сохранённого объекта.

`sync.Once` кэширует и успешный результат, и ошибку:

```text
первый fetch вернул error
        |
        v
once считается выполненным
        |
        v
следующие вызовы получают ту же ошибку
```

Если неудачную инициализацию нужно повторять, `sync.Once` не подходит без дополнительной модели состояний. Также нужно отдельно продумать eviction: удаление `lazyValue` позволяет последующему вызову создать новый объект и снова выполнить fetch.

---

## Когда использовать singleflight

`singleflight.Group` дедуплицирует только одновременно выполняющиеся вызовы:

```go
value, err, shared := group.Do(key, func() (any, error) {
	return fetchConfig(key)
})
```

После завершения вызова результат не остаётся полноценной записью cache. Поэтому роли различаются:

| Требование | Подход |
| --- | --- |
| Хранить результат между запросами | Cache, возможно на `sync.Map` |
| Объединить только одновременные запросы | `singleflight` |
| Хранить результат и не дублировать cache miss | Cache + `singleflight` |
| Выполнить инициализацию объекта один раз за его lifetime | `sync.Once` внутри value |

При комбинации cache и `singleflight` внутри общей функции обычно повторно проверяют cache: другой путь мог заполнить её между первым miss и фактическим началом fetch.

---

## CAS как переход состояния одного ключа

```go
const (
	Pending = "pending"
	Running = "running"
)

if jobs.CompareAndSwap(jobID, Pending, Running) {
	// Только goroutine, успешно сменившая состояние,
	// получает право запустить job.
}
```

В отличие от `Load` + `Store`, один CAS не оставляет окна между проверкой и изменением:

```text
Load + Store:
goroutine A: Load(Pending)
goroutine B: Load(Pending)
goroutine A: Store(Running)
goroutine B: Store(Running)  -> обе считают себя победителями

CompareAndSwap:
goroutine A: CAS(Pending, Running) -> true
goroutine B: CAS(Pending, Running) -> false
```

Приём подходит, пока всё проверяемое состояние помещается в одно comparable value одного ключа.

Например, можно хранить comparable struct:

```go
type JobState struct {
	Status  string
	Attempt int
}

old := JobState{Status: "pending", Attempt: 1}
next := JobState{Status: "running", Attempt: 1}

if jobs.CompareAndSwap(jobID, old, next) {
	startJob(jobID)
}
```

Если состояние может вернуться из `Running` в `Pending`, возникает ABA-сценарий: отложенная goroutine снова увидит то же значение `Pending` и её CAS может успешно начать уже другой цикл. Для различения циклов в comparable state добавляют поколение или номер попытки и увеличивают его при возврате:

```text
ожидалось: {Status: pending, Attempt: 1}
фактически: {Status: pending, Attempt: 2}

CAS возвращает false, хотя Status снова выглядит одинаково
```

Если вместе нужно атомарно изменить owner, timestamp, несколько индексов или несколько ключей, лучше использовать общую блокировку или другую модель состояния. Указатель в CAS сравнивается по адресу, а не по содержимому объекта.

CAS также не делает внешний side effect exactly-once. Успешная смена `Pending -> Running` и вызов удалённого сервиса остаются двумя отдельными действиями; процесс может упасть между ними.

---

## Как сравнивать с map и Mutex

У `sync.Map` в Go 1.24–1.26 есть путь чтения без mutex узлов и локальные блокировки изменений, но также есть цена:

- ключи и значения проходят через interfaces;
- trie содержит дополнительные nodes и atomic pointers;
- замена значений создаёт новые entries;
- освобождением старых объектов занимается GC;
- сложнее выразить операции над несколькими ключами.

Benchmark должен отражать:

- реальное соотношение reads/writes/deletes;
- один горячий ключ или равномерно распределённые/непересекающиеся ключи;
- число goroutines и доступных CPU cores;
- размер map;
- стоимость hash конкретного типа ключа;
- lifetime и размер значений;
- p95/p99 latency, allocations и GC, а не только средний throughput.

Результат benchmark отвечает только за проверенный профиль нагрузки. Нельзя измерить map только с чтениями из 1024 целочисленных ключей и перенести вывод на churn с удалениями, строковыми ключами и крупными значениями.

---

## Минимальный параллельный benchmark

Пример ниже сравнивает только конкурентные чтения уже заполненной map:

```go
type lockedMap struct {
	mu sync.RWMutex
	m  map[int]int
}

func (m *lockedMap) Load(key int) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.m[key]
	return value, ok
}

func BenchmarkConcurrentReads(b *testing.B) {
	const keys = 1024

	var concurrent sync.Map
	locked := lockedMap{m: make(map[int]int, keys)}
	for key := range keys {
		concurrent.Store(key, key)
		locked.m[key] = key
	}

	b.Run("sync.Map", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			key := 0
			for pb.Next() {
				_, _ = concurrent.Load(key & (keys - 1))
				key++
			}
		})
	})

	b.Run("map+RWMutex", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			key := 0
			for pb.Next() {
				_, _ = locked.Load(key & (keys - 1))
				key++
			}
		})
	})
}
```

Запуск:

```bash
go test -run='^$' -bench=ConcurrentReads -benchmem -cpu=1,4,8 -count=10
```

Несколько запусков сравниваются через `benchstat`, иначе шум легко принять за закономерность.

Ограничения примера:

- нет записей и удалений;
- ключи равномерно переиспользуются;
- значения малы;
- не измеряется latency отдельной операции;
- `keys` является степенью двойки, поэтому выражение `key & (keys - 1)` корректно заменяет modulo.

---

## Какие профили нагрузки проверить отдельно

Минимальный набор:

| Benchmark | Что показывает |
| --- | --- |
| Только чтение, много ключей | Стоимость конкурентного lookup |
| 90% read / 10% write | Смешанную нагрузку |
| Update одного горячего ключа | Максимальную конкуренцию за один parent/value |
| Записи по непересекающимся ключам | Пользу локальных блокировок ветвей |
| Insert/delete churn | Allocations, очистку ветвей и GC |
| `LoadOrStore` с частыми miss | Стоимость создания и публикации entries |
| `Range` параллельно с writes | Стоимость best-effort обхода |
| `Clear` большой map | Время вызова и отложенное освобождение памяти |

Проверять нужно как throughput, так и:

```bash
go test -race ./...
go test -bench=. -benchmem -count=10
```

Для allocation и GC pressure полезны `allocs/op`, heap profile и `gctrace`. Для contention — mutex/block profile и CPU profile. При этом отсутствие времени в mutex profile не означает отсутствие конкуренции за cache lines или atomic operations.

---

## Типичные ошибки

1. **Ожидать, что `LoadOrStore` выполнит функцию создания один раз.** Метод выбирает сохранённое значение, но функция уже могла параллельно выполниться несколько раз.
2. **Игнорировать конкурентный `Delete`.** После удаления тот же ключ может получить другой объект, а старый указатель может остаться у goroutine.
3. **Кэшировать временную ошибку через `sync.Once` без решения.** Ошибка останется результатом на весь lifetime значения.
4. **Считать CAS транзакцией.** Он защищает переход одного comparable value, но не внешний side effect и не несколько ключей.
5. **Сравнивать pointers по содержимому.** CAS для pointers сравнивает адреса.
6. **Переносить чужой benchmark.** Результат зависит от версии Go, CPU, ключей и профиля доступа.
7. **Измерять только средний throughput.** Tail latency, allocations и GC могут изменить выбор.
8. **Копировать typed wrapper после использования.** Внутри него остаётся `sync.Map`, которую нельзя копировать.

---

## Interview-ready answer

**1. Что гарантирует `LoadOrStore`?**

- Гарантия — один вызов атомарно возвращает существующее value либо сохраняет переданное.
- Ограничение — предварительная функция создания может выполниться в нескольких goroutines.
- Изменение ключа — `Store`, `Swap`, успешный CAS, `Delete` или `Clear` могут привести к другому value для последующих вызовов.

**2. Как выполнить дорогую инициализацию один раз на ключ?**

- Шаг 1 — сохранить через `LoadOrStore` дешёвый объект с `sync.Once`.
- Шаг 2 — выполнять дорогую функцию внутри `once.Do` сохранённого объекта.
- Оговорка — `sync.Once` кэширует ошибку; retry требует другой state machine.

**3. Чем `sync.Once` отличается от `singleflight`?**

- `sync.Once` — выполняет функцию один раз за lifetime конкретного объекта.
- `singleflight` — объединяет одновременно выполняющиеся вызовы, но не является постоянной cache.
- Вместе — cache хранит результат, а `singleflight` дедуплицирует одновременный cache miss.

**4. Когда CAS в `sync.Map` уместен?**

- Подходит — переход зависит от одного comparable value одного ключа.
- Не подходит — нужно атомарно изменить несколько ключей или выполнить внешний side effect exactly-once.
- ABA — при возврате к прежнему состоянию нужно добавить поколение или номер попытки.
- Указатель — сравнивается по адресу, а не по содержимому объекта.

**5. Как сравнивать `sync.Map` с map под блокировкой?**

- Нагрузка — воспроизвести реальные чтения, записи, удаления и распределение ключей.
- Среда — проверить нужное число goroutines и CPU cores на используемой версии Go.
- Метрики — сравнивать throughput, p95/p99, allocations и GC.
- Вывод — не переносить результат read-only benchmark на update-heavy систему.

---

## Официальные источники

- [`sync.Map` documentation](https://pkg.go.dev/sync#Map)
- [`sync.Once` documentation](https://pkg.go.dev/sync#Once)
- [`singleflight` documentation](https://pkg.go.dev/golang.org/x/sync/singleflight)
- [Go benchmark package](https://pkg.go.dev/testing#hdr-Benchmarks)
