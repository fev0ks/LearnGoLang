# Race detector, Fuzzing и Benchmarks

Три инструмента для классов проблем, которые обычные тесты не поймают: гонки данных, неожиданные входные значения и деградация производительности.

## Содержание

- [Race detector](#race-detector)
- [Fuzzing](#fuzzing)
- [Benchmarks](#benchmarks)
- [Читаем результаты benchmark](#читаем-результаты-benchmark)
- [Когда benchmark врёт](#когда-benchmark-врёт)

---

## Race detector

`go test -race` инструментирует код и обнаруживает data race во время выполнения. Включай в CI на каждый PR.

```bash
go test -race ./...
```

### Что ловит

```go
// Data race — горутина читает и пишет без синхронизации
type Cache struct {
    data map[string]string   // не защищён mutex
}

func (c *Cache) Set(k, v string) {
    c.data[k] = v  // WRITE
}

func (c *Cache) Get(k string) string {
    return c.data[k]  // READ — race если есть конкурентный Set
}

// Race detector сообщит:
// WARNING: DATA RACE
// Write at 0x00c0001a8000 by goroutine 7:
//   main.(*Cache).Set(...)
// Previous read at 0x00c0001a8000 by goroutine 6:
//   main.(*Cache).Get(...)
```

### Паттерны которые часто имеют race

```go
// 1. Захват переменной цикла (до Go 1.22)
for _, item := range items {
    go func() {
        process(item)  // item меняется — race
    }()
}

// Fix: явный захват
for _, item := range items {
    item := item  // захватить копию
    go func() {
        process(item)
    }()
}

// 2. Shared slice без синхронизации
var results []string
var wg sync.WaitGroup
for _, id := range ids {
    wg.Add(1)
    go func(id string) {
        defer wg.Done()
        results = append(results, fetch(id))  // race на slice
    }(id)
}

// Fix: channel или mutex
results := make(chan string, len(ids))
for _, id := range ids {
    go func(id string) { results <- fetch(id) }(id)
}

// 3. Инициализация в горутине
var client *http.Client
go func() {
    client = &http.Client{}  // WRITE
}()
// ... через время ...
resp, _ := client.Do(req)  // READ — нет happens-before

// Fix: sync.Once или инициализировать до горутины
```

### Тест с намеренной параллельностью

```go
func TestCache_ConcurrentAccess(t *testing.T) {
    t.Parallel()
    cache := NewCache()

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(2)
        go func(i int) {
            defer wg.Done()
            cache.Set(fmt.Sprintf("key-%d", i), "value")
        }(i)
        go func(i int) {
            defer wg.Done()
            cache.Get(fmt.Sprintf("key-%d", i))
        }(i)
    }
    wg.Wait()
    // С -race: выявит если Cache не thread-safe
}
```

**Чего race detector не гарантирует:** он реагирует только на пути которые были исполнены. Race в редко достигаемом коде может остаться незамеченной. `go test -race` — необходимое условие, но не достаточное.

---

## Fuzzing

Fuzzing генерирует случайные входные данные и ищет паники, ошибки и инварианты. Добавлен в стандартную библиотеку с Go 1.18.

```bash
# Запустить в fuzz-режиме (генерирует входы в цикле)
go test -fuzz=FuzzNormalizePhone

# Запустить только corpus без генерации (как обычный тест)
go test -run=FuzzNormalizePhone
```

### Структура fuzz-теста

```go
func FuzzNormalizePhone(f *testing.F) {
    // Seed corpus — стартовые примеры (обязательно!)
    f.Add("+7 (999) 111-22-33")
    f.Add("+1-800-555-0199")
    f.Add("")
    f.Add("abc")
    f.Add("79991112233")

    f.Fuzz(func(t *testing.T, input string) {
        // Инвариант: функция не должна паниковать
        // и если вернула результат — он должен быть валидным
        result, err := NormalizePhone(input)
        if err != nil {
            return  // ошибка допустима, паника — нет
        }
        // Инвариант: нормализованный номер состоит только из цифр и +
        for _, c := range result {
            if c != '+' && (c < '0' || c > '9') {
                t.Errorf("normalized phone %q contains invalid char %q", result, c)
            }
        }
    })
}
```

### Хорошие кандидаты для fuzzing

```go
// Парсеры — URL, конфигурации, протоколы
func FuzzParseConfig(f *testing.F) {
    f.Add(`{"host":"localhost","port":8080}`)
    f.Add(`{}`)

    f.Fuzz(func(t *testing.T, data string) {
        var cfg Config
        if err := json.Unmarshal([]byte(data), &cfg); err != nil {
            return
        }
        // Инвариант: если распарсилось, должно валидироваться без паники
        _ = cfg.Validate()
    })
}

// Кодирование/декодирование — должны быть обратно совместимы
func FuzzEncodeDecodeRoundtrip(f *testing.F) {
    f.Add([]byte("hello"))
    f.Add([]byte{0, 1, 2, 255})

    f.Fuzz(func(t *testing.T, data []byte) {
        encoded := Encode(data)
        decoded, err := Decode(encoded)
        if err != nil {
            t.Fatalf("decode failed: %v", err)
        }
        if !bytes.Equal(data, decoded) {
            t.Fatalf("roundtrip failed: got %v, want %v", decoded, data)
        }
    })
}

// Валидаторы — не должны паниковать на любом вводе
func FuzzValidateEmail(f *testing.F) {
    f.Add("alice@example.com")
    f.Add("")
    f.Add(strings.Repeat("a", 10000))

    f.Fuzz(func(t *testing.T, input string) {
        // Просто не должно паниковать
        _ = ValidateEmail(input)
    })
}
```

### Найденный баг сохраняется в corpus

Когда fuzzer находит паник или инвариант — создаёт файл в `testdata/fuzz/<FuncName>/`. Этот файл коммитится в репозиторий и запускается в `go test -run` как обычный регрессионный тест.

```
testdata/
└── fuzz/
    └── FuzzNormalizePhone/
        └── a1b2c3d4  # входное значение которое нашло баг
```

---

## Benchmarks

Benchmark измеряет производительность и аллокации. Нужен перед и после оптимизации.

```go
func BenchmarkNormalizePhone(b *testing.B) {
    input := "+7 (999) 111-22-33"

    b.ResetTimer()  // не считать setup в результат
    for b.Loop() {  // Go 1.24+; старый стиль: for i := 0; i < b.N; i++
        NormalizePhone(input)
    }
}

// С аллокациями
func BenchmarkBuildQuery(b *testing.B) {
    b.ReportAllocs()  // явно включить allocs/op в вывод
    ids := []string{"id-1", "id-2", "id-3", "id-4", "id-5"}

    for b.Loop() {
        _ = BuildInQuery(ids)
    }
}
```

```bash
# Запустить все benchmarks с аллокациями
go test -bench=. -benchmem ./...

# Конкретный benchmark
go test -bench=BenchmarkNormalizePhone -benchmem -count=5 ./internal/phone/...

# Сравнить два варианта через benchstat
go test -bench=. -count=10 -benchmem > before.txt
# ... изменить код ...
go test -bench=. -count=10 -benchmem > after.txt
benchstat before.txt after.txt
```

### Паттерн: сравнение реализаций

```go
func BenchmarkBuildInQuery_Sprintf(b *testing.B) {
    ids := generateIDs(100)
    for b.Loop() {
        _ = buildQuerySprintf(ids)
    }
}

func BenchmarkBuildInQuery_Builder(b *testing.B) {
    ids := generateIDs(100)
    for b.Loop() {
        _ = buildQueryBuilder(ids)
    }
}

// Запустить: go test -bench=BenchmarkBuildInQuery -benchmem
// Вывод покажет ns/op, B/op, allocs/op для каждой реализации
```

### Sub-benchmarks для разных размеров входа

```go
func BenchmarkMarshalUser(b *testing.B) {
    sizes := []int{1, 10, 100, 1000}

    for _, size := range sizes {
        users := generateUsers(size)
        b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
            for b.Loop() {
                json.Marshal(users)
            }
        })
    }
}
```

---

## Читаем результаты benchmark

```
BenchmarkNormalizePhone-8     5000000     234 ns/op    48 B/op    2 allocs/op
│                             │           │            │           └─ аллокаций на вызов
│                             │           │            └─ байт выделено на вызов
│                             │           └─ наносекунд на вызов
│                             └─ итераций (b.N подобрано автоматически)
└─ -8 = GOMAXPROCS
```

**Что смотреть:**

| Метрика | Когда важна |
|---|---|
| `ns/op` | горячий путь в CPU-bound коде |
| `B/op` | GC pressure, allocation-heavy код |
| `allocs/op` | структуры в hot path (0 — лучший результат) |

---

## Когда benchmark врёт

```go
// Компилятор оптимизирует если результат не используется
func BenchmarkBad(b *testing.B) {
    for b.Loop() {
        NormalizePhone("+7 999 111 22 33")  // результат выброшен — компилятор может убрать вызов
    }
}

// Fix: sink через глобальную переменную или b.Sink (Go 1.24+)
var sinkResult string
func BenchmarkGood(b *testing.B) {
    var r string
    for b.Loop() {
        r = NormalizePhone("+7 999 111 22 33")
    }
    sinkResult = r
}
```

**Другие источники ошибок:**
- benchmark меряет warm cache, production работает на cold cache
- `b.N=1` на первой итерации — не `ResetTimer` скрывает дорогой setup
- машина под нагрузкой даёт нестабильные результаты — запускать с `-count=5` и использовать `benchstat`
- hotspot в другом месте — профилировать с `go test -bench=. -cpuprofile=cpu.out`, затем `go tool pprof`
