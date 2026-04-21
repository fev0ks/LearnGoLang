# Linux Internals

Ядерные механизмы Linux, которые нужно понимать backend инженеру. Без этих знаний сложно объяснить, как работают Docker и Kubernetes, почему SIGTERM не всегда работает, как Go обрабатывает 10k соединений, почему контейнер получает OOMKilled.

## Материалы

- [Namespaces и cgroups](./05-namespaces-and-cgroups.md) — все 8 типов namespaces с механикой; cgroups v1 vs v2; cpu.max, memory.max, PSI; как Docker собирает контейнер из этих примитивов
- [Signals и процессы](./04-signals-and-processes.md) — таблица сигналов; PID 1 special behavior; zombie/orphan; `docker stop` sequence; Kubernetes grace period; Go signal handling
- [File Descriptors и I/O модели](./02-file-descriptors-and-io.md) — fd таблицы; ulimit; blocking / select / poll / epoll; Go netpoller на базе epoll; io_uring
- [Virtual Memory](./01-virtual-memory.md) — page tables; page fault; mmap; page cache; copy-on-write; OOM killer; overcommit; Go heap и GOMEMLIMIT
- [TCP Сокеты](./03-tcp-sockets.md) — socket lifecycle; TCP states; accept backlog; TIME_WAIT; CLOSE_WAIT; SO_REUSEPORT; Nagle / TCP_NODELAY; socket buffers; sysctl tuning

## Что важно уметь объяснить

**Контейнеры**
- Контейнер = Linux-процесс в namespaces + cgroup limits + overlay FS. Ядро хоста shared.
- pid namespace: свой счётчик PID от 1. PID 1 нет default signal handlers.
- cgroups v2: `cpu.max` (CFS bandwidth), `memory.max` (hard limit + OOM), PSI.

**Сигналы и процессы**
- SIGTERM можно поймать; SIGKILL — нельзя.
- Go при PID 1 молча игнорирует SIGTERM без явного `signal.NotifyContext`.
- Zombie = дочерний завершился, родитель не вызвал `wait()`. PID 1 reap orphans.

**I/O и file descriptors**
- epoll: O(готовых событий) vs O(n) для select/poll. Red-black tree для регистрации.
- Go netpoller: горутина паркуется при EAGAIN, OS thread свободен, epoll wakeup → runnable.
- `ulimit -n` = лимит fd на процесс. 10k соединений требуют ≥ 10k fd.

**Память**
- Page fault: minor (страница в RAM, нужно добавить в page table) vs major (читать с диска).
- Page cache занимает всю свободную RAM — это нормально и хорошо.
- Go heap через anonymous mmap; `MADV_DONTNEED` возвращает физические страницы; `GOMEMLIMIT` защищает от OOMKilled.

**TCP**
- TIME_WAIT: 2*MSL ≈ 60s, нужен для надёжного закрытия. Тысячи — нормально.
- CLOSE_WAIT: всегда баг — приложение не вызвало close() после получения FIN.
- Nagle буферизует маленькие write(); `TCP_NODELAY` отключает (нужен для API и Redis).
- SO_REUSEPORT: несколько сокетов на одном порту, ядро балансирует — основа nginx multi-worker.
