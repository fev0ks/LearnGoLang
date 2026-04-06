# LearnGoLang

Личный учебный репозиторий по Go: здесь собраны короткие эксперименты, разборы языковых ловушек, interview-задачи и небольшие sandbox-примеры.

Сейчас проект не выглядит как "одна программа" и это нормально. Его удобнее воспринимать как набор тематических полигонов.

## Как теперь читать проект

- `topics/` — теперь это основной слой навигации и физической раскладки примеров по темам.
- `topics/01-language-basics` — базовые заметки по языку: `slice`, `map`, `defer`, `interfaces`, `pointers`, `structs`, строки.
- `topics/02-concurrency` — `chan`, goroutine, multithreading, rate limiting, lock-free и concurrency patterns.
- `topics/03-runtime-and-internals` — `atomic`, alignment, typed `nil`, netpoll, syscall, tracing, generics, low-level поведение runtime.
- `topics/04-storage-and-integrations` — Redis, Mongo, интеграционные и сетевые примеры.
- `topics/05-patterns-and-system-design` — observer/pub-sub, outbox, websocket, design-style sandbox.
- `topics/06-algorithms-and-tasks` — алгоритмы, leetcode, маленькие interview tasks и структуры данных.
- `topics/07-mixed-sandbox` — остаточный sandbox для исторических и разрозненных экспериментов.
- `topics/08-interview-prep` — заметки и упражнения именно под интервью.
- `pkg/` — теперь в основном служебные/переиспользуемые пакеты и несколько оставшихся библиотечных модулей.

## Как запускать

- запуск базового примера: `go run ./topics/01-language-basics/examples/slices`
- запуск concurrency-примера: `go run ./topics/02-concurrency/examples/channels`
- запуск runtime-примера: `go run ./topics/03-runtime-and-internals/examples/nil_interface`
- запуск sandbox-примера: `go run ./topics/07-mixed-sandbox/cmd3`
- запуск алгоритмического примера: `go run ./topics/06-algorithms-and-tasks/leetcode/find_flight_dest`
- запуск тестов по concurrency examples: `go test ./topics/02-concurrency/...`

## Правило для новых материалов

Если добавляешь новый пример:
- выбирай тематическую папку в `topics/`, а не остаточный sandbox;
- в начале файла коротко пиши, что демонстрирует пример;
- рядом с важными `fmt.Println(...)` оставляй комментарий с ожидаемым выводом или причиной неожиданного поведения;
- если пример intentionally паникует, явно комментируй это рядом со строкой.

## Что уже приведено в порядок

- добавлена навигация по структуре;
- ключевые примеры про `slice`, `interface`, `defer`, `chan` и typed `nil` снабжены пояснениями на русском;
- опасные места, где пример мог вводить в заблуждение, прокомментированы более явно.
- исправлены самые заметные опечатки в именах файлов и каталогов: `mian -> main`, `serach -> search`, `small_tasts -> small_tasks`, `fixBrakets -> fix_brackets`, `theads -> threads`.
- старые хаотичные директории физически разнесены по `topics/`:
  `cmd*`, `image_byte`, `everything`, `leetcode`, бывшие `pkg/code_examples/*`, `some_intw`, `multithreading`, `impl_observer`, `mongo_lrn`, `interview`.

## Тематическая карта

- [topics/01-language-basics/README.md](/Users/fev0ks/Projects/personal/LearnGoLang/topics/01-language-basics/README.md)
- [topics/02-concurrency/README.md](/Users/fev0ks/Projects/personal/LearnGoLang/topics/02-concurrency/README.md)
- [topics/03-runtime-and-internals/README.md](/Users/fev0ks/Projects/personal/LearnGoLang/topics/03-runtime-and-internals/README.md)
- [topics/04-storage-and-integrations/README.md](/Users/fev0ks/Projects/personal/LearnGoLang/topics/04-storage-and-integrations/README.md)
- [topics/05-patterns-and-system-design/README.md](/Users/fev0ks/Projects/personal/LearnGoLang/topics/05-patterns-and-system-design/README.md)
- [topics/06-algorithms-and-tasks/README.md](/Users/fev0ks/Projects/personal/LearnGoLang/topics/06-algorithms-and-tasks/README.md)
- [topics/07-mixed-sandbox/README.md](/Users/fev0ks/Projects/personal/LearnGoLang/topics/07-mixed-sandbox/README.md)
- [topics/08-interview-prep/README.md](/Users/fev0ks/Projects/personal/LearnGoLang/topics/08-interview-prep/README.md)
