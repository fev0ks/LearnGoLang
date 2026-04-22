# Benchmarks: измерение производительности

Бенчмарк без правильной методологии врёт. Эта тема про то, как писать бенчмарки которым можно доверять, как интегрировать их с pprof и как сравнивать результаты.

## Содержание

- [Основы: testing.B](#основы-testingb)
- [Типичные ошибки в бенчмарках](#типичные-ошибки-в-бенчмарках)
- [Флаги go test для бенчмарков](#флаги-go-test-для-бенчмарков)
- [Parallel benchmarks: b.RunParallel](#parallel-benchmarks-brunparallel)
- [Sub-benchmarks: b.Run](#sub-benchmarks-brun)
- [Интеграция с pprof](#интеграция-с-pprof)
- [benchstat: сравнение результатов](#benchstat-сравнение-результатов)
- [PGO: profile-guided optimization](#pgo-profile-guided-optimization)
- [Interview-ready answer](#interview-ready-answer)

---

## Основы: testing.B

```go
// Файл должен называться *_test.go
// Функция должна начинаться с Benchmark
func BenchmarkProcessItem(b *testing.B) {
    item := createTestItem()  // setup вне цикла

    b.ResetTimer()  // не считать setup время

    for i := 0; i < b.N; i++ {  // b.N подбирается автоматически
        processItem(item)
    }
}
```

`b.N` — Go подбирает автоматически чтобы бенчмарк занял ~1 секунду. Не задавай N вручную.

```bash
go test -bench=BenchmarkProcessItem -benchmem ./...
```

```
BenchmarkProcessItem-8       1234567     987 ns/op    128 B/op    3 allocs/op
                    ↑ GOMAXPROCS  ↑ итераций  ↑ нс/op   ↑ байт/op  ↑ аллокаций/op
```

### -benchmem обязателен

Без `-benchmem` не видно аллокаций. Аллокации → GC pressure → latency спайки. Всегда добавляй `-benchmem`.

```bash
go test -bench=. -benchmem ./...
```

---

## Типичные ошибки в бенчмарках

### Ошибка 1: Компилятор вырезал код (dead code elimination)

```go
// ❌ Компилятор видит что результат не используется и вырезает вызов
func BenchmarkProcessItem_WRONG(b *testing.B) {
    for i := 0; i < b.N; i++ {
        processItem(item)  // результат нигде не используется!
    }
}
// Результат: 0.1 ns/op — неправдоподобно быстро

// ✅ Sink переменная предотвращает dead code elimination
var globalSink interface{}

func BenchmarkProcessItem(b *testing.B) {
    var result ProcessResult
    for i := 0; i < b.N; i++ {
        result = processItem(item)
    }
    globalSink = result  // присвоить в глобальную переменную
}
```

### Ошибка 2: Забыли ResetTimer после дорогого setup

```go
// ❌ Время setup включается в результат
func BenchmarkQueryDB_WRONG(b *testing.B) {
    db, _ := setupTestDB()  // занимает несколько секунд
    // b.N итераций после долгого setup — время setup включено!
    for i := 0; i < b.N; i++ {
        db.Query("SELECT 1")
    }
}

// ✅ ResetTimer после setup
func BenchmarkQueryDB(b *testing.B) {
    db, _ := setupTestDB()
    defer db.Close()

    b.ResetTimer()  // сбросить после setup

    for i := 0; i < b.N; i++ {
        db.Query("SELECT 1")
    }
}
```

### Ошибка 3: Не изолировать от GC

```go
// ❌ GC во время бенчмарка может дать outliers
func BenchmarkHeavy(b *testing.B) {
    for i := 0; i < b.N; i++ {
        result = heavyWork()
    }
}

// ✅ Принудительный GC перед бенчмарком
func BenchmarkHeavy(b *testing.B) {
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        if i == 0 {
            runtime.GC()  // один GC перед замером, не в каждой итерации
        }
        result = heavyWork()
    }
}
```

### Ошибка 4: Измерять с кешем

```go
// ❌ Первый вызов прогревает кеш, последующие измеряются по кешу
func BenchmarkWithCache_MISLEADING(b *testing.B) {
    cache := NewCache()
    item := getTestItem()

    for i := 0; i < b.N; i++ {
        cache.Get(item.Key)  // после первой итерации всегда cache hit
    }
}

// ✅ Инвалидировать или использовать разные ключи
func BenchmarkWithCache(b *testing.B) {
    cache := NewCache()
    keys := generateKeys(b.N)  // b.N разных ключей

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cache.Get(keys[i])  // каждый раз уникальный ключ
    }
}
```

### Ошибка 5: Один run вместо нескольких

```go
// Один запуск даёт ненадёжный результат
go test -bench=. -benchmem -count=1  // ← ненадёжно

// ✅ Минимум 5–10 запусков для benchstat
go test -bench=. -benchmem -count=10 ./...
```

---

## Флаги go test для бенчмарков

```bash
# Запустить только бенчмарки (без тестов)
go test -bench=. -run='^$' ./...

# Конкретный бенчмарк по regex
go test -bench=BenchmarkProcess -run='^$' ./...

# Количество запусков (для benchstat)
go test -bench=. -count=5 -benchmem ./...

# Продолжительность каждого бенчмарка (по умолчанию 1s)
go test -bench=. -benchtime=3s ./...

# Задать число итераций явно (редко нужно)
go test -bench=. -benchtime=1000x ./...  # ровно 1000 итераций

# Параллелизм (по умолчанию GOMAXPROCS)
go test -bench=. -cpu=1,2,4,8 ./...  # запустить при разных GOMAXPROCS

# Сохранить профили
go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof -benchmem ./...
```

---

## Parallel benchmarks: b.RunParallel

Для тестирования thread-safety и конкурентной производительности:

```go
func BenchmarkCacheGet_Parallel(b *testing.B) {
    cache := NewCache()
    prepopulate(cache)

    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        // Каждая параллельная горутина выполняет свой цикл
        for pb.Next() {
            cache.Get("key")
        }
    })
}
```

```bash
# Запустить с разными уровнями параллелизма
go test -bench=BenchmarkCacheGet_Parallel -cpu=1,2,4,8 -benchmem ./...

BenchmarkCacheGet_Parallel      4     311 ns/op    0 B/op    0 allocs/op
BenchmarkCacheGet_Parallel-2    4     189 ns/op    0 B/op    0 allocs/op
BenchmarkCacheGet_Parallel-4    4     112 ns/op    0 B/op    0 allocs/op
BenchmarkCacheGet_Parallel-8    4     108 ns/op    0 B/op    0 allocs/op
# Производительность растёт до 4 CPU → при 8 нет прироста → есть bottleneck при 4+ cores
```

---

## Sub-benchmarks: b.Run

```go
func BenchmarkMarshal(b *testing.B) {
    sizes := []int{10, 100, 1000, 10000}

    for _, n := range sizes {
        n := n  // захватить локально
        b.Run(fmt.Sprintf("items=%d", n), func(b *testing.B) {
            items := generateItems(n)
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                globalSink, _ = json.Marshal(items)
            }
        })
    }
}
```

```
BenchmarkMarshal/items=10-8         500000     2341 ns/op    1024 B/op    10 allocs/op
BenchmarkMarshal/items=100-8         50000    23412 ns/op    8192 B/op    14 allocs/op
BenchmarkMarshal/items=1000-8         5000   234120 ns/op   65536 B/op    17 allocs/op
BenchmarkMarshal/items=10000-8         500  2341200 ns/op  524288 B/op    20 allocs/op
```

→ Видно что время линейно с N (нормально), но аллокации растут значительно меньше.

---

## Интеграция с pprof

```bash
# Собрать CPU профиль во время бенчмарка
go test -bench=BenchmarkHeavy -cpuprofile=cpu.prof -run='^$' ./...
go tool pprof -http=:6061 cpu.prof

# Собрать memory профиль
go test -bench=BenchmarkHeavy -memprofile=mem.prof -run='^$' ./...
go tool pprof -inuse_space -http=:6061 mem.prof
go tool pprof -alloc_space -http=:6061 mem.prof

# Оба сразу
go test -bench=BenchmarkHeavy -cpuprofile=cpu.prof -memprofile=mem.prof -benchmem -run='^$' ./...
```

**Workflow:**

```
1. go test -bench=. -benchmem -count=5 > baseline.txt
2. Найти проблему через pprof
3. Применить оптимизацию
4. go test -bench=. -benchmem -count=5 > optimized.txt
5. benchstat baseline.txt optimized.txt
6. Убедиться что улучшение значимо (p < 0.05)
```

---

## benchstat: сравнение результатов

`benchstat` сравнивает два набора результатов и показывает статистически значимые изменения.

```bash
# Установить
go install golang.org/x/perf/cmd/benchstat@latest

# Собрать baseline (несколько запусков важны!)
go test -bench=. -benchmem -count=10 ./... > before.txt

# Применить изменения, снова собрать
go test -bench=. -benchmem -count=10 ./... > after.txt

# Сравнить
benchstat before.txt after.txt
```

```
goos: linux
goarch: amd64

              │ before.txt  │          after.txt           │
              │   sec/op    │   sec/op     vs base          │
ProcessItem     1.234µ ± 2%  0.312µ ± 3%  -74.72% (p=0.000)
BuildIndex      5.678ms ± 1%  5.681ms ± 2%   +0.05% (p=0.912)  ← нет разницы

              │  B/op   │      B/op       │
ProcessItem    128 ± 0%   16 ± 0%  -87.50% (p=0.000)

              │ allocs/op  │  allocs/op   │
ProcessItem    3.00 ± 0%   1.00 ± 0%  -66.67% (p=0.000)
```

**Читаем:**
- `p=0.000` — изменение статистически значимо
- `p=0.912` — изменение незначимо (шум)
- `±2%` — разброс (низкий = стабильный бенчмарк)
- ProcessItem: -74% времени, -87% памяти, -66% аллокаций — отличная оптимизация

### Когда бенчмарк нестабилен (большой ±%)

```
ProcessItem   1.234µ ± 40%  ← проблема
```

Причины:
- Другие процессы на машине потребляют CPU
- GC срабатывает непредсказуемо
- Слишком маленькое benchtime

```bash
# Запустить на изолированной машине или с:
go test -bench=. -count=20 -benchtime=5s ./...
```

---

## PGO: profile-guided optimization

Go 1.20+ поддерживает **profile-guided optimization** — компилятор использует CPU профиль production сервиса для оптимизации кода.

### Как работает

1. Компилируешь без PGO, деплоишь в production
2. Собираешь CPU профиль под нагрузкой
3. Перекомпилируешь с профилем → компилятор агрессивнее инлайнит горячие функции

```bash
# Шаг 1: Собрать профиль из production
curl -o default.pgo "http://prod-service:6060/debug/pprof/profile?seconds=30"

# Шаг 2: Поместить в директорию пакета (Go ищет default.pgo автоматически)
cp default.pgo cmd/myapp/default.pgo

# Шаг 3: Обычная сборка — компилятор сам найдёт default.pgo
go build ./cmd/myapp/
# Или явно:
go build -pgo=default.pgo ./cmd/myapp/
```

### Ожидаемое улучшение

По данным Google: 2–7% улучшение throughput для типичных Go сервисов. Наибольший эффект для сервисов с:
- Много коротких функций (больше инлайнинга)
- Горячие пути с предсказуемыми ветвлениями

```bash
# Проверить что PGO применилось
go build -pgo=default.pgo -v ./... 2>&1 | grep pgo
```

---

## Interview-ready answer

**"Как ты измеряешь производительность Go кода?"**

Начинаю с микробенчмарков `testing.B` для подозрительных функций. Обязательные правила:
1. `-benchmem` — всегда, аллокации важны
2. `b.ResetTimer()` после setup — не мерить инициализацию
3. Sink переменная для результата — предотвратить dead code elimination
4. `-count=10` и benchstat — один запуск ненадёжен

```bash
go test -bench=BenchmarkHot -benchmem -count=10 ./... > before.txt
# ... оптимизация ...
go test -bench=BenchmarkHot -benchmem -count=10 ./... > after.txt
benchstat before.txt after.txt
```

Параллельные бенчмарки (`b.RunParallel`) показывают где появляется contention при росте числа CPU.

Для нахождения что именно оптимизировать — `go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof`, потом `go tool pprof`.

**Ловушки:**
- Бенчмарк с cache hit не то же самое что production без cache
- Компилятор может вырезать неиспользуемые результаты → нереальные цифры
- Большой ±% в результатах → бенчмарк нестабилен → не доверять
