# Compiler, измерения и CPU hot path

## Содержание

- [Диагностика компилятора](#диагностика-компилятора)
- [Assembly](#assembly)
- [Ручное разворачивание циклов](#ручное-разворачивание-циклов)
- [Inlining и разделение fast/slow path](#inlining-и-разделение-fastslow-path)
- [Defer и стек goroutine](#defer-и-стек-goroutine)
- [PGO до ручных трюков](#pgo-до-ручных-трюков)
- [Checklist](#checklist)
- [Источники](#источники)

Compiler diagnostics отвечают на вопрос «что сделал toolchain», но не говорят,
какую функцию стоит оптимизировать. Сначала профиль находит hot path, затем
`-m=2`, assembly и benchmark проверяют конкретную гипотезу.

---

## Диагностика компилятора

`-m=2` показывает причины inlining и escape analysis. Полезно ограничивать
вывод исследуемым package: `all=-m=2` для dependency graph быстро создаёт тысячи
строк.

```bash
# Для всех собираемых packages
go build -gcflags='all=-m=2' ./cmd/service 2> compiler.txt

# Для одного package pattern
go build \
    -gcflags='example.com/project/internal/codec=-m=2' \
    ./cmd/service 2> compiler.txt
```

Важные сообщения:

- `can inline` и `inlining call` — функция или конкретный вызов встроены;
- `cannot inline ... cost ... exceeds budget` — функция превышает текущий
  бюджет inliner;
- `escapes to heap` и `moved to heap` — значение не остаётся только в своём
  stack frame;
- `does not escape` — переданный указатель не сохраняется за пределами
  анализируемого вызова.

Это diagnostics конкретной сборки, а не контракт языка. Изменение caller,
generic instantiation, PGO profile или версии Go может поменять решение.

Gopls также умеет показывать compiler optimization details: escape, inlining,
nil checks и bounds checks. Такой режим удобен для локального исследования, но
итог всё равно подтверждается той сборкой, которая используется в production.

---

## Assembly

```bash
# Assembly во время сборки
go build -gcflags='example.com/project/internal/codec=-S' \
    ./cmd/service 2> asm.txt

# Disassembly уже собранного binary
go tool objdump \
    -s 'example.com/project/internal/codec\.Encode' \
    ./service
```

Assembly нужен для узкого вопроса:

- остался ли вызов функции;
- устранена ли проверка границ;
- выполняется ли лишнее копирование;
- появилась ли ожидаемая инструкция;
- сколько branches осталось в hot loop.

Начинать поиск производительности с полного assembly dump неудобно: профиль
быстрее показывает, какой участок заслуживает внимания.

Сборка с `-N -l` отключает оптимизации и inlining. Она полезна для debugging,
но не подходит для оценки production-кода.

---

## Ручное разворачивание циклов

Разворачивание уменьшает долю loop-control instructions и может открыть
instruction-level parallelism. Независимые accumulators особенно важны, когда
один accumulator создаёт dependency chain:

```go
func sumUnrolled(values []int64) int64 {
    var s0, s1, s2, s3 int64
    i := 0

    for ; i+4 <= len(values); i += 4 {
        s0 += values[i]
        s1 += values[i+1]
        s2 += values[i+2]
        s3 += values[i+3]
    }

    total := s0 + s1 + s2 + s3
    for ; i < len(values); i++ {
        total += values[i]
    }
    return total
}
```

Ускорение не гарантировано. Ручной unrolling может:

- увеличить функцию и pressure на instruction cache;
- сохранить или добавить bounds checks;
- увеличить register pressure и spill values на стек;
- изменить порядок вычислений; для floating-point это меняет rounding;
- проиграть оптимизациям новой версии compiler или другой CPU architecture.

Duff's device — исторический способ совместить обработку остатка и развёрнутый
цикл через необычный control flow. В обычном Go-коде явный основной цикл и
отдельный tail проще проверять и поддерживать.

Такое изменение принимается вместе с benchmark обычного и развёрнутого
вариантов на целевых архитектурах. Для особо горячей численной обработки также
сравнивают специализированную assembly/SIMD implementation. В Go 1.27 package
`simd` остаётся экспериментальным и требует `GOEXPERIMENT=simd`.

---

## Inlining и разделение fast/slow path

Inlining убирает границу вызова, но часто важнее то, что он открывает другие
оптимизации:

- constant propagation и удаление недостижимых branches;
- devirtualization конкретного вызова;
- более точный escape analysis;
- устранение повторных bounds и nil checks;
- совместную оптимизацию caller и callee.

Начиная с register-based calling convention аргументы и результаты не обязаны
копироваться через стек при каждом вызове. Поэтому модель «каждая функция всегда
копирует все параметры на стек» не объясняет универсальную цену call.

Большая функция может не поместиться в inline budget. Тогда редкую сложную ветку
можно вынести в отдельную функцию:

```go
func (p *Parser) NextByte() (byte, bool) {
    if p.offset < len(p.data) {
        value := p.data[p.offset]
        p.offset++
        return value, true
    }
    return p.nextByteSlow()
}

func (p *Parser) nextByteSlow() (byte, bool) {
    // Refill, boundary handling и подробная диагностика ошибки.
    return 0, false
}
```

Обычный путь становится маленьким кандидатом на inlining, а cold path не
расходует его budget. Такой приём используется в стандартной библиотеке,
например в fast path `sync.Mutex.Lock` с переходом в `lockSlow`.

После изменения проверяются две вещи:

```bash
# Встроился ли fast path
go build -gcflags='example.com/project/internal/parser=-m=2' ./cmd/service

# Изменилось ли измеряемое поведение
go test -run='^$' -bench=BenchmarkParser -benchmem -count=10 \
    ./internal/parser
```

Механическое дробление каждой функции на fast/slow parts ухудшает навигацию и
может увеличить число вызовов на реальном пути. Приём нужен только при явной
асимметрии: маленькая частая ветка и редкая сложная ветка.

---

## Defer и стек goroutine

В compiler Go 1.27 наличие `defer` остаётся одной из причин, по которым функция
может не быть inlineable. Это implementation detail, а не правило языка;
проверять нужно через `-m=2` после каждого существенного обновления toolchain.

Удалять `defer` только ради предположительного ускорения опасно. Compiler умеет
open-code многие defer calls, а ручной `Unlock`, `Close` или возврат buffer во
всех branches проще забыть. Замена оправдана, когда профиль и benchmark
показывают эффект, а control flow остаётся очевидным.

Closure внутри `defer` может захватить переменную и повлиять на escape. Но это
не означает, что каждый `defer` создаёт heap allocation. Проверяется конкретная
функция и конкретная сборка.

Вызов функции может содержать stack-bound check. Только при нехватке места
runtime останавливает конкретную goroutine, увеличивает её стек и переносит
данные. Копирование стека не происходит при каждом вызове, поэтому его нельзя
складывать в постоянную цену обычного call.

---

## PGO до ручных трюков

Profile-guided optimization использует representative CPU profile, чтобы
агрессивнее оптимизировать горячие calls. В Go PGO влияет, в частности, на
inlining и devirtualization.

Типичный flow:

1. выпустить обычный binary;
2. собрать CPU profiles на representative production workload;
3. объединить профили, если нужно покрыть разные instances и периоды;
4. положить `default.pgo` рядом с main package либо передать `-pgo`;
5. сравнить PGO и non-PGO builds на одинаковой нагрузке;
6. регулярно обновлять профиль, чтобы он не отставал от кода.

Микробенчмарк обычно является плохим источником PGO profile: он покрывает только
малую часть программы. Production profile лучше отражает реальные горячие
вызовы и зависимости.

PGO не отменяет benchmark. Он добавляет ещё один вариант сборки со своими
trade-offs: временем build, размером binary и зависимостью от качества profile.

---

## Checklist

- Hot path найден через профиль, а не выбран по внешнему виду кода.
- `-m=2` запущен для production-like build и нужного package.
- Assembly читается для конкретного вопроса, а не ради полного аудита binary.
- Обычный и оптимизированный варианты сравниваются несколькими запусками.
- Unrolling проверен на целевых `GOARCH` и типичных размерах slice.
- Fast/slow split действительно делает частый путь маленьким.
- Удаление `defer` не создаёт пропущенные cleanup paths.
- PGO проверен до распространения низкоуровневых трюков по codebase.
- После микробенчмарка проверены CPU, p95/p99, RSS и binary size сервиса.

---

## Источники

- [Go Diagnostics](https://go.dev/doc/diagnostics)
- [Gopls compiler optimization details](https://go.dev/gopls/features/diagnostics)
- [Profile-guided optimization](https://go.dev/doc/pgo)
- [Go 1.27 Release Notes](https://go.dev/doc/go1.27)
- [Go compiler inliner source](https://go.dev/src/cmd/compile/internal/inline/inl.go)
