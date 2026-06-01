# Runtime Scheduler

Как Go runtime исполняет горутины поверх OS-потоков: планировщик, системные вызовы и сетевой I/O. Эти три темы тесно связаны — scheduler решает, кто исполняется, а syscall и netpoller объясняют, что происходит, когда горутина блокируется. Читать по порядку.

## Материалы

- [01. Scheduler And Preemption](./01-scheduler-and-preemption.md) — GMP модель (G/M/P), local/global run queues, work stealing, кооперативная и async preemption (SIGURG), GOMAXPROCS в контейнерах
- [02. Syscall](./02-syscall.md) — entersyscall/exitsyscall, P handoff, sysmon retake, fast/slow path, Syscall vs RawSyscall, цена CGo, LockOSThread, thread exhaustion
- [03. Netpoller](./03-netpoller.md) — epoll/kqueue/IOCP, pollDesc, parking/wakeup горутин, SetDeadline через timer heap, DNS resolver (Go vs CGo), диагностика соединений

## Связи между файлами

```
01 Scheduler  → как горутины распределяются по потокам (G/M/P, work stealing, preemption)
02 Syscall    → что происходит с P, когда горутина блокируется в ядре (handoff)
03 Netpoller  → почему сетевой I/O НЕ блокирует M (epoll + parking)
```

Ключевая связка: при **файловом/блокирующем** syscall M блокируется в ядре, а P через handoff отдаётся другому M (файл 02). При **сетевом** I/O горутина паркуется в netpoller, и M вообще не блокируется (файл 03) — отсюда тысячи соединений на горстке потоков.

## Вопросы senior-уровня

- объясни GMP модель: зачем нужен P, чем он отличается от M
- что такое work stealing и почему крадут с хвоста чужой очереди
- как работает async preemption и какую проблему она решила в Go 1.14
- что происходит с P, когда горутина уходит в blocking syscall
- чем file I/O отличается от network I/O с точки зрения scheduler
- почему 100k соединений не требуют 100k OS threads
- как GOMAXPROCS влияет на CPU throttling в контейнерах и зачем automaxprocs
- почему CGo-вызов дороже обычного syscall
- когда нужен LockOSThread и чем он опасен
- как SetDeadline реализован без дополнительных syscall

## Подборка

- [Go scheduler: implementing language with lightweight concurrency](https://www.youtube.com/watch?v=-K11rY57K7k) — Dmitry Vyukov
- [Scheduling In Go (3 части)](https://www.ardanlabs.com/blog/2018/08/scheduling-in-go-part1-os-scheduler.html) — Ardan Labs
- [runtime/proc.go (source)](https://github.com/golang/go/blob/master/src/runtime/proc.go)
- [runtime/netpoll.go (source)](https://github.com/golang/go/blob/master/src/runtime/netpoll.go)
- [The Go netpoller](https://morsmachine.dk/netpoller)
