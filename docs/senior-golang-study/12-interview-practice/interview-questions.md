# Возможные вопросы на собеседовании (Interview-ready)

Сводный список **уникальных** вопросов со скринингов (дубли объединены) с короткими ответами «как сказать на собесе» — без глубокого разбора. Где есть профильный материал в `docs/senior-golang-study/`, дана ссылка на файл (раздел назван текстом рядом).

## Содержание

- [Go — язык](#go--язык)
- [Базы данных](#базы-данных)
- [Брокеры сообщений](#брокеры-сообщений)
- [Сеть и HTTP](#сеть-и-http)
- [ОС, процессы, память](#ос-процессы-память)
- [Контейнеры и Kubernetes](#контейнеры-и-kubernetes)
- [Архитектура и практики](#архитектура-и-практики)
- [Инструменты, ORM и эксплуатация](#инструменты-orm-и-эксплуатация)
- [Алгоритмы и структуры данных](#алгоритмы-и-структуры-данных)

---

## Go — язык

**Можно ли передать функцию как параметр в другую функцию?**

- Да. В Go функции — first-class values: их можно передавать аргументом, возвращать, хранить в переменных и полях структур. Тип параметра — сигнатура (`func(int) error`); часто комбинируется с замыканиями. См. [02-value-vs-pointer-semantics.md](../01-go-core/02-value-vs-pointer-semantics.md), раздел про семантику значений.

**Чем отличается слайс от массива?**

- Массив `[N]T` — фиксированная длина, часть типа, копируется целиком по значению. Слайс — дескриптор из трёх полей (указатель на массив, `len`, `cap`), копируется как заголовок и ссылается на общий backing-массив. См. [04-slices.md](../01-go-core/04-slices.md).

**Где выделяется память под новый массив при расширении слайса?**

- При `append` сверх `cap` рантайм аллоцирует новый backing-массив (обычно на куче — escape-анализ почти всегда отправляет его туда), копирует старые элементы и возвращает новый заголовок; старый массив остаётся, пока на него есть ссылки. Рост ёмкости — кратно (примерно ×2 на малых размерах, плавнее на больших). См. [04-slices.md](../01-go-core/04-slices.md) и [memory-internals/02-allocator.md](../01-go-core/memory-internals/02-allocator.md).

**Какая алгоритмическая сложность доступа по ключу для map?**

- Амортизированно O(1): хеш ключа → бакет → сравнение внутри бакета. Худший случай O(n) при массовых коллизиях, но рантайм рандомизирует seed и рехеширует, так что на практике константа. См. [map-internals](../01-go-core/map-internals/README.md).

**Есть ли set в Go? Как сделать?**

- Отдельного `set` в языке нет. Делают через `map[T]struct{}`: ключ — сам элемент (обеспечивает уникальность), значение — `struct{}{}` (нулевой размер, память не тратит). `map[T]bool` тоже подходит, но `struct{}` экономнее. См. [map-internals](../01-go-core/map-internals/README.md).

**Какие бывают виды каналов?**

- По направлению: двунаправленный `chan T`, только-на-приём `<-chan T`, только-на-отправку `chan<- T`. По буферизации: небуферизированный (rendezvous — отправитель ждёт получателя) и буферизированный (`make(chan T, n)` — блокирует при заполнении). См. [concurrency-and-performance/02-goroutines-and-channels.md](../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md).

**Зачем нужен context.Context?**

- Сквозная передача сигнала отмены/дедлайна и request-scoped значений по дереву вызовов и горутин. Главное — кооперативная отмена: при `cancel()`/таймауте закрывается `Done()`, и все слушающие операции (запросы в БД, HTTP, ожидание) прекращаются, не утекая в горутины. См. [concurrency-and-performance/04-context-patterns.md](../01-go-core/concurrency-and-performance/04-context-patterns.md).

**Есть ли исключения в Go?**

- Нет классических try/catch. Ошибки — обычные значения (`error`), возвращаются явно и проверяются `if err != nil`. Для нештатных, фатальных ситуаций есть `panic`/`recover`, но это не механизм бизнес-ошибок, а аварийный путь. См. [05-error-handling.md](../01-go-core/05-error-handling.md).

**Что такое mutex?**

- Примитив взаимного исключения: `sync.Mutex` гарантирует, что критическую секцию в любой момент выполняет одна горутина. `Lock`/`Unlock`; есть `sync.RWMutex` с разделением на множественных читателей и одного писателя. Защищает разделяемое состояние от гонок. См. [concurrency-and-performance/03-sync-primitives.md](../01-go-core/concurrency-and-performance/03-sync-primitives.md).

**Что такое go.mod и go.sum?**

- `go.mod` — манифест модуля: имя модуля (import path), версия Go и список прямых/косвенных зависимостей с их версиями (semver + MVS — minimal version selection). `go.sum` — список криптографических хешей (контрольных сумм) каждой зависимости и её `go.mod` для проверки целостности: при сборке Go сверяет загруженный код с хешем и не даёт незаметно подменить версию. Оба файла коммитятся. См. [service-topologies/03-go-project-layout.md](../04-architecture-and-patterns/service-topologies/03-go-project-layout.md).

**Чем make отличается от new?**

- `new(T)` выделяет память под `T`, обнуляет и возвращает `*T` — указатель на нулевое значение, работает для любого типа. `make` — только для slice/map/chan: не просто аллоцирует, а **инициализирует** внутреннюю структуру (backing-массив слайса, хеш-таблицу map, буфер канала) и возвращает **готовое значение** самого типа (не указатель). Коротко: `new` даёт зануленный указатель, `make` — рабочий slice/map/chan.

**Как работает встроенная функция append? Что происходит с capacity?**

- `append` дописывает элементы в конец слайса. Если `len < cap` — пишет в существующий backing-массив, `cap` не меняется. Если места нет — рантайм аллоцирует новый массив большего размера, копирует старые элементы и возвращает новый заголовок; рост `cap` примерно ×2 на малых слайсах и плавнее (≈×1.25) на больших. Поэтому результат `append` всегда нужно присваивать обратно (`s = append(s, …)`), а при общем backing-массиве возможны неожиданные перезаписи. См. [04-slices.md](../01-go-core/04-slices.md).

**Как работают новые map? Чем отличаются от старой реализации?**

- С Go 1.24 встроенный `map` перешёл на **Swiss Tables**: вместо старой схемы «бакет на 8 пар + цепочка overflow-бакетов» используется открытая адресация группами по 8 слотов с control-байтами (по байту метаданных на слот), что даёт SIMD-сканирование группы, меньше промахов кэша и быстрее lookup/insert при высокой заполненности. Семантика (случайный порядок итерации, запрет конкурентной записи) не изменилась. См. [map-internals/02-swiss-tables-since-1.24.md](../01-go-core/map-internals/02-swiss-tables-since-1.24.md).

**Что произойдёт при чтении из закрытого канала? А при записи?**

- Чтение из закрытого канала не блокируется: сначала отдаются оставшиеся в буфере значения, потом — нулевое значение типа; идиома `v, ok := <-ch` даёт `ok == false`, когда канал закрыт и пуст. Запись в закрытый канал и повторный `close` вызывают **панику**. Поэтому закрывает канал всегда отправитель и только один раз. См. [concurrency-and-performance/02-goroutines-and-channels.md](../01-go-core/concurrency-and-performance/02-goroutines-and-channels.md).

**Чем `interface{}` отличается от `any`?**

- Ничем — `any` это псевдоним (`type any = interface{}`), введённый в Go 1.18 для читаемости. Оба означают «любой тип» (пустой интерфейс без методов). `any` — рекомендуемая запись в новом коде. См. [03-interfaces-method-sets-and-nil.md](../01-go-core/03-interfaces-method-sets-and-nil.md).

**Как реализовать конкурентный доступ к общей переменной без гонок?**

- Варианты: защитить `sync.Mutex`/`RWMutex`; для простых счётчиков/флагов — пакет `sync/atomic` (lock-free CAS); передавать владение через канал («share memory by communicating»); для «один раз инициализировать» — `sync.Once`. Корректность проверяют race-детектором (`go test -race`). Главное правило: к одной переменной нельзя одновременно писать и читать без синхронизации. См. [concurrency-and-performance/03-sync-primitives.md](../01-go-core/concurrency-and-performance/03-sync-primitives.md) и [concurrency-and-performance/01-memory-model.md](../01-go-core/concurrency-and-performance/01-memory-model.md).

**Какие методы у context.Context?**

- Интерфейс из четырёх методов: `Done() <-chan struct{}` (канал, закрывающийся при отмене), `Err() error` (`Canceled`/`DeadlineExceeded` после отмены), `Deadline() (time.Time, bool)` (есть ли дедлайн), `Value(key any) any` (request-scoped значение). Создают через `context.Background()`/`TODO()` и обёртки `WithCancel`/`WithTimeout`/`WithDeadline`/`WithValue`. См. [concurrency-and-performance/04-context-patterns.md](../01-go-core/concurrency-and-performance/04-context-patterns.md).

**В чём разница sync.Mutex и sync.RWMutex? Когда предпочесть RWMutex?**

- `Mutex` даёт эксклюзивный доступ — один владелец и на чтение, и на запись. `RWMutex` различает `RLock` (много читателей одновременно) и `Lock` (один писатель, блокирует всех). RWMutex выгоден при **сильном перекосе в сторону чтения** и нетривиальной критической секции; при частой записи или очень коротких секциях он медленнее обычного `Mutex` из-за большего оверхеда и риска starvation писателя. См. [concurrency-and-performance/03-sync-primitives.md](../01-go-core/concurrency-and-performance/03-sync-primitives.md).

**Как работает сборщик мусора в Go? Какие фазы и как влияют на производительность?**

- Конкурентный трёхцветный mark-and-sweep с write-barrier, работает параллельно с приложением. Фазы: короткий STW на включение барьера → конкурентная маркировка достижимых объектов → короткий STW на завершение маркировки → конкурентная очистка (sweep). Паузы STW обычно суб-миллисекундные; цена — расход CPU на маркировку и барьеры. Частоту запуска регулируют `GOGC` (целевой рост кучи) и `GOMEMLIMIT` (мягкий лимит памяти). Главный рычаг ускорения — снижать аллокации (меньше мусора → реже и дешевле GC). См. [memory-internals/04-garbage-collector.md](../01-go-core/memory-internals/04-garbage-collector.md).

**Как происходит захват внешних переменных в замыканиях?**

- Замыкание захватывает переменную **по ссылке** (а не копию значения): держит указатель на ту же переменную, поэтому видит её изменения и может менять. Если такая переменная переживает кадр стека (замыкание возвращают или запускают в горутине), escape-анализ переносит её на кучу. Классическая ловушка — захват переменной цикла: до Go 1.22 все замыкания делили одну переменную `i`; с 1.22 (loopvar) переменная цикла своя на каждой итерации. См. [02-value-vs-pointer-semantics.md](../01-go-core/02-value-vs-pointer-semantics.md) и [memory-internals/03-escape-analysis.md](../01-go-core/memory-internals/03-escape-analysis.md).

**Какой размер у структуры `Foo{ a int32, b bool }` в байтах?**

- **8 байт.** `int32` — 4 байта, `bool` — 1 байт, итого «полезных» 5. Но выравнивание структуры равно максимальному выравниванию поля (здесь 4 байта у `int32`), поэтому размер округляется вверх до кратного 4 → после `b` добавляется 3 байта padding. Перестановка полей тут не помогает; проверить можно `unsafe.Sizeof(Foo{})`. См. [memory-internals/01-stack-and-heap.md](../01-go-core/memory-internals/01-stack-and-heap.md).

**Можно ли в Go использовать динамические библиотеки?**

- Да, несколькими путями: через **cgo** линковать системные `.so`/`.dll` (C-библиотеки); собрать сам Go-код как разделяемую библиотеку (`go build -buildmode=c-shared`/`c-archive`) для вызова из C/других языков; пакет **`plugin`** (`-buildmode=plugin`) — загрузка `.so` в рантайме с резолвом символов (только Linux/macOS, без Windows, с жёсткими ограничениями на совпадение версий и тулчейна). По умолчанию же Go статически линкует всё в один бинарь — это и есть его «фишка» (простой деплой). На практике динамику используют редко: cgo тянет зависимость от libc и ломает кросс-компиляцию, а `plugin` хрупкий. См. [08-unsafe-and-low-level.md](../01-go-core/08-unsafe-and-low-level.md).

## Базы данных

**Какие бывают виды БД? Плюсы и минусы.**

- **Реляционные** (PostgreSQL, MySQL) — строгая схема, ACID, JOIN, SQL; «+» консистентность и сложные запросы, «−» горизонтальное масштабирование сложнее.
- **Документные** (MongoDB) — гибкая JSON-схема, вложенные объекты; «+» удобно для меняющихся структур и агрегатов, «−» слабее транзакции и нормализация, дублирование данных.
- **Ключ-значение** (Redis, DynamoDB) — доступ по ключу; «+» очень быстро и просто масштабируется, «−» нет сложных запросов и JOIN.
- **Колоночные / wide-column** (ClickHouse, Cassandra) — аналитика и большой объём записи; «+» сжатие и быстрые агрегации по столбцам, «−» плохи для точечных update и OLTP.
- **Графовые** (Neo4j) — связи и обходы; «+» эффективны для графов отношений, «−» нишевые.
- **Поисковые** (Elasticsearch) — полнотекст и фасеты; «+» мощный поиск, «−» вторичное хранилище, не source of truth.
- Выбор — по модели данных, паттерну доступа и требованиям к консистентности/масштабу. См. [database-systems-catalog/01-comparison-table.md](../06-databases/database-systems-catalog/01-comparison-table.md) и [database-fundamentals/02-cap-and-base.md](../06-databases/database-fundamentals/02-cap-and-base.md).

**Зачем нужны индексы? Примеры.**

- Ускоряют поиск/сортировку/джойны, заменяя полный скан таблицы на навигацию по структуре. Виды: B-tree (диапазоны, сортировка, дефолт), Hash (равенство), GIN/инвертированный (jsonb, полнотекст, массивы), GiST/BRIN (геоданные, диапазоны по времени). Плата — замедление записи и место на диске. См. [relational-databases-and-sql/03-indexes-and-query-plans.md](../06-databases/relational-databases-and-sql/03-indexes-and-query-plans.md) и [postgresql/02-indexes.md](../06-databases/database-systems-catalog/postgresql/02-indexes.md).

**Чем hash-индекс отличается от B-tree-индекса?**

- **Hash-индекс** хранит хеши значений и поддерживает только поиск по **точному равенству** (`=`) за O(1); не умеет диапазоны (`<`, `>`, `BETWEEN`), сортировку, `LIKE 'a%'` и сортированный обход. **B-tree** упорядочен, поэтому покрывает равенство, диапазоны, `ORDER BY`, префиксный поиск — за O(log n) и потому является дефолтным индексом. Hash выигрывает лишь в узком случае массивных lookup’ов по равенству; на практике B-tree выбирают почти всегда. См. [relational-databases-and-sql/03-indexes-and-query-plans.md](../06-databases/relational-databases-and-sql/03-indexes-and-query-plans.md) и [postgresql/02-indexes.md](../06-databases/database-systems-catalog/postgresql/02-indexes.md).

**Почему поиск в B-tree быстрее полного перебора? Чем B-tree лучше AVL?**

- **AVL** — самобалансирующееся **бинарное** дерево поиска: у каждого узла максимум 2 ребёнка, для каждого узла высоты левого и правого поддеревьев отличаются не больше чем на 1 (балансировка вращениями при вставке/удалении). За счёт строгого баланса гарантирует O(log n) на поиск — но один узел хранит **один ключ**, поэтому дерево высокое.
- **B-tree** — сбалансированное **многопутевое** дерево: у узла не 2, а сотни-тысячи детей и много ключей в узле, все листья на одной глубине. Тоже O(log n), но основание логарифма большое (fan-out), поэтому дерево очень низкое (2–4 уровня на миллионы строк).
- **Почему B-tree лучше для БД:** узел B-tree совпадает по размеру со страницей диска (обычно 8 КБ), и за одну дисковую I/O-операцию читаются сразу сотни ключей. У БД узкое место — именно random-I/O к диску, а не сравнения в памяти. AVL с одним ключом на узел и большой высотой потребовал бы на порядки больше дисковых обращений. AVL хорош в RAM (например, in-memory индекс), B-tree — когда данные на диске. См. [relational-databases-and-sql/03-indexes-and-query-plans.md](../06-databases/relational-databases-and-sql/03-indexes-and-query-plans.md) и [16-.../05-trees-and-graphs.md](../16-algorithms-and-data-structures/05-trees-and-graphs.md).

**Зачем нужны транзакции? Какие уровни изоляции?**

- Транзакция объединяет операции в атомарную единицу с гарантиями ACID — либо всё применилось, либо ничего. Уровни изоляции (по нарастанию строгости): Read Uncommitted, Read Committed (дефолт PostgreSQL), Repeatable Read, Serializable — каждый отсекает свои аномалии (dirty/non-repeatable read, phantom). См. [relational-databases-and-sql/02-transactions-isolation-and-locks.md](../06-databases/relational-databases-and-sql/02-transactions-isolation-and-locks.md) и [database-fundamentals/01-acid.md](../06-databases/database-fundamentals/01-acid.md).

**В чём отличие WHERE и HAVING?**

- `WHERE` фильтрует строки до группировки (`GROUP BY`) и не работает с агрегатами. `HAVING` фильтрует уже сгруппированные результаты и применяется к агрегатам (`HAVING COUNT(*) > 5`). Порядок: WHERE → GROUP BY → HAVING. См. [relational-databases-and-sql/01-relational-model-and-sql-basics.md](../06-databases/relational-databases-and-sql/01-relational-model-and-sql-basics.md).

**Примеры агрегатных функций. В какой секции их фильтруют?**

- `COUNT`, `SUM`, `AVG`, `MIN`, `MAX`. Они схлопывают группы строк в одно значение; чтобы отфильтровать по результату агрегата, используют секцию `HAVING` (в `WHERE` агрегаты нельзя). См. [relational-databases-and-sql/01-relational-model-and-sql-basics.md](../06-databases/relational-databases-and-sql/01-relational-model-and-sql-basics.md).

**Что такое триггер? Для чего и какие проблемы?**

- Триггер — процедура, автоматически выполняемая БД на событие (INSERT/UPDATE/DELETE) над таблицей. Применяют для аудита, поддержания инвариантов, денормализации. Проблемы: скрытая логика (неочевидна из кода приложения), сложность отладки, влияние на производительность и каскады. Часто лучше выносить логику в приложение.

**В чём разница OLTP и OLAP?**

- OLTP — много коротких транзакций, запись/чтение по ключу, нормализованная схема, низкая латентность (типовой backend). OLAP — аналитика: тяжёлые агрегации по большим объёмам, колоночное хранение, редкая запись пачками (ClickHouse, хранилища). См. [database-fundamentals/03-oltp-vs-olap.md](../06-databases/database-fundamentals/03-oltp-vs-olap.md).

**Что такое репликация БД?**

- Копирование данных с primary-узла на реплики. Даёт отказоустойчивость (failover) и масштабирование чтения. Бывает синхронная (надёжнее, медленнее) и асинхронная (быстрее, риск отставания/lag и чтения устаревших данных). См. [postgresql/06-replication.md](../06-databases/database-systems-catalog/postgresql/06-replication.md).

**Что такое шардирование БД?**

- Горизонтальное разбиение данных на части (шарды) по ключу (hash/range/directory), каждая на своём узле. Масштабирует запись и объём, которые репликация не решает. Платой идут кросс-шардовые запросы, ребалансировка и распределённые транзакции. См. [postgresql/12-sharding.md](../06-databases/database-systems-catalog/postgresql/12-sharding.md).

**Что такое Redis? Плюсы и минусы.**

- In-memory key-value хранилище с богатыми структурами (string, hash, list, set, sorted set, stream, bitmap, HLL); работает в одном потоке по командам, поэтому операции атомарны. Применяют как кэш, счётчики, rate-limiter, очереди, лидерборды, distributed lock, pub/sub.
- **Плюсы:** суб-миллисекундная латентность, удобные структуры данных, атомарные операции и TTL, простота.
- **Минусы:** данные в RAM (дорого и ограничено объёмом), durability компромиссная (RDB-снимки / AOF-лог можно потерять), single-threaded — тяжёлая команда блокирует всех, кластеризация и сильная консистентность нетривиальны.
- См. [database-systems-catalog/08-redis.md](../06-databases/database-systems-catalog/08-redis.md) и [caching/01-redis-as-cache.md](../06-databases/caching/01-redis-as-cache.md).

**SQL — декларативный или императивный язык?**

- Декларативный: описывается **что** нужно получить (какой результат), а не **как** его вычислять пошагово. Способ выполнения (порядок джойнов, индексы, алгоритмы) выбирает планировщик/оптимизатор СУБД. Императивные вставки возможны в процедурных расширениях (PL/pgSQL), но сам SQL — декларативный. См. [relational-databases-and-sql/01-relational-model-and-sql-basics.md](../06-databases/relational-databases-and-sql/01-relational-model-and-sql-basics.md).

**Что такое первичный ключ и какие у него свойства? Может ли он быть составным?**

- Первичный ключ однозначно идентифицирует строку в таблице. Свойства: **уникальность**, **NOT NULL** (не может быть пустым), неизменность (желательно), один на таблицу; СУБД автоматически строит по нему уникальный индекс. Да, ключ может быть **составным** (несколько столбцов) — уникальна тогда комбинация значений (напр. `(order_id, product_id)` в строках заказа). См. [relational-databases-and-sql/01-relational-model-and-sql-basics.md](../06-databases/relational-databases-and-sql/01-relational-model-and-sql-basics.md).

**Что такое нормализация БД?**

- Процесс проектирования схемы по нормальным формам (1NF–3NF/BCNF): данные раскладывают по таблицам так, чтобы убрать избыточность и аномалии вставки/обновления/удаления, а связи выражались через внешние ключи. Каждый факт хранится в одном месте. См. [relational-databases-and-sql/01-relational-model-and-sql-basics.md](../06-databases/relational-databases-and-sql/01-relational-model-and-sql-basics.md).

**Зачем применяют денормализацию и какие у неё минусы?**

- Денормализация — намеренное дублирование/предрасчёт данных (объединение таблиц, кэш-колонки, materialized view) ради ускорения чтения: меньше JOIN и агрегаций на горячем пути. Минусы: избыточность и рост объёма, риск рассинхронизации копий, усложнение записи (надо обновлять в нескольких местах), нагрузка на консистентность. Применяют точечно под конкретный read-паттерн. См. [database-fundamentals/03-oltp-vs-olap.md](../06-databases/database-fundamentals/03-oltp-vs-olap.md).

**Какие инструменты и методы используете для отладки медленных SQL-запросов?**

- `EXPLAIN ANALYZE` — реальный план выполнения (seq scan vs index scan, оценки vs факт, узкие узлы). Поиск проблемных запросов — `pg_stat_statements` (агрегаты по запросам), лог медленных запросов (`log_min_duration_statement`), `auto_explain`. Дальше: добавить/поправить индексы, переписать запрос, убрать N+1, проверить статистику (`ANALYZE`), пагинацию по keyset. См. [postgresql/03-query-planning.md](../06-databases/database-systems-catalog/postgresql/03-query-planning.md) и [postgresql/10-monitoring-and-diagnostics.md](../06-databases/database-systems-catalog/postgresql/10-monitoring-and-diagnostics.md).

## Брокеры сообщений

**Чем Kafka отличается от RabbitMQ?**

- **Kafka** — распределённый durable **лог**: сообщения пишутся в партиции на диск, потребители читают по offset, при чтении сообщение не удаляется (retention по времени/размеру), pull-модель, высокий throughput, возможен реплей и много независимых consumer-групп.
- **RabbitMQ** — классический **брокер очередей** (AMQP): умная маршрутизация через exchange/binding/routing key, push-модель, сообщение удаляется после ack, per-message приоритеты и TTL, гибкие топологии.
- **Когда что:** Kafka — событийные стримы, большой поток, аналитика, реплей; RabbitMQ — task-queue, RPC, сложная маршрутизация и приоритеты. См. [07-message-brokers-and-streaming/01-kafka.md](../07-message-brokers-and-streaming/01-kafka.md), [02-rabbitmq.md](../07-message-brokers-and-streaming/02-rabbitmq.md) и [07-comparison.md](../07-message-brokers-and-streaming/07-comparison.md).

**Какие плюсы и минусы у Kafka?**

- **Плюсы:** очень высокий throughput, горизонтальное масштабирование через партиции, durable-лог с retention и реплеем, гарантия порядка внутри партиции, много независимых consumer-групп, богатая экосистема (Connect, Streams, ksqlDB).
- **Минусы:** операционная сложность (координация — ZooKeeper, в новых версиях KRaft), порядок только в пределах партиции (не глобальный), не подходит для сложной маршрутизации/приоритетов, латентность выше in-memory-решений, избыточен для маленьких нагрузок.
- См. [07-message-brokers-and-streaming/01-kafka.md](../07-message-brokers-and-streaming/01-kafka.md).

## Сеть и HTTP

**В чём отличия TCP и UDP?**

- TCP — с установлением соединения (handshake), надёжный, упорядоченный, с контролем потока и перегрузки; накладные расходы выше. UDP — без соединения, без гарантий доставки и порядка, минимальный оверхед и латентность. TCP — HTTP/БД; UDP — DNS, видео/голос, игры, QUIC. См. [linux/03-tcp-sockets.md](../10-devops-and-observability/linux/03-tcp-sockets.md).

**Какие протоколы прикладного уровня поверх TCP и UDP?**

- Поверх TCP: HTTP/1.1, HTTP/2, HTTPS/TLS, gRPC, SMTP, FTP, БД-протоколы. Поверх UDP: DNS, DHCP, RTP/VoIP, QUIC (и HTTP/3 на его базе). Выбор транспорта диктует требование к надёжности vs латентности.

**Из каких частей состоит HTTP-запрос?**

- Стартовая строка (метод + URL/path + версия, напр. `GET /v1/items HTTP/1.1`), заголовки (`Host`, `Content-Type`, `Authorization`…), пустая строка-разделитель и опциональное тело (body, для POST/PUT). См. [protocols/02-http-server.md](../08-networking-and-api/protocols/02-http-server.md).

**В чём отличия HTTP и HTTPS?**

- HTTPS — это HTTP поверх TLS: трафик шифруется, проверяется подлинность сервера по сертификату и обеспечивается целостность. HTTP передаёт данные открытым текстом. См. [request-lifecycle/03-tcp-tls-and-http-request.md](../08-networking-and-api/request-lifecycle/03-tcp-tls-and-http-request.md).

**Что такое HTTPS и для чего нужен?**

- Защищённая версия HTTP: TLS-handshake устанавливает шифрованный канал, сервер подтверждает identity сертификатом (CA). Нужен для конфиденциальности, защиты от перехвата/MITM и подмены. См. [request-lifecycle/03-tcp-tls-and-http-request.md](../08-networking-and-api/request-lifecycle/03-tcp-tls-and-http-request.md).

**Зачем нужны таймауты в HTTP-запросах и как их подобрать?**

- Чтобы не висеть бесконечно на медленном/мёртвом пире, не копить горутины и соединения, давать быстрый отказ и работать с ретраями. Раздельные таймауты: connect, TLS, response-header, общий. Подбирают от p99 латентности зависимости с запасом, увязывают с дедлайном вызывающего (`context`) и ретраями, чтобы суммарный бюджет не превышал SLA. См. [reliability-patterns/01-timeouts-and-deadlines.md](../05-system-design/reliability-patterns/01-timeouts-and-deadlines.md) и [protocols/03-http-client.md](../08-networking-and-api/protocols/03-http-client.md).

**Зачем нужны HTTP-коды и какие бывают?**

- Стандартный способ сообщить клиенту результат, чтобы он реагировал единообразно (ретрай, ошибка, редирект). Классы: 1xx информационные, 2xx успех (200, 201, 204), 3xx редиректы (301, 304), 4xx ошибка клиента (400, 401, 403, 404, 409, 429), 5xx ошибка сервера (500, 502, 503, 504). См. [api-design/08-errors.md](../08-networking-and-api/api-design/08-errors.md).

**Что такое сокеты и какими они бывают?**

- Сокет — конечная точка сетевого соединения (пара IP:порт) и API ОС для обмена данными. Бывают потоковые (`SOCK_STREAM`, TCP), датаграммные (`SOCK_DGRAM`, UDP) и Unix-domain (локальный IPC между процессами на одной машине). См. [linux/03-tcp-sockets.md](../10-devops-and-observability/linux/03-tcp-sockets.md).

**Что такое NAT?**

- Network Address Translation — подмена IP-адресов и портов на маршрутизаторе: множество хостов из приватной сети (10.x / 192.168.x) выходят в интернет через один публичный IP. Роутер ведёт таблицу трансляций (внутренний IP:порт ↔ внешний порт) и сопоставляет ответы обратно. Экономит дефицитные IPv4 и скрывает внутреннюю топологию, но ломает входящие соединения — для p2p/WebRTC нужны port-forwarding, STUN/TURN. См. [linux/03-tcp-sockets.md](../10-devops-and-observability/linux/03-tcp-sockets.md) и [protocols/09-webrtc.md](../08-networking-and-api/protocols/09-webrtc.md).

## ОС, процессы, память

**Чем отличаются потоки и процессы?**

- Процесс — изолированная единица с собственным адресным пространством и ресурсами. Поток — единица исполнения внутри процесса; потоки одного процесса делят память и дескрипторы, поэтому переключение и обмен дешевле, но нужна синхронизация. См. [hardware-and-os/06-processes-and-threads.md](../10-devops-and-observability/hardware-and-os/06-processes-and-threads.md).

**Какие есть виды межпроцессного взаимодействия (IPC)?**

- Pipes/named pipes, сигналы, разделяемая память (shared memory), очереди сообщений, семафоры, сокеты (в т.ч. Unix-domain), memory-mapped files. Выбор — по объёму данных и границе (одна машина vs сеть). См. [linux/04-signals-and-processes.md](../10-devops-and-observability/linux/04-signals-and-processes.md).

**В чём разница кооперативной и вытесняющей многозадачности?**

- Кооперативная — задача сама уступает управление в точках yield (риск: зависшая задача держит CPU). Вытесняющая — планировщик принудительно прерывает задачу по таймеру/прерыванию, гарантируя справедливость. Планировщик Go исторически был кооперативным, с 1.14 — асинхронно вытесняющий. См. [runtime-scheduler/01-scheduler-and-preemption.md](../01-go-core/runtime-scheduler/01-scheduler-and-preemption.md).

**Что такое переключение контекста?**

- Сохранение состояния (регистры, указатель стека, счётчик команд) текущего потока/процесса и загрузка состояния другого, чтобы CPU переключился между задачами. Стоит недёшево: инвалидация кэшей/TLB, режимные переходы. См. [hardware-and-os/07-context-switching-and-scheduling.md](../10-devops-and-observability/hardware-and-os/07-context-switching-and-scheduling.md).

**Что такое файловый дескриптор и зачем нужен?**

- Целочисленный хендл, который ОС выдаёт процессу на открытый ресурс (файл, сокет, pipe). Через него выполняются read/write. У процесса лимит на число fd (`ulimit -n`); утечка дескрипторов — частая причина «too many open files». См. [linux/02-file-descriptors-and-io.md](../10-devops-and-observability/linux/02-file-descriptors-and-io.md).

**Какой командой убить процесс? А если не умирает?**

- `kill <pid>` шлёт `SIGTERM` (мягко, даёт прибраться). Если процесс игнорирует — `kill -9 <pid>` (`SIGKILL`, безусловное завершение ядром, без shutdown-хуков). Массово — `pkill <name>`/`killall`. См. [linux/04-signals-and-processes.md](../10-devops-and-observability/linux/04-signals-and-processes.md).

**Какой командой проверить потребляемые ресурсы на Unix?**

- `top`/`htop` (CPU/RAM в реальном времени), `ps aux` (снимок по процессам), `free -h` (память), `df -h`/`du` (диск), `iostat`/`vmstat` (I/O и система). См. [linux/06-linux-commands.md](../10-devops-and-observability/linux/06-linux-commands.md).

**Что происходит, когда в коде просим выделить 1 КБ памяти?**

- Сначала аллокатор пытается выдать память из уже зарезервированных у ОС арен/спанов (в Go — per-P кэш `mcache` → `mcentral` → `mheap`), без syscall. Если своей памяти нет — процесс просит у ОС страницы (`mmap`/`brk`), и физическая RAM подключается лениво при первом обращении (page fault). 1 КБ обычно попадает в готовый size-class, syscall не нужен. См. [memory-internals/02-allocator.md](../01-go-core/memory-internals/02-allocator.md) и [linux/01-virtual-memory.md](../10-devops-and-observability/linux/01-virtual-memory.md).

## Контейнеры и Kubernetes

**Что такое pod в Kubernetes?**

- Минимальная единица деплоя: один или несколько контейнеров, делящих сетевой namespace (один IP), тома и жизненный цикл. Обычно один основной контейнер + sidecar-ы. Pod эфемерен — управляется Deployment/ReplicaSet, при падении пересоздаётся. См. [kubernetes/03-pod-vs-container.md](../10-devops-and-observability/kubernetes/03-pod-vs-container.md).

**Для чего нужны Deployment и Service?**

- **Deployment** управляет жизненным циклом stateless-подов через ReplicaSet: держит нужное число реплик, делает rolling-update и rollback, пересоздаёт упавшие поды. **Service** — стабильная точка доступа к набору подов: у подов эфемерные IP, а Service даёт постоянное имя/virtual IP (ClusterIP) и балансирует трафик по живым подам через label-selector. Типы: ClusterIP (внутри кластера), NodePort, LoadBalancer (наружу). См. [kubernetes/02-core-objects-and-deployment-flow.md](../10-devops-and-observability/kubernetes/02-core-objects-and-deployment-flow.md).

**Что такое Ingress?**

- Объект L7-маршрутизации HTTP/HTTPS-трафика снаружи в Service’ы по хосту и пути (`api.example.com/v1 → service-a`), с терминацией TLS в одной точке. Сам по себе Ingress — лишь правила; их исполняет **Ingress-контроллер** (nginx, Traefik и т.п.). Заменяет россыпь LoadBalancer-ов одним входом. См. [kubernetes/02-core-objects-and-deployment-flow.md](../10-devops-and-observability/kubernetes/02-core-objects-and-deployment-flow.md).

**Что такое Docker? Чем отличается от виртуализации?**

- Docker — платформа контейнеризации: упаковывает приложение с зависимостями в образ, запускает как изолированный процесс через namespaces/cgroups, разделяя ядро хоста. ВМ виртуализирует железо и тащит полноценную гостевую ОС с собственным ядром через гипервизор. Контейнеры легче, стартуют за секунды, плотнее; ВМ дают более сильную изоляцию. См. [docker/01-container-vs-virtual-machine.md](../10-devops-and-observability/docker/01-container-vs-virtual-machine.md).

## Архитектура и практики

**Что такое интерфейсы, зачем нужны, чем отличаются от классов?**

- Интерфейс — набор сигнатур методов (контракт поведения) без реализации; даёт полиморфизм и развязку (depend on abstractions). В Go реализация неявная (duck typing — тип соответствует интерфейсу, если имеет методы). В отличие от классов, интерфейс не хранит данные и не задаёт реализацию; в Go нет классов и наследования — есть структуры + интерфейсы + композиция. См. [03-interfaces-method-sets-and-nil.md](../01-go-core/03-interfaces-method-sets-and-nil.md).

**Что такое SOLID?**

- Пять принципов ООП-дизайна: Single Responsibility (одна причина для изменения), Open/Closed (открыт для расширения, закрыт для изменения), Liskov Substitution (подтипы заменяемы), Interface Segregation (узкие интерфейсы), Dependency Inversion (зависеть от абстракций). В Go особенно естественны ISP и DIP через мелкие интерфейсы. См. [patterns/06-solid-in-go.md](../04-architecture-and-patterns/patterns/06-solid-in-go.md).

**Что такое пирамида тестирования?**

- Модель распределения тестов: много быстрых дешёвых unit-тестов в основании, меньше integration-тестов в середине, совсем мало медленных e2e/UI-тестов сверху. Цель — быстрый фидбэк и стабильность, не перегружая верх дорогими хрупкими тестами. См. [09-testing-and-quality/01-testing-strategy.md](../09-testing-and-quality/01-testing-strategy.md).

**Что такое асинхронное API? Зачем и как реализуется?**

- Клиент не ждёт результат в том же запросе: сервер принимает задачу, отвечает `202 Accepted` с id, обрабатывает в фоне, результат отдаёт через polling, callback/webhook или подписку (SSE/WebSocket). Нужно для долгих операций, развязки и сглаживания нагрузки. Реализуется через очередь/брокер и воркеры. См. [external-request-flows/03-write-request-with-queue-and-async-processing.md](../05-system-design/external-request-flows/03-write-request-with-queue-and-async-processing.md).

**Что такое идемпотентность запросов?**

- Повторное выполнение запроса даёт тот же результат, что и однократное, без побочных эффектов-дублей. Критично для ретраев и at-least-once доставки. Реализуют через idempotency-key + дедупликацию на сервере; GET/PUT/DELETE идемпотентны по семантике, POST — нет (нужен ключ). См. [reliability-patterns/06-idempotency.md](../05-system-design/reliability-patterns/06-idempotency.md) и [protocols/07-idempotency.md](../08-networking-and-api/protocols/07-idempotency.md).

**В чём разница устойчивой и неустойчивой сортировки?**

- Устойчивая (stable) сохраняет относительный порядок элементов с равными ключами; неустойчивая может его переставить. Важно при многоключевой сортировке (сначала по одному полю, потом по другому). В Go `sort.Stable`/`slices.SortStableFunc` — устойчивые, `sort.Sort`/`slices.Sort` — нет. См. [02-go-stdlib-and-tools/01-sort-and-slices.md](../02-go-stdlib-and-tools/01-sort-and-slices.md).

## Инструменты, ORM и эксплуатация

> Часть вопросов в скрининге была про личный опыт («какой линтер вам нравится», «с какими ORM работали»). Ниже — нейтральный обзор «что вообще бывает», чтобы было от чего оттолкнуться; на собесе уместно дополнить своим примером.

**Какой линтер для Go и за что?**

- Стандарт де-факто — **golangci-lint**: агрегатор, запускающий десятки линтеров одним проходом с кэшем и конфигом. Под ним: `govet` (подозрительные конструкции), `staticcheck` (богатейший набор проверок и багов), `errcheck` (непроверенные ошибки), `revive` (стиль, замена golint), `gosec` (безопасность), `ineffassign`, `gocritic`. Ценят за единый конфиг в репо, интеграцию в CI и pre-commit, инкрементальность. На собесе достаточно назвать golangci-lint + пару любимых линтеров и за что (например, staticcheck — за глубину анализа).

**Есть ли линтер, проверяющий выравнивание полей в структурах? Как называется?**

- Да — **fieldalignment** из `golang.org/x/tools/go/analysis/passes/fieldalignment` (есть и как отдельная утилита `fieldalignment`, и внутри `govet`/golangci-lint). Он находит структуры, где перестановка полей уменьшит размер за счёт устранения padding (выравнивания), и умеет авто-фиксить порядок. Полезно для «горячих» структур, которых много в памяти. См. [memory-internals/01-stack-and-heap.md](../01-go-core/memory-internals/01-stack-and-heap.md).

**Какие ORM для Go знаете? Какие задачи решает ORM и когда оправдан?**

- Известные: **GORM** (самый популярный full-ORM), **Ent** (схема-как-код, кодогенерация, графовые связи — от Meta), **Bun**, **SQLBoiler** (генерация из схемы). Рядом — не-ORM подходы: `sqlc` (генерация типобезопасного кода из SQL), `sqlx`, query-builder’ы (squirrel, goqu).
- **Задачи ORM:** маппинг строк ↔ структуры, генерация CRUD/SQL, миграции, связи и предзагрузка, транзакции — меньше boilerplate.
- **Когда оправдан:** типовой CRUD, быстрый старт, много однообразных моделей. **Когда нет:** сложные/тяжёлые запросы, жёсткие требования к производительности и плану (ORM генерирует неоптимальный SQL, прячет N+1) — тогда лучше `sqlc`/`pgx` с ручным SQL. В Go-сообществе часто предпочитают явный SQL поверх «магии» ORM. См. [go-database-libraries/05-orm-and-query-builder-options.md](../06-databases/go-database-libraries/05-orm-and-query-builder-options.md) и [go-database-libraries/06-choosing-a-library-for-a-go-service.md](../06-databases/go-database-libraries/06-choosing-a-library-for-a-go-service.md).

**Какие инструменты использовали для поиска утечки памяти?**

- **pprof heap-профиль** (`/debug/pprof/heap`) — сравнение двух снимков во времени (`-base`), `top`/`list`/`-alloc_space` показывают, что и где аллоцирует и не освобождается. **Метрики рантайма** — `runtime.MemStats`/`go_memstats_*` в Grafana: растёт ли `HeapInuse`, число объектов и горутин (утечка горутин = утечка памяти). **goroutine-профиль** — найти зависшие горутины, удерживающие память. Непрерывное профилирование (Pyroscope/Parca) ловит медленный рост в проде. Метод: воспроизвести нагрузку → снять базовый профиль → снять второй спустя время → искать растущую разницу. См. [profiling/03-memory-profiling.md](../01-go-core/profiling/03-memory-profiling.md) и [incident-investigation-and-profiling/03-finding-leaks-contention-and-memory-problems.md](../10-devops-and-observability/incident-investigation-and-profiling/03-finding-leaks-contention-and-memory-problems.md).

**Какие способы мониторинга производительности Go-приложения в проде знаете?**

- **Метрики** — Prometheus + Grafana: RED/USE (latency, RPS, ошибки, ресурсы) и встроенные `go_*`-метрики рантайма (горутины, heap, паузы GC). **Профилирование** — `net/http/pprof` (CPU/heap/goroutine/mutex/block прямо с прода), непрерывное профилирование (Pyroscope/Parca). **Трейсинг** — OpenTelemetry для распределённых запросов. **Рантайм-трейс** — `runtime/trace` для планировщика/латентности. **Логи** — структурный `slog` + сбор (Loki/ELK). См. [profiling/01-pprof-tools-and-workflow.md](../01-go-core/profiling/01-pprof-tools-and-workflow.md), [incident-investigation-and-profiling/02-go-profiling-tracing-and-performance-debugging.md](../10-devops-and-observability/incident-investigation-and-profiling/02-go-profiling-tracing-and-performance-debugging.md) и [prometheus-and-metrics/01-metric-types-and-design.md](../10-devops-and-observability/prometheus-and-metrics/01-metric-types-and-design.md).

## Алгоритмы и структуры данных

**Что такое B-tree, как работает и где применяется?**

- Сбалансированное многопутевое дерево поиска: узел широкий (много ключей), все листья на одной глубине, поиск/вставка/удаление за O(log n). Узел = страница диска, поэтому минимум I/O — отсюда применение в индексах БД и файловых системах. Отличие от бинарных деревьев (AVL, RB) — высокий fan-out и низкая высота под дисковую модель. См. [relational-databases-and-sql/03-indexes-and-query-plans.md](../06-databases/relational-databases-and-sql/03-indexes-and-query-plans.md) и [16-.../05-trees-and-graphs.md](../16-algorithms-and-data-structures/05-trees-and-graphs.md).

**Что такое хеш-таблицы, назначение и ограничения?**

- Структура «ключ → значение» с доступом за амортизированную O(1) через хеш-функцию и бакеты. Назначение — быстрый поиск/вставка/удаление по ключу. Ограничения: нет упорядоченности и диапазонных запросов, деградация при плохом хеше/коллизиях, расходы на рехеш, нестабильная латентность. В Go это встроенный `map`. См. [map-internals](../01-go-core/map-internals/README.md) и [16-.../01-time-and-space-complexity.md](../16-algorithms-and-data-structures/01-time-and-space-complexity.md).

**Что такое динамический массив, назначение и ограничения?**

- Массив с автоматическим расширением ёмкости (в Go — слайс над backing-массивом). Доступ по индексу O(1), `append` в конец — амортизированная O(1). Ограничения: вставка/удаление в середине O(n), при росте — реаллокация и копирование, возможен перерасход памяти из-за запаса `cap`. См. [04-slices.md](../01-go-core/04-slices.md) и [16-.../01-time-and-space-complexity.md](../16-algorithms-and-data-structures/01-time-and-space-complexity.md).

**Что такое куча (heap) и где применяется?**

- Бинарная куча — дерево с heap-свойством (родитель ≤/≥ детей), даёт минимум/максимум за O(1) и вставку/извлечение за O(log n). Применяется в приоритетных очередях, heapsort, поиске top-K, алгоритмах на графах (Дейкстра). В Go — `container/heap`. (Не путать с heap как областью памяти для аллокаций.) См. [16-.../07-sorting-and-heap.md](../16-algorithms-and-data-structures/07-sorting-and-heap.md).
