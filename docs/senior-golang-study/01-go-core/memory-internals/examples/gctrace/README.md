# gctrace demo

Запускаемая программа, чтобы «вживую» посмотреть на сборщик мусора Go через `GODEBUG=gctrace=1` и почувствовать, как на GC влияют `GOGC` и `GOMEMLIMIT`. Связана с [../../04-garbage-collector.md](../../04-garbage-collector.md).

## Что внутри

`main.go` даёт **управляемую аллокационную нагрузку**:

- **мусор** (`-alloc` KB за итерацию, пауза `-rate`) — короткоживущие слайсы, которые сразу становятся garbage → заставляют GC работать;
- **живой набор** (`-live` MB) — постоянно удерживаемые ссылками данные → это «live heap», вокруг которого GC считает следующую цель;
- раз в секунду печатает `MemStats` (`NumGC`, `HeapAlloc`, `HeapSys`, `GCCPUFraction`).

## Как запустить

```bash
cd docs/senior-golang-study/01-go-core/memory-internals/examples/gctrace

# Базово
GODEBUG=gctrace=1 go run .

# Больше мусора → чаще GC
GODEBUG=gctrace=1 go run . -alloc=512 -rate=1ms

# Держать 50 MB живыми → видно, как цель ≈ 2×live
GODEBUG=gctrace=1 go run . -live=50 -alloc=512

# GOGC: агрессивнее / реже
GOGC=50  GODEBUG=gctrace=1 go run . -live=50
GOGC=400 GODEBUG=gctrace=1 go run . -live=50

# GOMEMLIMIT: мягкий лимит памяти
GOMEMLIMIT=128MiB GODEBUG=gctrace=1 go run . -live=80 -alloc=512
GOGC=off GOMEMLIMIT=200MiB GODEBUG=gctrace=1 go run . -live=50
```

> Для чистого вывода (без шума родителя `go run`) собери бинарник: `go build -o gcd . && GODEBUG=gctrace=1 ./gcd ...`.

## Как читать строку gc

```
gc 6 @0.188s 0%: 0.12+0.21+0.006 ms clock, ... 100->100->51 MB, 102 MB goal, ... 16 P
   │    │     │                              │    │    │        │
   │    │     │                              │    │    │        └ цель следующего GC (≈ 2×live при GOGC=100)
   │    │     │                              │    │    └ heap ПОСЛЕ sweep ≈ ЖИВОЙ heap (тут ~51 MB = -live=50)
   │    │     │                              │    └ heap в начале sweep
   │    │     │                              └ heap ДО GC (дорос до цели)
   │    │     └ доля CPU, потраченная на GC (следи, чтобы не росла под нагрузкой)
   │    └ время с старта
   └ номер цикла GC
```

Полный разбор полей — в [04-garbage-collector.md](../../04-garbage-collector.md#диагностика).

## Что попробовать (мини-эксперименты)

1. **Формула `NextGC = live × (1 + GOGC/100)`.** Запусти `-live=50` и смотри на тройку `X->Y->Z` и `goal`: `Z ≈ 51 MB` (живой набор), а `goal ≈ 102 MB` ≈ 2×live (GOGC=100). Это та самая формула триггера из теории.
2. **GOGC меняет частоту GC.** Сравни `GOGC=50` vs `GOGC=400` при том же `-live=50`: с 50 цель ≈ 1.5×live (GC чаще, RSS меньше), с 400 ≈ 5×live (GC реже, памяти больше). Смотри `NumGC` в строках `[app]`.
3. **GOMEMLIMIT как потолок.** `GOMEMLIMIT=128MiB go run . -live=80`: даже если по GOGC цель была бы выше, GC начнёт собирать у лимита, удерживая память. Подними `-live` близко к лимиту → увидишь, как GC становится частым (приближение к death spiral).
4. **GOGC=off + GOMEMLIMIT.** `GOGC=off GOMEMLIMIT=200MiB`: GC по росту heap не запускается вообще — только у лимита памяти. Полезно для throughput-нагрузок (но если live set близок к лимиту — тот же death spiral).
5. **Allocation rate и GCCPUFraction.** Уменьшай `-rate` (1ms → 100µs) или увеличивай `-alloc`: `GCCPUFraction` в `[app ...]` поползёт вверх — это и есть «давление на GC» из-за высокого allocation rate.

## Соседнее

- Разбор алгоритма GC, фаз, write barrier, GOGC/GOMEMLIMIT, Green Tea — [../../04-garbage-collector.md](../../04-garbage-collector.md).
- Наблюдать **планировщик** (а не GC) — отдельный пример [runtime-scheduler/examples/schedtrace](../../../runtime-scheduler/examples/schedtrace/).
