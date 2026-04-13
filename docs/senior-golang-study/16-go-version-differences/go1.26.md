# Go 1.26

Релиз: февраль 2026.

Главная идея релиза: следующий шаг после 1.25, где experimental идеи начинают становиться default, а tooling сильнее помогает с модернизацией кода.

## Что изменилось

### Язык

- Встроенная функция `new` теперь принимает выражение: можно писать `new(expr)`, сразу создавая указатель на значение выражения.
- Это особенно удобно для optional pointer fields, например в `encoding/json` или protobuf-моделях.
- Снято ограничение на self-reference generic type в списке type parameters constraint'а.
- Это делает generic constraints выразительнее и полезнее для самотипизированных интерфейсов.

### Tooling и `go` command

- `go fix` фактически перезапущен как платформа modernizers.
- Он теперь строится на том же analysis framework, что и `go vet`, и рассчитан на безопасные автоматические миграции к современным idioms и API.
- `go mod init` теперь по умолчанию пишет более низкую версию Go в новый `go.mod`.
- На toolchain `1.26.x` новый модуль по умолчанию получит `go 1.25.0`, а не `go 1.26.0`.
- `cmd/doc` и `go tool doc` удалены; стандартная замена теперь `go doc`.
- В `pprof -http` flame graph стал default view.

### Runtime и debugging

- Green Tea GC, который в 1.25 был экспериментом, в 1.26 включен по умолчанию.
- Для GC-heavy нагрузок релиз обещает заметное снижение GC overhead, а на новых amd64 CPU ожидается дополнительный выигрыш.
- Базовый overhead `cgo` вызовов снижен примерно на 30%.
- На 64-битных платформах включена randomization heap base address как security hardening.
- Появился experimental leak profile `goroutineleak` в `runtime/pprof` и endpoint `/debug/pprof/goroutineleak`.
- Появился experimental `runtime/secret` для более надежного стирания временных данных, связанных с секретами.

### Standard library

- Новый пакет `crypto/hpke` добавляет Hybrid Public Key Encryption по RFC 9180, включая post-quantum hybrid KEMs.
- Появился experimental `simd/archsimd` для architecture-specific SIMD на amd64.
- `bytes.Buffer.Peek` позволяет посмотреть следующие `n` байт без продвижения указателя чтения.
- В crypto-пакетах усиливается тренд на более безопасное использование randomness: часть API теперь игнорирует пользовательский random source и берет криптографически безопасный источник.

### Platform и compatibility

- Go 1.26 требует для bootstrap минимум Go 1.24.6.
- `windows/arm` 32-bit удален.
- `linux/riscv64` получил поддержку race detector.
- Go 1.26 - последний релиз с поддержкой macOS 12 Monterey.

## Что это меняет на практике

- `go fix` становится реальным инструментом массовой миграции кодовой базы, а не историческим артефактом;
- если сервис сильно упирается в GC или делает много `cgo` вызовов, апгрейд до 1.26 стоит мерить отдельными benchmark/profiling прогонами;
- leak detection для горутин становится ближе к production use, особенно в CI и на сервисах со сложной конкурентностью;
- security-команды и криптографический код получают новые примитивы и более безопасные defaults.

## Что проверить перед апгрейдом

- нет ли внутренних скриптов или IDE-интеграций, которые все еще вызывают `go tool doc`;
- устраивает ли команду новый default `go` version в `go mod init`;
- есть ли сервисы, которым полезно включить и протестировать `goroutineleak` profile;
- не используется ли кастомный источник randomness там, где новое crypto API теперь его игнорирует.

## Что могут спросить на интервью

- зачем менять `go fix`, если есть линтеры и ручные refactor'ы;
- почему lower default version в `go mod init` полезен для совместимости модулей;
- чем Go 1.26 отличается от 1.25 по состоянию Green Tea GC;
- какие типы goroutine leaks можно найти новым профилем, а какие он не поймает.

## Источники

- [Go 1.26 Release Notes](https://go.dev/doc/go1.26)
- [Go Release History](https://go.dev/doc/devel/release)
