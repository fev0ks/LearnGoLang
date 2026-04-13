# Go 1.25

Релиз: август 2025.

Главная идея релиза: акцент на runtime-поведение в контейнерах, диагностику и более сильный набор инструментов для concurrent/testing задач.

## Что изменилось

### Язык

- Языковых изменений, влияющих на поведение программ, в Go 1.25 нет.
- Это хороший пример релиза, где главная ценность не в синтаксисе, а в runtime, tooling и stdlib.

### Tooling и `go` command

- `go build -asan` теперь по умолчанию включает leak detection на выходе процесса.
- В дистрибутиве стало меньше prebuilt tool binaries: редкие инструменты будут собираться `go tool` по мере необходимости.
- В `go.mod` появился `ignore` directive для директорий, которые `go` command должен пропускать при `./...` и похожих pattern matches.
- Появился `go doc -http`, который поднимает локальный documentation server.
- `go version -m -json` упрощает машинный разбор embedded build info в бинарях.

### Runtime и observability

- `GOMAXPROCS` стал container-aware.
- На Linux runtime теперь учитывает cgroup CPU bandwidth limit, а не только число доступных CPU.
- Runtime также умеет периодически обновлять `GOMAXPROCS`, если лимиты или доступные CPU изменились во время жизни процесса.
- Это важно для Go-сервисов в Kubernetes: поведение по умолчанию стало ближе к реальным CPU limits контейнера.
- Появился experimental Green Tea GC через `GOEXPERIMENT=greenteagc`.
- Появился `runtime/trace.FlightRecorder`: легковесная кольцевая запись trace с возможностью снять последние секунды после инцидента.
- Изменился текст unhandled panic при recover+repanic, а на Linux runtime теперь умеет помечать anonymous mappings более информативными именами.

### Compiler и поведение кода

- Исправлен compiler bug из Go 1.21-1.24, который мог откладывать nil check слишком поздно.
- Код, который раньше "случайно работал", в Go 1.25 может начать корректно падать с nil pointer panic.
- Это не regression релиза, а устранение некорректного поведения.

### Standard library

- `testing/synctest` стал general availability и дает удобный способ тестировать concurrent code в изолированном bubble с виртуализированным временем.
- Появился experimental `encoding/json/v2` и низкоуровневый `encoding/json/jsontext`.
- При `GOEXPERIMENT=jsonv2` стандартный `encoding/json` использует новую реализацию, где decoding заметно быстрее во многих сценариях.

## Что это меняет на практике

- в Kubernetes можно реже тянуться к внешним библиотекам для автоматической настройки `GOMAXPROCS`;
- для редких production-инцидентов trace можно собирать точечно, а не держать тяжелый continuous tracing;
- команды, у которых много flaky/сложных concurrent tests, получают сильный новый инструмент через `testing/synctest`;
- перед апгрейдом на 1.25 нужно прогнать тесты на скрытые nil dereference, которые раньше маскировались compiler bug.

## Что проверить перед апгрейдом

- нет ли сервисов, где логика неявно полагалась на старый default `GOMAXPROCS`;
- не ломаются ли интеграции, которые ожидают старое panic output;
- нет ли "случайно работающего" кода с обращением к результату до проверки `err`;
- есть ли смысл экспериментально погонять сервисы с `GOEXPERIMENT=greenteagc` или `GOEXPERIMENT=jsonv2`.

## Что могут спросить на интервью

- как container-aware `GOMAXPROCS` влияет на CPU throttling и throughput в Kubernetes;
- чем `FlightRecorder` лучше постоянной записи полного runtime trace;
- чем `testing/synctest` полезнее обычных sleep-based тестов;
- почему исправление compiler bug может проявиться как новый panic после апгрейда.

## Источники

- [Go 1.25 Release Notes](https://go.dev/doc/go1.25)
- [Go Release History](https://go.dev/doc/devel/release)
