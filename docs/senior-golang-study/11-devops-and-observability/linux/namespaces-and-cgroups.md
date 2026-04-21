# Linux Namespaces и cgroups

Это ядерные примитивы, на которых построены контейнеры. Docker, containerd, podman — всё это обёртки поверх них.

## Содержание

- [Namespaces: изоляция видимости](#namespaces-изоляция-видимости)
  - [pid namespace](#pid-namespace)
  - [net namespace](#net-namespace)
  - [mnt namespace](#mnt-namespace)
  - [uts namespace](#uts-namespace)
  - [ipc namespace](#ipc-namespace)
  - [user namespace](#user-namespace)
  - [cgroup namespace](#cgroup-namespace)
  - [time namespace](#time-namespace)
- [cgroups v1 vs v2](#cgroups-v1-vs-v2)
- [cgroups v2: контроллеры и файловый интерфейс](#cgroups-v2-контроллеры-и-файловый-интерфейс)
- [Как Docker собирает всё вместе](#как-docker-собирает-всё-вместе)
- [Практические команды](#практические-команды)
- [Interview-ready answer](#interview-ready-answer)

---

## Namespaces: изоляция видимости

Namespace — это обёртка вокруг глобального системного ресурса, которая делает его **изолированным** для определённой группы процессов. Изменения внутри namespace не видны снаружи.

Процесс всегда принадлежит ровно одному namespace каждого типа. При `fork()` ребёнок наследует namespace родителя. `clone()` с флагами `CLONE_NEW*` создаёт новые namespaces.

```bash
# Namespaces текущего процесса — символические ссылки на inode namespace
ls -la /proc/self/ns/
# lrwxrwxrwx cgroup -> cgroup:[4026531835]
# lrwxrwxrwx ipc    -> ipc:[4026531839]
# lrwxrwxrwx mnt    -> mnt:[4026531841]
# lrwxrwxrwx net    -> net:[4026531840]
# lrwxrwxrwx pid    -> pid:[4026531836]
# lrwxrwxrwx user   -> user:[4026531837]
# lrwxrwxrwx uts    -> uts:[4026531838]
# lrwxrwxrwx time   -> time:[4026531834]

# Проверить: два процесса в одном namespace — одинаковый inode
ls -la /proc/1/ns/pid /proc/$(pgrep nginx | head -1)/ns/pid
```

---

### pid namespace

**Изолирует**: дерево процессов и нумерацию PID.

Каждый pid namespace имеет свой счётчик PID, начиная с 1. Процесс с PID 1 в контейнере — это init для этого namespace. Процессы снаружи namespace не видны изнутри.

```text
Host pid namespace:
  PID 1: systemd
  PID 1234: containerd
  PID 5678: sh (в контейнере)  ← хост видит этот PID

Container pid namespace:
  PID 1: sh  ← внутри контейнера это PID 1
```

**Вложенность**: pid namespaces могут быть вложены. Родительский namespace видит все процессы дочерних под их реальными PID. Дочерний не видит родительский.

**Zombie reaping**: PID 1 в namespace отвечает за вызов `wait()` для осиротевших процессов. Если PID 1 это Go-приложение без обработки `SIGCHLD` — осиротевшие процессы (от `os/exec`) станут зомби. Для этого используют `tini`.

```bash
# Запустить bash в новом pid namespace с PID 1
sudo unshare --pid --fork --mount-proc bash
echo $$  # → 1
ps aux   # → видно только bash
```

---

### net namespace

**Изолирует**: сетевой стек целиком — интерфейсы, таблицы маршрутизации, iptables/nftables, сокеты, порты.

Каждый net namespace имеет свой `lo`, может иметь свои интерфейсы. Один и тот же порт `8080` может быть занят в нескольких разных net namespaces одновременно.

**veth pair** — виртуальная пара Ethernet-интерфейсов. Один конец в контейнере (`eth0`), другой на хосте (`veth3f4a1b`). Пакет, отправленный в один конец, выходит из другого.

```text
Host net namespace:
  docker0 (bridge): 172.17.0.1
    └── veth3f4a1b ─────────────────┐
                                     │ veth pair
Container net namespace:             │
  eth0: 172.17.0.2 ─────────────────┘
  lo: 127.0.0.1
```

Docker bridge network: Docker создаёт bridge `docker0`, подключает к нему veth-пары всех контейнеров. NAT через iptables обеспечивает выход в интернет.

```bash
# Посмотреть интерфейсы контейнера с хоста
PID=$(docker inspect -f '{{.State.Pid}}' my-container)
sudo nsenter -t $PID --net -- ip addr
sudo nsenter -t $PID --net -- ip route
```

---

### mnt namespace

**Изолирует**: таблицу mount-точек процесса.

Каждый процесс видит свою mount table. Монтирование внутри namespace не видно снаружи (если не настроена propagation).

**Overlay filesystem** — механизм, на котором строятся image layers Docker:

```text
Overlay mount:
  upperdir  = /var/lib/docker/overlay2/<id>/diff   ← writable layer (контейнер)
  lowerdir  = layer3:layer2:layer1                 ← read-only image layers
  workdir   = /var/lib/docker/overlay2/<id>/work   ← служебная директория
  merged    = /var/lib/docker/overlay2/<id>/merged ← то, что видит контейнер

При записи в файл из lowerdir:
  1. Файл копируется из lowerdir в upperdir (copy-on-write)
  2. Изменения идут в upperdir
  3. При удалении контейнера — upperdir исчезает
```

**Bind mount** — монтирование директории хоста в контейнер:
```bash
docker run -v /host/path:/container/path myimage
# /host/path монтируется как bind mount в /container/path в mnt namespace контейнера
```

**Mount propagation** режимы:
- `private` (default): монтирования в контейнере не видны снаружи и наоборот.
- `shared`: изменения распространяются в обе стороны.
- `slave`: изменения от хоста видны в контейнере, обратно — нет.

---

### uts namespace

**Изолирует**: hostname и NIS domainname.

```bash
# Каждый контейнер может иметь своё hostname
docker run --hostname my-service alpine hostname
# → my-service

# На хосте hostname не изменился
hostname
# → my-server
```

Используется: при логировании внутри контейнера `os.Hostname()` возвращает имя контейнера (обычно container ID). В Kubernetes — имя Pod'а.

---

### ipc namespace

**Изолирует**: объекты System V IPC (message queues, semaphores, shared memory) и POSIX message queues.

Процессы в разных ipc namespaces не могут общаться через IPC-механизмы. Важно для изоляции приложений, использующих shared memory.

```bash
# Shared memory сегменты в текущем namespace
ipcs -m
# После docker run — отдельный namespace, свой список
```

Для Go-сервисов напрямую не актуально (Go не использует System V IPC), но важно при запуске legacy приложений в контейнере.

---

### user namespace

**Изолирует**: UID/GID — отображение пользователей между namespace и хостом.

Это позволяет создать **rootless containers**: root (UID 0) внутри namespace соответствует непривилегированному пользователю на хосте (например, UID 1000).

```text
Внутри контейнера:   UID 0 (root)
На хосте:            UID 65534 (nobody)  ← через /proc/<pid>/uid_map
```

```bash
cat /proc/$(docker inspect -f '{{.State.Pid}}' my-container)/uid_map
# 0 1000 1 → UID 0 в ns = UID 1000 на хосте, только 1 mapping
```

**Capabilities** в user namespace: процесс может иметь capabilities (например, `CAP_NET_ADMIN`) внутри своего namespace, но это не даёт реальных привилегий на хосте.

**Rootless Docker** (Docker ≥ 20.10, rootless mode) и **Podman** (rootless по умолчанию) используют user namespace — весь Docker daemon работает без root.

---

### cgroup namespace

**Изолирует**: видимость иерархии cgroups.

Без cgroup namespace контейнер мог бы увидеть всё дерево `/sys/fs/cgroup` хоста — включая ресурсы других контейнеров. С cgroup namespace корень иерархии cgroup внутри контейнера выглядит как `/`, хотя на хосте это глубоко вложенная директория.

```bash
# Без cgroup namespace в контейнере было бы видно:
cat /proc/self/cgroup
# 0::/system.slice/docker-<long-id>.scope/

# С cgroup namespace в контейнере:
cat /proc/self/cgroup
# 0::/   ← выглядит как корень
```

---

### time namespace

**Изолирует**: монотонные часы (`CLOCK_MONOTONIC`, `CLOCK_BOOTTIME`) — позволяет сдвигать время для группы процессов.

Добавлен в Linux 5.6 (март 2020). Применяется при:
- **Container migration** (CRIU): при восстановлении контейнера на другом хосте монотонные часы сбрасываются. Time namespace позволяет сохранить относительное время.
- Тестирование time-sensitive кода.

`CLOCK_REALTIME` (wall clock) намеренно **не** изолируется — изменение реального времени было бы опасно.

На практике в большинстве production-сценариев не используется. Docker и containerd не создают time namespace по умолчанию.

---

## cgroups v1 vs v2

### cgroups v1 (legacy, но ещё встречается)

Отдельная иерархия на каждый контроллер:
```
/sys/fs/cgroup/
  memory/
    docker/
      <container-id>/
        memory.limit_in_bytes
        memory.usage_in_bytes
  cpu/
    docker/
      <container-id>/
        cpu.cfs_quota_us
        cpu.cfs_period_us
  blkio/
    ...
```

Проблемы v1: сложное управление, нет unified resource accounting, трудно делать atomically.

### cgroups v2 (unified hierarchy, современный стандарт)

Единое дерево, все контроллеры в одном месте:
```
/sys/fs/cgroup/
  system.slice/
    docker-<container-id>.scope/
      cgroup.controllers    ← доступные контроллеры
      cpu.max               ← CPU limit
      memory.max            ← memory limit
      pids.max              ← process limit
      io.max                ← I/O limit
      memory.pressure       ← PSI
```

Статус: Ubuntu 22.04+, RHEL 9+, Debian 11+ — v2 по умолчанию. Docker поддерживает v2 начиная с версии 20.10.

---

## cgroups v2: контроллеры и файловый интерфейс

### CPU controller

```bash
cat /sys/fs/cgroup/system.slice/docker-<id>.scope/cpu.max
# 50000 100000
# Формат: <quota> <period> в микросекундах
# 50000/100000 = 50% одного CPU
# "max 100000" = без ограничений

# Статистика CPU
cat /sys/fs/cgroup/system.slice/docker-<id>.scope/cpu.stat
# usage_usec 1234567    ← суммарное CPU время (мкс)
# user_usec 987654
# system_usec 246913
# throttled_usec 56789  ← время, проведённое под throttling
```

**CFS (Completely Fair Scheduler)**: реализует CPU bandwidth control. При достижении `quota` из `period` — процессы группы переходят в throttled state до следующего периода.

Важно для Go: если процесс throttled — горутинный планировщик не может запустить работу. Это проявляется как latency spikes, не как высокий CPU. GOMAXPROCS по числу логических CPU хоста усугубляет — много потоков конкурируют за quota.

### Memory controller

```bash
# Лимиты
cat /sys/fs/cgroup/.../memory.max       # hard limit (OOM при превышении)
cat /sys/fs/cgroup/.../memory.high      # soft limit (замедление allocations)
cat /sys/fs/cgroup/.../memory.swap.max  # swap limit

# Текущее состояние
cat /sys/fs/cgroup/.../memory.current   # текущее использование
cat /sys/fs/cgroup/.../memory.stat      # детальная статистика

# События
cat /sys/fs/cgroup/.../memory.events
# low 0
# high 3         ← сколько раз превышали memory.high
# max 0
# oom 1          ← сколько раз срабатывал OOM killer
# oom_kill 1
```

**OOM behavior**: при превышении `memory.max` ядро вызывает OOM killer для процессов в группе. Docker помечает контейнер как `OOMKilled`. В Kubernetes — `kubectl describe pod` покажет OOMKilled event.

**memory.high** vs **memory.high**: `memory.high` — мягкий лимит. При его превышении ядро начинает throttle allocations и агрессивнее запускает GC (в том числе в runtimes, которые используют `mallinfo`). Go не реагирует на `memory.high` автоматически — нужен `GOMEMLIMIT`.

### pids controller

```bash
cat /sys/fs/cgroup/.../pids.max    # максимальное число процессов/потоков
cat /sys/fs/cgroup/.../pids.current

# docker run --pids-limit 100 → pids.max = 100
```

Защита от fork bombs. Go создаёт OS-потоки (M в runtime), каждый считается как pid. При высоком GOMAXPROCS или большом числе goroutines, заблокированных на системных вызовах, можно упереться в pids.max.

### PSI (Pressure Stall Information)

```bash
cat /sys/fs/cgroup/.../cpu.pressure
# some avg10=0.00 avg60=0.00 avg300=0.00 total=0
# full avg10=0.00 avg60=0.00 avg300=0.00 total=0
```

- `some`: % времени, когда хотя бы один task ждал ресурс.
- `full`: % времени, когда **все** tasks ждали ресурс (полная остановка).
- `avg10/avg60/avg300`: скользящее среднее за 10s/60s/300s.

PSI позволяет обнаружить **CPU throttling**, **memory pressure**, **I/O stall** до того, как произойдёт OOM или деградация. Используется в Kubernetes node pressure eviction.

---

## Как Docker собирает всё вместе

Последовательность при `docker run`:

```text
1. docker CLI → Docker daemon (REST API)

2. Docker daemon → containerd (gRPC)
   "Создай контейнер из image X с параметрами Y"

3. containerd → image pull/unpack (overlay layers)

4. containerd → runc (OCI runtime, порождает контейнер)

5. runc делает clone() с флагами:
   CLONE_NEWPID | CLONE_NEWNET | CLONE_NEWMNT |
   CLONE_NEWUTS | CLONE_NEWIPC | CLONE_NEWCGROUP
   (user namespace — опционально, в rootless режиме)

6. runc настраивает cgroups v2:
   mkdir /sys/fs/cgroup/system.slice/docker-<id>.scope/
   echo "50000 100000" > .../cpu.max
   echo "536870912"    > .../memory.max

7. runc монтирует overlay filesystem:
   mount -t overlay overlay
     -o lowerdir=layer3:layer2:layer1,upperdir=diff,workdir=work
     merged/

8. runc bind-монтирует /etc/resolv.conf, /etc/hosts в mnt namespace

9. runc exec() бинаря приложения → он становится PID 1 в pid namespace

10. После exec() — runc завершается, контейнер живёт самостоятельно
```

---

## Практические команды

```bash
# Посмотреть все namespaces процесса
ls -la /proc/<pid>/ns/

# Войти в namespace запущенного контейнера (требует root)
PID=$(docker inspect -f '{{.State.Pid}}' my-container)
sudo nsenter -t $PID --pid --net --mnt -- bash

# Только в net namespace (не влияет на файловую систему)
sudo nsenter -t $PID --net -- ip addr show

# Запустить процесс в новых namespaces вручную
sudo unshare --pid --fork --net --mount --uts --ipc --mount-proc bash

# Посмотреть cgroup процесса
cat /proc/<pid>/cgroup

# Найти cgroup контейнера
systemd-cgls /sys/fs/cgroup/system.slice/ | grep docker

# Текущее использование памяти контейнера
docker_cg="/sys/fs/cgroup/system.slice/docker-$(docker inspect -f '{{.Id}}' my-container).scope"
cat $docker_cg/memory.current
cat $docker_cg/memory.events

# CPU throttling статистика
cat $docker_cg/cpu.stat | grep throttled

# PSI метрики
cat $docker_cg/cpu.pressure
cat $docker_cg/memory.pressure

# Посмотреть overlay mount контейнера
docker inspect -f '{{.GraphDriver.Data}}' my-container

# Посмотреть veth pair на хосте
ip link | grep veth
```

---

## Interview-ready answer

Linux namespaces изолируют **видимость**: pid (дерево процессов), net (сетевой стек), mnt (файловая система), uts (hostname), ipc (IPC-объекты), user (UID/GID mapping), cgroup (видимость cgroup tree), time (монотонные часы). Каждый namespace создаётся через `clone()` с флагами `CLONE_NEW*`. cgroups ограничивают **потребление**: cpu.max (bandwidth), memory.max (hard limit + OOM), pids.max (fork bomb protection). cgroups v2 — unified hierarchy в `/sys/fs/cgroup/`, v1 — отдельная директория на контроллер. PSI (Pressure Stall Information) показывает % времени под давлением ресурса — диагностика без OOM. Контейнер = процесс в отдельных namespaces + cgroup limits + overlay filesystem. Docker/containerd используют runc для создания через `clone()` и настройки cgroups через filesystem interface.
