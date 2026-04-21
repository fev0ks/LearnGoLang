# Memory Internals

Как Go управляет памятью на уровне runtime: от layout стека горутины до алгоритма GC. Читать по порядку — каждый файл строится на предыдущем.

## Материалы

- [01. Stack And Heap](./01-stack-and-heap.md) — goroutine stack (2KB, рост копированием, shrink), heap arenas, scavenger, RSS vs VSZ, safe points
- [02. Allocator](./02-allocator.md) — size classes, mcache/mcentral/mheap иерархия, tiny allocator, noscan, large objects
- [03. Escape Analysis](./03-escape-analysis.md) — как компилятор решает стек или heap, причины escape, `-gcflags=-m`, inlining
- [04. Garbage Collector](./04-garbage-collector.md) — tri-color mark-and-sweep, write barrier, фазы GC, GOGC, GOMEMLIMIT, sync.Pool, gctrace

## Связи между файлами

```
01 Stack & Heap    → физическая организация памяти
02 Allocator       → как объекты попадают в heap
03 Escape Analysis → когда объект попадает в heap (решение компилятора)
04 GC              → что происходит с объектами в heap потом
```

`05-escape-analysis` и `08-garbage-collector` были частью `01-go-core` — они перенесены сюда, потому что оба файла о памяти и логично читаются вместе.

## Вопросы senior-уровня

- почему goroutine дешевле OS-потока по памяти и что происходит при переполнении стека
- как устроен Go аллокатор: почему аллокация small objects почти без блокировок
- что такое escape analysis и почему `new(T)` не гарантирует heap allocation
- что такое noscan span и как он ускоряет GC
- что такое write barrier и зачем он нужен при concurrent GC
- как GOGC и GOMEMLIMIT взаимодействуют в контейнере
- когда sync.Pool полезен и когда нет
- что значит RSS vs VmSize и почему Kubernetes смотрит на RSS

## Подборка

- [A Guide to the Go Garbage Collector](https://go.dev/doc/gc-guide)
- [Go Memory Model](https://go.dev/ref/mem)
- [runtime/malloc.go (source)](https://github.com/golang/go/blob/master/src/runtime/malloc.go)
- [runtime/mheap.go (source)](https://github.com/golang/go/blob/master/src/runtime/mheap.go)
- [Getting to Go: The Journey of Go's Garbage Collector](https://go.dev/blog/ismmkeynote)
- [TCMalloc: Thread-Caching Malloc](https://goog-perftools.sourceforge.net/doc/tcmalloc.html) — прообраз Go аллокатора
