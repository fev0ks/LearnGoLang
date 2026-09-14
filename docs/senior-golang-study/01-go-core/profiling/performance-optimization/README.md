# Практическая оптимизация Go-кода

Материал превращает идеи из доклада
[«Чтобы код был быстрым, достаточно всего лишь…»](https://www.youtube.com/watch?v=RQ5G-rjbrr0)
в воспроизводимый процесс оптимизации. Ускорения из доклада рассматриваются как
гипотезы, а не универсальные коэффициенты: результат зависит от версии Go,
архитектуры процессора, размеров данных, конкурентности и места функции в полном
request path.

Compiler/runtime details проверены относительно Go 1.27. После обновления
toolchain их нужно перепроверять через diagnostics, benchmark и source code.

## Материалы

| Порядок | Материал | Что внутри |
| --- | --- | --- |
| 1 | [Compiler и измерения](./01-compiler-and-measurement.md) | `-m=2`, assembly, benchmark, PGO, loop unrolling, inlining и fast/slow path |
| 2 | [Аллокации, буферы и pools](./02-allocations-buffers-and-pools.md) | строки, preallocation, caller-provided buffer, `sync.Pool`, size classes и optional values |
| 3 | [Структуры данных и layout](./03-data-structures-and-layout.md) | concurrent map, `copy`, ring buffer, padding, cache locality и false sharing |
| 4 | [Unsafe и сериализация](./04-unsafe-and-serialization.md) | zero-copy, reinterpretation слайсов, Protobuf и FlatBuffers |

---

## Карта приёмов из видео

| Таймкод | Тема | Глава |
| --- | --- | --- |
| [02:46](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=166s) | compiler diagnostics и assembly | [01](./01-compiler-and-measurement.md#диагностика-компилятора) |
| [05:50](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=350s) | строки и precomputation | [02](./02-allocations-buffers-and-pools.md#строки-и-предварительное-выделение-памяти) |
| [07:39](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=459s) | zero-copy `[]byte` и `string` | [04](./04-unsafe-and-serialization.md#zero-copy-byte-и-string) |
| [08:45](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=525s) | reinterpretation слайсов | [04](./04-unsafe-and-serialization.md#reinterpretation-слайса) |
| [09:39](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=579s) | размещение результата | [02](./02-allocations-buffers-and-pools.md#caller-provided-destination) |
| [11:13](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=673s) | `sync.Pool` и pools по размеру | [02](./02-allocations-buffers-and-pools.md#переиспользование-через-syncpool) |
| [15:56](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=956s) | concurrent map | [03](./03-data-structures-and-layout.md#конкурентные-map) |
| [18:40](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=1120s) | optional value без указателя | [02](./02-allocations-buffers-and-pools.md#optional-bool-без-указателя) |
| [20:16](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=1216s) | сдвиг через `copy` | [03](./03-data-structures-and-layout.md#copy-вместо-ручного-сдвига) |
| [21:23](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=1283s) | Duff's device и loop unrolling | [01](./01-compiler-and-measurement.md#ручное-разворачивание-циклов) |
| [26:45](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=1605s) | inlining и fast/slow path | [01](./01-compiler-and-measurement.md#inlining-и-разделение-fastslow-path) |
| [33:33](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=2013s) | alignment, padding и locality | [03](./03-data-structures-and-layout.md#layout-структур-padding-и-cache-locality) |
| [37:40](https://www.youtube.com/watch?v=RQ5G-rjbrr0&t=2260s) | FlatBuffers вместо Protobuf | [04](./04-unsafe-and-serialization.md#protobuf-и-flatbuffers) |

---

## Рабочий цикл оптимизации

Оптимизация начинается с наблюдаемой проблемы: превышенного CPU budget, роста
стоимости инфраструктуры, высокого allocation rate, недостаточного throughput
или выхода p99 latency за SLO.

1. **Зафиксировать требование.** Например: снизить CPU с 3.5 до 2.5 cores при
   20 000 RPS, не ухудшив p99 и RSS.
2. **Снять репрезентативный профиль.** CPU, `alloc_space`, mutex/block profile
   или trace выбираются по симптому.
3. **Найти hot path.** Ускорение функции, занимающей 0.2% CPU, почти не повлияет
   на end-to-end результат.
4. **Сформулировать гипотезу.** Например: `fmt.Sprintf` создаёт лишние
   allocations или глобальный mutex сериализует независимые ключи.
5. **Создать benchmark.** Он воспроизводит размеры данных, hit ratio, число
   goroutines и другие свойства production workload.
6. **Сравнить варианты.** Измеряются `ns/op`, `B/op`, `allocs/op`; для
   конкурентного кода — масштабирование по CPU и latency percentiles.
7. **Проверить полный сервис.** Микробенчмарк не видит сеть, scheduler, GC и
   operational cost.
8. **Сохранить доказательство.** Benchmark, причина необычного кода, версия Go
   и ожидаемый эффект остаются рядом с реализацией.

Главная граница проходит не между «красивым» и «быстрым» кодом, а между
измеренной и предполагаемой пользой.

---

## Уровни риска

- **Базовые приёмы** — preallocation, `append`, `copy`, уменьшение числа
  преобразований и выбор подходящей структуры данных.
- **Оптимизации горячего пути** — caller-provided buffer, `sync.Pool`, sharded
  map, выделение fast path и изменение layout массовых структур.
- **Низкоуровневые приёмы** — ручной loop unrolling, `unsafe`, zero-copy и
  смена формата сериализации.

Чем выше цена ошибки и поддержки, тем сильнее требуется end-to-end
доказательство, а не только микробенчмарк.

---

## Таблица выбора

| Наблюдение | Сначала попробовать | Когда идти глубже |
| --- | --- | --- |
| `fmt` заметен в CPU/alloc profile | `strings.Builder`, `append`, `strconv.Append*` | custom encoding и precomputation |
| много временных buffers | caller-provided buffer, preallocation | `sync.Pool`, затем size-class pools |
| contention на одной map | сократить lock scope, проверить `Mutex` | `sync.Map` или sharding под access pattern |
| миллионы маленьких pointers | значения и explicit presence | compact representation после измерений |
| горячий сдвиг slice | `copy` | ring buffer или другая структура данных |
| CPU hotspot в простом цикле | проверить алгоритм и bounds checks | unrolling, SIMD или assembly |
| fast path не inline-ится | вынести cold code | PGO и specialization |
| высокий RSS у `[]T` | переставить поля, убрать pointers | split hot/cold fields, custom layout |
| копирование bytes доминирует | reuse destination buffer | локальный unsafe zero-copy |
| serialization доминирует | tuning текущего codec и reuse | другой wire format с end-to-end benchmark |

---

## Порядок проверки результата

```bash
# До изменения
go test -run='^$' -bench=BenchmarkHot -benchmem -count=10 ./internal/codec \
    > before.txt

# После изменения
go test -run='^$' -bench=BenchmarkHot -benchmem -count=10 ./internal/codec \
    > after.txt

benchstat before.txt after.txt
```

После локального сравнения проверяются CPU на единицу нагрузки, allocation rate,
GC CPU, RSS, throughput и p95/p99. Успешное уменьшение `ns/op` не компенсирует
ухудшение метрики, ради которой началась работа.

---

## Связанные материалы

- [Benchmarks](../06-benchmarks.md)
- [CPU Profiling](../02-cpu-profiling.md)
- [Memory Profiling](../03-memory-profiling.md)
- [Escape Analysis](../../memory-internals/03-escape-analysis.md)
- [Strings](../../07-strings.md)
- [Unsafe And Low-Level](../../08-unsafe-and-low-level.md)
- [`sync.Map`](../../map-internals/sync-map/README.md)

---

## Interview-ready answer

**1. Как выглядит правильный процесс оптимизации Go-сервиса?**

- Требование — сначала фиксирую SLO или resource budget: CPU, throughput, p99,
  RSS или allocation rate.
- Профиль — нахожу hot path через CPU, alloc, mutex/block profile или trace.
- Гипотеза — связываю наблюдение с конкретной причиной, а не переписываю код
  вслепую.
- Benchmark — воспроизвожу production sizes, access pattern и concurrency,
  сравниваю несколько запусков через `benchstat`.
- Проверка — подтверждаю эффект в полном сервисе и смотрю побочные метрики.
- Поддержка — сохраняю benchmark, инварианты и причину необычного решения рядом
  с кодом.

**2. Когда стоит использовать sync.Pool?**

- Назначение — временные объекты часто создаются параллельными независимыми
  операциями, а reuse снижает GC pressure.
- Контракт — runtime может удалить объект в любой момент; `Pool` не является
  надёжным cache.
- Безопасность — объект сбрасывается перед reuse и не используется после `Put`.
- Память — большие backing arrays не возвращаются в pool либо разделяются по
  size classes.
- Доказательство — alloc profile и benchmark показывают выигрыш полного hot
  path.

**3. Почему inlining ускоряет код сильнее, чем только экономия call/return?**

- Контекст — compiler видит caller и callee как один участок.
- Оптимизации — становятся возможны constant propagation, devirtualization,
  удаление branches и bounds checks.
- Память — более точный escape analysis может оставить значение на стеке.
- Ограничение — решение зависит от inline budget, PGO и версии compiler.

**4. Когда unsafe zero-copy оправдан?**

- Hot path — копирование подтверждено профилем как значимая часть стоимости.
- Ownership — известны владелец памяти, срок жизни view и запрет мутации.
- Изоляция — `unsafe` находится в маленьком helper с безопасным внешним API.
- Проверка — есть benchmark, tests, `go vet`, race/checkptr runs и постепенный
  rollout.
- Альтернатива — caller-provided buffer, preallocation и обычная копия уже
  рассмотрены.

**5. Как выбирать между map под lock, sync.Map и sharded map?**

- `map + Mutex` — default для понятных mixed operations и общих инвариантов.
- `RWMutex` — кандидат при конкурентных чтениях и редких записях, но только
  после benchmark против обычного `Mutex`.
- `sync.Map` — специализированный вариант для write-once/read-many или
  независимых ключей.
- Sharding — помогает при распределённом contention, но добавляет hash и
  сложность multi-key operations.
- Benchmark — повторяет read/write ratio, hit rate, hot keys и число goroutines.

---

## Источники

- [Видео: «Чтобы код был быстрым, достаточно всего лишь…»](https://www.youtube.com/watch?v=RQ5G-rjbrr0)
- [Go Diagnostics](https://go.dev/doc/diagnostics)
- [Profile-guided optimization](https://go.dev/doc/pgo)
- [Go 1.27 Release Notes](https://go.dev/doc/go1.27)
- [`sync.Pool` documentation](https://pkg.go.dev/sync#Pool)
- [`unsafe` package documentation](https://pkg.go.dev/unsafe)
- [FlatBuffers: Go guide](https://flatbuffers.dev/languages/go/)
- [Protocol Buffers: Encoding](https://protobuf.dev/programming-guides/encoding/)
