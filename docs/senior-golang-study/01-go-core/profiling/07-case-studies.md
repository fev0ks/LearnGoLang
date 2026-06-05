# Case Studies: разбор реальных сценариев

Пять типичных проблем производительности. Каждый кейс: симптом → как диагностировать → корень причины → fix → результат.

---

## Кейс 1: Высокий CPU — regexp в горячем цикле

### Симптом

- CPU usage 85% при ~500 RPS — кажется много для такой нагрузки
- Latency нормальная: p50 = 8ms, p99 = 45ms
- CPU растёт пропорционально RPS

### Диагностика

```bash
# Собрать CPU профиль под нагрузкой
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/profile?seconds=30"
```

Flamegraph показывает широкое плато:

```
handleRequest (95% cum)
  └─ validateFields (88% cum)
       └─ regexp.(*Regexp).FindString (71% flat!)  ← вот оно
       └─ regexp.Compile (15% flat!)               ← и это
```

```
(pprof) list validateFields

     0.11s      8.23s (flat, cum) 88.00% of Total
         .          .     34:func validateFields(fields []Field) bool {
     0.11s      8.23s     35:    for _, f := range fields {
         .      6.87s     36:        re := regexp.MustCompile(f.Pattern)  // ← компиляция КАЖДЫЙ вызов
         .      1.36s     37:        if !re.MatchString(f.Value) {
         .          .     38:            return false
         .          .     39:        }
         .          .     40:    }
         .          .     41:    return true
         .          .     42:}
```

### Корень причины

`regexp.MustCompile` вызывается при каждом запросе для каждого поля. `Compile` парсит regex-паттерн и строит NFA/DFA — это дорогая операция (~микросекунды). При 500 RPS × 20 полей = 10 000 компиляций в секунду.

### Fix

```go
// ❌ было
func validateFields(fields []Field) bool {
    for _, f := range fields {
        re := regexp.MustCompile(f.Pattern)
        if !re.MatchString(f.Value) {
            return false
        }
    }
    return true
}

// ✅ стало: компилировать один раз при старте
type FieldValidator struct {
    patterns map[string]*regexp.Regexp
}

func NewFieldValidator(patterns map[string]string) *FieldValidator {
    compiled := make(map[string]*regexp.Regexp, len(patterns))
    for name, pattern := range patterns {
        compiled[name] = regexp.MustCompile(pattern)
    }
    return &FieldValidator{patterns: compiled}
}

func (v *FieldValidator) Validate(fields []Field) bool {
    for _, f := range fields {
        re, ok := v.patterns[f.Name]
        if !ok { continue }
        if !re.MatchString(f.Value) { return false }
    }
    return true
}
```

### Результат

```
Benchmark         Before          After       Delta
ValidateFields    1823 ns/op      34 ns/op    -98.1%
                    312 B/op       0 B/op     -100%
                      7 allocs/op  0 allocs/op -100%
```

CPU usage: 85% → 12% при той же нагрузке.

---

## Кейс 2: Memory leak — unbounded cache

### Симптом

- RSS процесса растёт на ~50MB в час
- После 8 часов работы: RSS = 3GB, OOM killer останавливает pod
- Перезапуск помогает, но ненадолго
- CPU нормальный, метрики запросов нормальные

### Диагностика

```bash
# Профиль при запуске
curl -o heap_1h.prof "http://localhost:6060/debug/pprof/heap"
# ... подождать час ...
curl -o heap_2h.prof "http://localhost:6060/debug/pprof/heap"

# Сравнить дельту
go tool pprof -inuse_space -diff_base heap_1h.prof -http=:6061 heap_2h.prof
```

```
(pprof) top
      flat  flat%   sum%        cum   cum%
   48.23MB 96.45% 96.45%    48.23MB 96.45%  main.(*UserService).GetUser
```

```
(pprof) list GetUser

    48.23MB    48.23MB (flat, cum) 96.45% of Total
          .          .    67:func (s *UserService) GetUser(ctx context.Context, id int64) (*User, error) {
    48.23MB    48.23MB    68:    if user, ok := s.cache[id]; ok {  // ← здесь держится
          .          .    69:        return user, nil
          .          .    70:    }
```

### Корень причины

```go
type UserService struct {
    db    *pgxpool.Pool
    cache map[int64]*User  // map без ограничения размера!
}

func (s *UserService) GetUser(ctx context.Context, id int64) (*User, error) {
    if user, ok := s.cache[id]; ok {
        return user, nil
    }
    user, err := s.db.QueryRow(ctx, "SELECT * FROM users WHERE id = $1", id)
    if err != nil { return nil, err }
    s.cache[id] = user  // добавляется, никогда не удаляется
    return user, nil
}
```

За 8 часов в систему пришли запросы по ~600k уникальным ID → 600k записей в map → 3GB.

### Fix

```go
import lru "github.com/hashicorp/golang-lru/v2"

type UserService struct {
    db    *pgxpool.Pool
    cache *lru.Cache[int64, *User]  // LRU с ограничением
}

func NewUserService(db *pgxpool.Pool) *UserService {
    cache, _ := lru.New[int64, *User](10_000)  // максимум 10k пользователей
    return &UserService{db: db, cache: cache}
}

func (s *UserService) GetUser(ctx context.Context, id int64) (*User, error) {
    if user, ok := s.cache.Get(id); ok {
        return user, nil
    }
    user, err := fetchFromDB(ctx, s.db, id)
    if err != nil { return nil, err }
    s.cache.Add(id, user)  // при достижении лимита вытесняет старые
    return user, nil
}
```

**Альтернатива с TTL:**
```go
import "github.com/patrickmn/go-cache"

c := cache.New(5*time.Minute, 10*time.Minute)  // TTL=5min, GC каждые 10min
c.Set(key, value, cache.DefaultExpiration)
```

### Результат

RSS стабилизировался на ~200MB и больше не растёт. Через 24 часа — те же 200MB.

---

## Кейс 3: Goroutine leak — worker без context

### Симптом

- `runtime.NumGoroutine()` растёт с каждым деплоем на ~100 горутин
- После 48 часов: 50k горутин
- Память растёт (50k × ~2KB стека = ~100MB)
- При нагрузке CPU спайки — много горутин конкурируют

### Диагностика

```bash
# Дамп горутин
curl "http://localhost:6060/debug/pprof/goroutine?debug=1" > goroutines.txt

head -50 goroutines.txt
```

```
847 @ 0x43b0c8 0x40ca7b 0x40c894 0x6d8a12 0x6d8a9e 0x468fc1
#   0x6d8a11  main.(*EventProcessor).processLoop+0x71  /app/processor.go:54
#   0x6d8a9e  main.startProcessor+0x5e                 /app/processor.go:23

847 горутин на одном стеке!  ← утечка
```

```
(pprof) list processLoop
```

```go
// /app/processor.go:54
func (p *EventProcessor) processLoop() {
    for {
        select {
        case event := <-p.events:
            p.handle(event)
        }
        // ← нет case <-ctx.Done()!
    }
}

// /app/processor.go:23
func startProcessor(events <-chan Event) {
    proc := &EventProcessor{events: events}
    go proc.processLoop()  // горутина никогда не завершится
}
```

При каждом деплое в Kubernetes: старый pod получает SIGTERM, новый стартует. Но старые горутины в `processLoop` не завершались — они ждали на `p.events` который закрылся. Нет, они не ждали — они были в `select` без `ctx.Done()`.

На самом деле: `startProcessor` вызывается в `init()` при каждой переинициализации компонента — и старые горутины остаются.

### Fix

```go
// ✅ С context
type EventProcessor struct {
    events <-chan Event
    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
}

func NewEventProcessor(parentCtx context.Context, events <-chan Event) *EventProcessor {
    ctx, cancel := context.WithCancel(parentCtx)
    p := &EventProcessor{
        events: events,
        ctx:    ctx,
        cancel: cancel,
    }
    p.wg.Add(1)
    go p.processLoop()
    return p
}

func (p *EventProcessor) processLoop() {
    defer p.wg.Done()
    for {
        select {
        case event, ok := <-p.events:
            if !ok { return }  // канал закрыт — выйти
            p.handle(event)
        case <-p.ctx.Done():
            return  // отмена — выйти
        }
    }
}

func (p *EventProcessor) Stop() {
    p.cancel()
    p.wg.Wait()  // дождаться завершения
}
```

```go
// В тестах — проверяем что горутин не остаётся
import "go.uber.org/goleak"

func TestEventProcessor(t *testing.T) {
    defer goleak.VerifyNone(t)

    ctx, cancel := context.WithCancel(context.Background())
    events := make(chan Event)
    proc := NewEventProcessor(ctx, events)

    // ... тест ...

    cancel()        // сигнал завершения
    proc.Stop()     // дождаться
    close(events)
}
```

### Результат

После fix: при деплое горутины процессоров корректно завершаются → NumGoroutine стабилен.

---

## Кейс 4: Lock contention — глобальный mutex при высокой конкурентности

### Симптом

- При RPS < 200: p99 = 15ms, CPU нормальный
- При RPS > 500: p99 → 2000ms, CPU не высокий (~30%)
- Throughput не масштабируется — добавление pods не помогает (проблема внутри одного инстанса)

### Диагностика

CPU профиль: функция `getMetric` с небольшим flat, но `sync.(*RWMutex).RLock` в топе.

```bash
# Включить mutex профиль
# (в коде уже должно быть: runtime.SetMutexProfileFraction(10))
go tool pprof -http=:6061 "http://localhost:6060/debug/pprof/mutex"
```

```
(pprof) top
      flat  flat%   sum%        cum   cum%
     1.23s 89.13% 89.13%      1.23s 89.13%  sync.(*RWMutex).RLock   ← почти всё время
```

```
(pprof) list getMetric
...
   func (r *MetricsRegistry) getMetric(name string) *Metric {
       r.mu.RLock()
       defer r.mu.RUnlock()        ← ← вот здесь contention
       return r.metrics[name]
   }
```

### Корень причины

```go
type MetricsRegistry struct {
    mu      sync.RWMutex
    metrics map[string]*Metric
}

func (r *MetricsRegistry) Record(name string, value float64) {
    metric := r.getMetric(name)
    if metric == nil {
        r.addMetric(name)  // Write lock
        metric = r.getMetric(name)
    }
    metric.Add(value)
}
```

При 500 RPS × 50 метрик на запрос = 25 000 вызовов `getMetric` в секунду. Все они берут `RLock` — при большом числе горутин это становится узким местом, так как `RLock` всё равно требует атомарную операцию на счётчик.

### Fix: sync.Map для read-heavy, редко пишем

```go
// ✅ sync.Map: оптимизирована для read-heavy workloads
type MetricsRegistry struct {
    metrics sync.Map  // map[string]*Metric
}

func (r *MetricsRegistry) getOrCreate(name string) *Metric {
    // LoadOrStore — атомарная операция без global lock
    actual, _ := r.metrics.LoadOrStore(name, &Metric{name: name})
    return actual.(*Metric)
}

func (r *MetricsRegistry) Record(name string, value float64) {
    r.getOrCreate(name).Add(value)
}
```

**Альтернатива для более сложных случаев: sharded map**

```go
const numShards = 256

type ShardedMap struct {
    shards [numShards]struct {
        mu   sync.RWMutex
        data map[string]*Metric
        _    [40]byte  // padding до cache line (64 bytes)
    }
}

func (m *ShardedMap) shard(key string) int {
    h := fnv.New32()
    h.Write([]byte(key))
    return int(h.Sum32()) % numShards
}

func (m *ShardedMap) Get(key string) (*Metric, bool) {
    s := m.shard(key)
    m.shards[s].mu.RLock()
    defer m.shards[s].mu.RUnlock()
    v, ok := m.shards[s].data[key]
    return v, ok
}
```

### Результат

```
RPS     p99 before    p99 after
200     15ms          14ms     (без изменений при низкой нагрузке)
500     2000ms        18ms     (100x улучшение при высокой нагрузке)
1000    timeout       22ms
```

---

## Кейс 5: GC pressure — тысячи мелких объектов в секунду

### Симптом

- CPU 45% при 300 RPS — много
- В pprof: `runtime.mallocgc` = 25% flat CPU
- `GODEBUG=gctrace=1` показывает GC каждые 200ms
- `gc 42 @30s: 8% CPU on gc` — 8% времени на GC

### Диагностика

```bash
go tool pprof -alloc_space -http=:6061 "http://localhost:6060/debug/pprof/allocs"
```

```
(pprof) top
      flat  flat%   sum%        cum   cum%
     8.91GB 45.23% 45.23%      8.91GB 45.23%  encoding/json.Marshal    ← 8.91 GB total allocs!
     4.23GB 21.45% 66.68%      4.23GB 21.45%  fmt.Sprintf
     2.11GB 10.71% 77.39%      2.11GB 10.71%  strings.(*Builder).grow
```

Сервис работал 2 часа и суммарно выделил 15GB памяти — при реальном инстансе в 512MB. GC убирает за собой, но постоянно.

```
(pprof) list buildResponse

         .    8.91GB (flat, cum) 45.23% of Total
         .          .     67:func buildResponse(items []Item) []byte {
    8.91GB    8.91GB     68:    data, _ := json.Marshal(items)  // новый буфер каждый раз
         .          .     69:    return data
         .          .     70:}
```

### Корень причины

На каждый из 300 запросов в секунду: JSON marshal создаёт несколько временных буферов для каждого из 50+ items. Это short-lived объекты — GC их убирает, но цена — паузы каждые 200ms.

### Fix: sync.Pool + bytes.Buffer

```go
// ❌ было: новый буфер каждый раз
func buildResponse(items []Item) []byte {
    data, _ := json.Marshal(items)
    return data
}

// ✅ стало: переиспользуем буфер через pool
var jsonBufPool = sync.Pool{
    New: func() interface{} { return new(bytes.Buffer) },
}

func buildResponse(items []Item) []byte {
    buf := jsonBufPool.Get().(*bytes.Buffer)
    buf.Reset()
    defer jsonBufPool.Put(buf)

    enc := json.NewEncoder(buf)
    if err := enc.Encode(items); err != nil {
        return nil
    }
    // Копируем результат — buf вернётся в pool
    result := make([]byte, buf.Len())
    copy(result, buf.Bytes())
    return result
}
```

**Дополнительно: pre-allocated encoder pool**

```go
// Для очень горячих путей: pool с encoder
type jsonEncoder struct {
    buf *bytes.Buffer
    enc *json.Encoder
}

var encoderPool = sync.Pool{
    New: func() interface{} {
        buf := new(bytes.Buffer)
        return &jsonEncoder{buf: buf, enc: json.NewEncoder(buf)}
    },
}

func marshalFast(v interface{}) ([]byte, error) {
    je := encoderPool.Get().(*jsonEncoder)
    je.buf.Reset()
    defer encoderPool.Put(je)

    if err := je.enc.Encode(v); err != nil {
        return nil, err
    }
    result := make([]byte, je.buf.Len())
    copy(result, je.buf.Bytes())
    return result, nil
}
```

**Для hot path: рассмотреть jsoniter или easyjson**

```go
// jsoniter — drop-in замена, 2-3x быстрее encoding/json
import jsoniter "github.com/json-iterator/go"

var json = jsoniter.ConfigCompatibleWithStandardLibrary

data, err := json.Marshal(items)  // тот же API, меньше аллокаций
```

### Результат

```
Метрика              До          После
GC frequency         каждые 200ms  каждые 3s
CPU on GC            8%            0.5%
Total CPU            45%           18%
Alloc rate           4.5 GB/min    0.3 GB/min
p99 latency          120ms         22ms
```

---

## Шпаргалка: симптом → инструмент

| Симптом | Первый инструмент | Что искать |
|---|---|---|
| Высокий CPU, пропорционально RPS | CPU профиль, flamegraph | Широкое плато, regexp/fmt/json |
| RSS растёт постоянно | `inuse_space` + `-diff_base` | Что выросло за период |
| GC паузы (latency spikes) | `gctrace=1` + `alloc_space` | Кто создаёт мусор |
| Throughput не масштабируется | mutex профиль | `sync.(*RWMutex).RLock` |
| Горутины растут | goroutine dump | chan receive/send без ctx |
| CPU нормальный, но latency высокая | runtime/trace | GC STW, пустые P |
| Периодические latency spikes | runtime/trace | Регулярные GC паузы |
