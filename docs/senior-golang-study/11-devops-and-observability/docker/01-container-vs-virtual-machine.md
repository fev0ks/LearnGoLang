# Container vs Virtual Machine

Один из самых частых инфраструктурных вопросов на backend интервью. Ответ "контейнер легче VM" — поверхностный. Senior должен понимать ядерные примитивы.

## Содержание

- [Linux primitives: namespaces и cgroups](#linux-primitives-namespaces-и-cgroups)
- [Namespaces: изоляция видимости](#namespaces-изоляция-видимости)
- [cgroups: ограничение ресурсов](#cgroups-ограничение-ресурсов)
- [Container Runtime: как Docker собирает всё вместе](#container-runtime-как-docker-собирает-всё-вместе)
- [Virtual Machine: другой уровень изоляции](#virtual-machine-другой-уровень-изоляции)
- [Сравнение](#сравнение)
- [Безопасность: container escape vs VM escape](#безопасность-container-escape-vs-vm-escape)
- [Kata Containers и gVisor](#kata-containers-и-gvisor)
- [Interview-ready answer](#interview-ready-answer)

## Linux primitives: namespaces и cgroups

Контейнер — это не магия Docker. Это **обычный Linux-процесс** с двумя наборами ограничений:
- **namespaces** — изолируют *видимость* (что процесс видит).
- **cgroups** — ограничивают *потребление* ресурсов (сколько может использовать).

Детальное описание всех 8 типов namespaces, cgroups v2 контроллеров и практических команд — в [linux/05-namespaces-and-cgroups.md](../linux/05-namespaces-and-cgroups.md).

```bash
# Посмотреть namespaces процесса
ls -la /proc/$(pgrep nginx)/ns/

# Показать cgroup для процесса
cat /proc/$(pgrep nginx)/cgroup
```

## Namespaces: изоляция видимости

Linux предоставляет 8 типов namespaces:

| Namespace | Изолирует |
|---|---|
| `pid` | Дерево процессов. Процесс в контейнере видит свои PID, начиная с 1. |
| `net` | Сетевые интерфейсы, маршруты, firewall rules. Контейнер имеет своё `eth0`. |
| `mnt` | Файловую систему (mount points). Контейнер видит свой `/`. |
| `uts` | Hostname и domainname. Контейнер может иметь своё имя хоста. |
| `ipc` | Межпроцессное взаимодействие (semaphores, shared memory). |
| `user` | UID/GID mapping. root в контейнере (UID 0) = обычный пользователь на хосте. |
| `cgroup` | Видимость cgroup hierarchy. |
| `time` | Системное время (Linux 5.6+). |

```bash
# Запустить процесс в новых namespaces (аналог docker run вручную)
unshare --pid --fork --mount-proc bash
# → bash запустится с PID 1 в изолированном pid namespace
```

Важно: **ядро хоста одно**. Контейнер использует те же системные вызовы (syscalls), что и обычный процесс. Namespaces только ограничивают, что он *видит*.

## cgroups: ограничение ресурсов

cgroups (control groups) — ограничивают и изолируют потребление ресурсов группой процессов.

cgroups v2 (unified hierarchy, современный стандарт):

```bash
# Docker создаёт cgroup при запуске контейнера
ls /sys/fs/cgroup/system.slice/docker-<container_id>.scope/

# Ограничения CPU
cat /sys/fs/cgroup/system.slice/docker-<id>.scope/cpu.max
# 50000 100000  ← 50% от одного CPU (50000 мкс из 100000 мкс периода)

# Ограничение памяти
cat /sys/fs/cgroup/system.slice/docker-<id>.scope/memory.max
# 536870912  ← 512 MB
```

Docker передаёт лимиты через `--memory` и `--cpus`:
```bash
docker run --memory=512m --cpus=0.5 my-go-service
```

Если процесс превышает `memory.max` — OOM killer убивает его. Контейнер падает с `OOMKilled`.

## Container Runtime: как Docker собирает всё вместе

```text
docker run → Docker Engine (daemon)
              → containerd
                → runc (OCI runtime)
                  → clone() syscall с флагами CLONE_NEWPID | CLONE_NEWNET | ...
                  → cgroup v2 limits
                  → overlay filesystem mount (image layers)
                  → exec() бинаря приложения
```

**OCI (Open Container Initiative)** — стандарт образов и runtime. Image в формате OCI работает в Docker, containerd, podman — они взаимозаменяемы.

**overlay filesystem**: Docker image — это набор слоёв (layer). Каждая инструкция `RUN`/`COPY` в Dockerfile создаёт новый слой. При запуске контейнера создаётся writable layer поверх read-only слоёв image.

```text
Container write layer (ephemeral)
─────────────────────────────────
Layer 3: COPY app /app          ← read-only
Layer 2: RUN apk add ca-certs   ← read-only
Layer 1: FROM scratch           ← read-only
```

Изменения в контейнере (создание файлов) попадают в writable layer. При удалении контейнера — writable layer исчезает. Данные нужно писать в volumes.

## Virtual Machine: другой уровень изоляции

```text
VM stack:
┌─────────────────────┐
│ Application         │
├─────────────────────┤
│ Guest OS (kernel)   │ ← собственное ядро, драйверы
├─────────────────────┤
│ Hypervisor (VMM)    │ ← KVM, VMware ESXi, Hyper-V
├─────────────────────┤
│ Host Hardware       │
└─────────────────────┘

Container stack:
┌─────────────────────┐
│ Application         │
├─────────────────────┤
│ Container runtime   │ ← runc, namespaces, cgroups
├─────────────────────┤
│ Host OS kernel      │ ← shared
├─────────────────────┤
│ Host Hardware       │
└─────────────────────┘
```

VM имеет **полностью виртуализированную hardware**: виртуальный CPU, виртуальный диск, виртуальная сеть. Guest OS общается с виртуальным железом через гипервизор.

Типы гипервизоров:
- **Type 1** (bare-metal): KVM, VMware ESXi, Xen. Работает прямо на железе. AWS EC2 использует KVM+Nitro.
- **Type 2** (hosted): VirtualBox, VMware Workstation. Работает поверх хостовой ОС. Медленнее.

## Сравнение

| | Container | Virtual Machine |
|---|---|---|
| Запуск | < 1 секунды | 5–60 секунд |
| Размер | Мегабайты | Гигабайты |
| Ядро | Shared с хостом | Собственное |
| Изоляция | Средняя (namespace) | Сильная (hardware) |
| Плотность | 100–1000 контейнеров на ноде | 10–50 VM |
| Overhead | Минимальный | 5–15% на виртуализацию |
| Portability | OCI image = любой runtime | Зависит от гипервизора |

Оба подхода не конкурируют: **Kubernetes ноды — это VM** (EC2 инстансы), а на них уже работают контейнеры.

## Безопасность: container escape vs VM escape

**Container escape** — это когда процесс в контейнере получает доступ к хостовой ОС. Возможно через:
- privileged mode (`--privileged` даёт все capabilities + доступ к устройствам).
- misconfigured volume mounts (монтирование `/` хоста).
- uже ядра Linux (kernel exploits — shared kernel = общая уязвимость).
- Docker socket mount (`/var/run/docker.sock` = root на хосте).

**VM escape** — намного сложнее. Нужно взломать гипервизор. Исторически редкие уязвимости (VENOM, Cloudbleed).

Для multi-tenant (разные клиенты на одном железе, как облачный хостинг) — VM обязательны. Облачные провайдеры запускают VM для каждого клиента, не контейнеры.

## Kata Containers и gVisor

Попытки совместить легковесность контейнеров с изоляцией VM:

**Kata Containers**: каждый контейнер запускается в лёгкой VM (KVM micro-VM). Overhead ~130ms старта, ~50MB RAM. Используется в Azure ACI, AWS Fargate.

**gVisor** (Google): user-space ядро на Go, перехватывает syscalls контейнера. Контейнер думает, что говорит с Linux kernel, но на самом деле — с gVisor. Защита без VM overhead. Используется в GKE Sandbox.

Для большинства backend сервисов стандартных namespaces + cgroups достаточно.

## Interview-ready answer

Контейнер — это Linux-процесс, изолированный через namespaces (ограничивают видимость: PID tree, network, filesystem) и cgroups (ограничивают ресурсы: CPU, RAM). Ядро хоста shared — это ключевое отличие от VM. VM имеет полностью виртуализированную hardware и собственный kernel, что даёт более сильную изоляцию, но дороже по ресурсам и медленнее стартует. Shared kernel — главный security риск контейнеров: уязвимость ядра = угроза для всех контейнеров на ноде. Практически: Kubernetes ноды — VM, на них бегут контейнеры. OCI — стандарт image и runtime, Docker/containerd/podman взаимозаменяемы.
