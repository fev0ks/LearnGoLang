# Go Core

Сюда складывай материалы по самому языку и runtime.

Темы:
- указатели, value vs reference semantics;
- интерфейсы, method sets, nil interface pitfalls;
- generics и ограничения по их применению;
- ошибки, wrapping, sentinel errors, `errors.Is` и `errors.As`;
- контексты, cancellation, deadlines, propagation;
- modules, versioning, replace, workspace mode;
- escape analysis, stack vs heap;
- garbage collector и влияние allocation rate;
- scheduler, `GOMAXPROCS`, preemption;
- memory model, happens-before, visibility between goroutines.

Вопросы для senior-уровня:
- почему горутина может "утечь" и как это заметить;
- чем отличается `nil` интерфейс от интерфейса с `nil` внутри;
- когда generics полезнее интерфейсов, а когда нет;
- как читать вывод `go build -gcflags=-m`;
- какие anti-patterns чаще всего встречаются в production Go code.

## Подборка

- [Go Documentation](https://go.dev/doc)
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Language Specification](https://go.dev/ref/spec)
- [The Go Memory Model](https://go.dev/ref/mem)
- [Go FAQ](https://go.dev/doc/faq)
- [A Guide to the Go Garbage Collector](https://go.dev/doc/gc-guide)

## Вопросы

- как работает `interface` под капотом и почему `nil`-ловушка так часто встречается;
- в каких случаях значение уходит в heap и как это проверить;
- чем отличается `panic` от обычной ошибки в production-коде;
- когда стоит использовать generics, а когда они только усложняют API;
- как scheduler Go влияет на latency и fairness;
- почему копирование struct иногда безопаснее, чем передача указателя;
- как бы ты объяснил memory model Go человеку без глубокого runtime-бэкграунда.
