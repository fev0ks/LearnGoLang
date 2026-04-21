# Virtual Memory

Каждый процесс видит свой изолированный адресный пространство. Понимание виртуальной памяти объясняет: почему Go-сервис потребляет больше памяти, чем кажется; как работает mmap; почему fork() дешёвый; как ядро кэширует файлы.

## Содержание

- [Виртуальное адресное пространство](#виртуальное-адресное-пространство)
- [Page tables: трансляция виртуальный → физический](#page-tables-трансляция-виртуальный--физический)
- [Page fault: minor vs major](#page-fault-minor-vs-major)
- [mmap: memory-mapped files и anonymous mappings](#mmap-memory-mapped-files-и-anonymous-mappings)
- [Page cache: ядро кэширует файловый I/O](#page-cache-ядро-кэширует-файловый-io)
- [Copy-on-Write и fork()](#copy-on-write-и-fork)
- [Huge pages: TLB и производительность](#huge-pages-tlb-и-производительность)
- [OOM killer: как ядро выбирает жертву](#oom-killer-как-ядро-выбирает-жертву)
- [Memory overcommit](#memory-overcommit)
- [Go и виртуальная память](#go-и-виртуальная-память)
- [Диагностика: /proc/\<pid\>/maps и smaps](#диагностика-procpidmaps-и-smaps)
- [Interview-ready answer](#interview-ready-answer)

---

## Виртуальное адресное пространство

На x86-64 каждый процесс имеет 128 TB виртуального адресного пространства (48-bit адрес, старшая половина — ядро):

```text
0xFFFF_FFFF_FFFF_FFFF ┐
                      │  Kernel space (не доступно user-space)
0xFFFF_8000_0000_0000 ┘
          ...           (non-canonical hole)
0x0000_7FFF_FFFF_FFFF ┐
                      │  Stack (растёт вниз)
                      │  ...
                      │  mmap region (shared libs, anonymous mmap, heap overflow)
                      │  ...
                      │  Heap (растёт вверх, через brk/mmap)
                      │  BSS (неинициализированные глобальные переменные)
                      │  Data (инициализированные глобальные переменные)
                      │  Text (код программы, read-only)
0x0000_0000_0040_0000 ┘  (обычный start)
```

Посмотреть карту адресного пространства процесса:
```bash
cat /proc/$(pgrep my-service)/maps
# 00400000-00401000 r-xp 00000000 08:01 123456 /app/server   ← code
# 00600000-00601000 r--p 00200000 08:01 123456 /app/server   ← rodata
# 00601000-00602000 rw-p 00201000 08:01 123456 /app/server   ← data
# 7f1234560000-7f1234580000 rw-p 00000000 00:00 0            ← heap/mmap
# 7ffdc0000000-7ffdc0200000 rw-p 00000000 00:00 0 [stack]
```

---

## Page tables: трансляция виртуальный → физический

Виртуальный адрес не является физическим адресом в RAM. CPU использует **MMU** (Memory Management Unit) и **page tables** для трансляции.

Страница — минимальная единица: **4 KB** (стандарт), 2 MB или 1 GB (huge pages).

На x86-64 четырёхуровневые page tables:

```text
Виртуальный адрес (48 бит):
  [PGD 9 бит][PUD 9 бит][PMD 9 бит][PTE 9 бит][offset 12 бит]
      │           │           │           │
      ▼           ▼           ▼           ▼
   Page Global  Page Upper  Page Middle  Page Table  Physical
   Directory    Directory   Directory    Entry       Address
  (уровень 4)  (уровень 3)  (уровень 2) (уровень 1)
```

Каждый процесс имеет свои page tables → изоляция: один и тот же виртуальный адрес в разных процессах указывает на разные физические страницы.

**TLB (Translation Lookaside Buffer)** — кэш трансляций в CPU. При каждом обращении к памяти MMU проверяет TLB. TLB miss → обход page tables → медленно.

Context switch (переключение процессов) → инвалидация TLB (частичная или полная через ASID) → TLB cold start → замедление в первые микросекунды после переключения.

---

## Page fault: minor vs major

**Page fault** — обращение к виртуальной странице, которой нет в page table или она помечена как недоступная.

### Minor page fault (мягкий)

Страница уже в физической памяти, просто ещё не замаплена в page table процесса.

Причины:
- Первое обращение к странице heap (lazy allocation — ядро не выделяет физическую память при `mmap`, только при записи).
- Страница в page cache (другой процесс читал тот же файл).
- После `fork()` при copy-on-write.

Обработка: ядро добавляет запись в page table. **Быстро** (~1 мкс).

### Major page fault (жёсткий)

Страница не в RAM — нужно читать с диска (из swap или из файла).

Причины:
- Своп (physical RAM исчерпана, страница вытеснена на диск).
- `mmap` файла, страница ещё не прочитана.
- Долго неиспользуемая страница (kernel swapped).

Обработка: дисковый I/O → **медленно** (1–10 мс).

```bash
# Статистика page faults процесса
/usr/bin/time -v ./my-program
# Major (requiring I/O) page faults: 5
# Minor (reclaiming a frame) page faults: 8423

# Для запущенного процесса
cat /proc/<pid>/stat | awk '{print "minor:", $10, "major:", $12}'
```

---

## mmap: memory-mapped files и anonymous mappings

`mmap()` — системный вызов для маппирования файла или anonymous memory в виртуальное адресное пространство.

### File-backed mmap

```c
int fd = open("data.bin", O_RDONLY);
void *addr = mmap(NULL, size, PROT_READ, MAP_PRIVATE, fd, offset);
// Теперь addr[i] читает байт из файла
// Чтение происходит через page cache — при первом обращении → major/minor fault
```

Использование: базы данных (LevelDB, RocksDB, SQLite в WAL mode), исполняемые файлы (ядро мапит ELF через mmap при exec).

### Anonymous mmap (heap)

```c
void *mem = mmap(NULL, size, PROT_READ|PROT_WRITE,
                 MAP_PRIVATE|MAP_ANONYMOUS, -1, 0);
```

Go runtime использует anonymous mmap для выделения арен GC (не `malloc`).

`MADV_DONTNEED` — advisory: сообщить ядру что страницы больше не нужны, можно освободить физическую память:
```c
madvise(addr, size, MADV_DONTNEED);
// Виртуальные страницы остаются (адрес не меняется)
// Физические страницы освобождаются
// При следующем доступе → minor page fault (обнулённая страница)
```

Go GC использует `MADV_DONTNEED` (или `MADV_FREE` на Linux 4.5+) для возврата неиспользуемых heap-страниц ОС.

---

## Page cache: ядро кэширует файловый I/O

**Page cache** — основной кэш ядра для содержимого файлов и блочных устройств. Занимает всю свободную RAM.

```text
Process read("data.bin")
    │
    ├─ Page в cache? → вернуть (никакого диска)
    │
    └─ Нет в cache:
        → читать с диска
        → сохранить в page cache
        → вернуть процессу
```

Запись работает аналогично:
- `write()` → данные попадают в page cache → помечаются `dirty`.
- Ядро периодически сбрасывает dirty pages на диск (pdflush/writeback).
- `fsync(fd)` → принудительный flush dirty pages для файла.
- `sync()` → flush всех dirty pages.

**Без fsync**: данные "записаны" (в page cache), но при падении питания они потеряются. Это почему PostgreSQL вызывает fsync при commit.

```bash
# Использование памяти с учётом page cache
free -h
#              total    used   free   shared  buff/cache  available
# Mem:          15Gi   3.2Gi   8.1Gi  234Mi    4.1Gi     11.6Gi
# ↑ "available" = free + buff/cache (kernel освободит под процесс если нужно)

# Сколько конкретно в page cache
cat /proc/meminfo | grep -E "Cached|Buffers|Dirty|Writeback"
# Buffers:         123456 kB   ← metadata
# Cached:         4200000 kB   ← page cache
# Dirty:            12345 kB   ← ещё не записано на диск
```

**Важно**: `free` показывает много "used" памяти из-за page cache. Это **нормально** и **хорошо** — ядро использует свободную RAM под кэш. При нехватке памяти под процессы — ядро выбросит page cache.

---

## Copy-on-Write и fork()

`fork()` создаёт полную копию процесса. Но копирование гигабайт heap было бы медленным.

**Copy-on-Write (CoW)**: после fork обе копии (родитель и ребёнок) используют **одни физические страницы** (read-only). При первой **записи** в любую страницу — ядро создаёт копию этой страницы для пишущего процесса.

```text
fork():
  Parent pages: A, B, C → все помечены read-only в page table обоих
  Child pages:  A, B, C → указывают на те же физические страницы

Child пишет в страницу B:
  Page fault → ядро копирует страницу B
  Child.B → новая физическая страница (можно писать)
  Parent.B → старая физическая страница (не затронута)
```

**Последствия для Go**: Go не использует `fork()` для горутин (не нужно), но CGO может. Redis (написан на C) использует `fork()` для BGSAVE — CoW делает снапшот дешёвым, но активная запись в Redis после fork вызывает копирование страниц → memory usage растёт.

---

## Huge pages: TLB и производительность

Стандартная страница: **4 KB**. Для маппирования 1 GB нужно 262144 page table entries + 262144 TLB entries.

**Huge pages**:
- **2 MB** (transparent huge pages, THP) — поддерживается автоматически.
- **1 GB** (explicit huge pages через hugetlbfs) — требует явной настройки.

Преимущество: меньше TLB entries → меньше TLB misses → меньше time на трансляцию адресов. Критично для приложений с большим рабочим набором данных (БД, JVM, Redis).

```bash
# Статус THP (Transparent Huge Pages)
cat /sys/kernel/mm/transparent_hugepage/enabled
# [always] madvise never
# "always" — ядро создаёт THP когда возможно
# "madvise" — только если процесс явно попросит через madvise MADV_HUGEPAGE
# "never" — отключены

# Статистика THP
cat /proc/meminfo | grep -i huge
# AnonHugePages:    204800 kB
# HugePages_Total:       0
# HugePages_Free:        0
```

**Проблема THP**: promotion (обычная → huge) и splitting (huge → обычные) требуют дефрагментации. Это вызывает latency spikes. Базы данных (MongoDB, Redis, PostgreSQL) рекомендуют отключать THP.

```bash
# Отключить THP для production DB
echo madvise > /sys/kernel/mm/transparent_hugepage/enabled
```

Go runtime не имеет особых настроек для huge pages, но автоматически выигрывает от THP при больших heap.

---

## OOM killer: как ядро выбирает жертву

Когда физическая память исчерпана и swap недоступен — ядро активирует **OOM (Out-of-Memory) killer**.

Killer выбирает процесс с наибольшим **oom_score**:

```text
oom_score = функция от:
  - RSS (resident set size) процесса — чем больше использует RAM, тем выше score
  - Nice value — high nice (низкий приоритет) → выше score
  - Runtime (давно работающие процессы чуть защищены)
  - oom_score_adj: от -1000 (защита от убийства) до +1000 (убить первым)
```

```bash
# Посмотреть score текущих процессов
cat /proc/<pid>/oom_score        # текущий score (0-1000)
cat /proc/<pid>/oom_score_adj    # adjustment (-1000 to 1000)

# Защитить критичный процесс от OOM killer
echo -500 > /proc/<pid>/oom_score_adj

# Сделать systemd сервис приоритетом для убийства
# /etc/systemd/system/myapp.service
# [Service]
# OOMScoreAdjust=500

# В контейнере: cgroup OOM killer
cat /sys/fs/cgroup/.../memory.events | grep oom
```

В Kubernetes OOMKill виден через `kubectl describe pod`:
```
Last State: Terminated
  Reason: OOMKilled
  Exit Code: 137   ← 128 + SIGKILL (9)
```

---

## Memory overcommit

Linux по умолчанию **overcommit** память: позволяет процессам запрашивать больше виртуальной памяти, чем есть физической.

```bash
cat /proc/sys/vm/overcommit_memory
# 0 = heuristic overcommit (default)
# 1 = always overcommit (позволяет запросить что угодно)
# 2 = never overcommit (strict: commit ≤ RAM + swap)
```

Режим 0 (heuristic): `mmap()` / `malloc()` всегда успешны. Физическая память выделяется при **первой записи** в страницу (lazy allocation). Если при записи физической памяти нет → OOM killer.

Последствие: `docker run --memory 512m` резервирует виртуальный адрес, но реальная физическая память выделяется по мере записи. `free()` / `MADV_DONTNEED` возвращает физические страницы ОС, виртуальный адрес остаётся.

Go allocator запрашивает большие арены через `mmap` (виртуальная память выглядит как "много" в `/proc/pid/status VmSize`), но реально использует (`VmRSS`) меньше.

---

## Go и виртуальная память

Go runtime управляет памятью самостоятельно — не через `malloc`/`free`.

```text
Go heap:
  Арены (64 MB на Linux) ← выделяются через mmap (anonymous, MAP_PRIVATE)
  └─ spans (8 KB)
     └─ objects (разные size classes)

GC работает:
  Mark phase: обходит объекты, помечает живые
  Sweep phase: освобождает мёртвые spans
  MADV_DONTNEED / MADV_FREE → возвращает физические страницы ОС
                                (виртуальный адрес остаётся)
```

Почему `docker stats` и `kubectl top` показывают разные значения:

```bash
# Что мониторинг обычно смотрит (cgroup memory.current):
cat /sys/fs/cgroup/.../memory.current
# → RSS + page cache + сjared memory

# /proc/<pid>/status
VmSize:  4096000 kB   ← виртуальное адресное пространство (большое, не страшно)
VmRSS:    512000 kB   ← реально в RAM (это важно)
VmSwap:        0 kB   ← в swap

# smaps — детально по регионам
cat /proc/<pid>/smaps | grep -A 6 "heap"
```

**GOGC vs GOMEMLIMIT**: два рычага управления памятью:
- `GOGC=100` (default): GC запускается когда heap вырос в 2 раза от live set после предыдущего GC. Увеличь `GOGC` → реже GC → больше heap.
- `GOMEMLIMIT=512MiB`: мягкий потолок heap. При приближении к лимиту GC становится агрессивнее независимо от GOGC. Защищает от OOMKilled.

Рекомендация: `GOMEMLIMIT = 90% от cgroup memory.max`. Оставляй запас на non-heap память (goroutine stacks, os/exec, CGO, mmap'ы).

```bash
# Посмотреть runtime статистику Go
# (GODEBUG=gctrace=1 при запуске или через expvar/pprof)
GODEBUG=gctrace=1 ./my-service 2>&1 | head -5
# gc 1 @0.012s 2%: 0.014+2.3+0.003 ms clock, ...
#    │         │              │
#    │         │              └─ sweep phase
#    │         └─ % CPU на GC (2% — хорошо, >10% — плохо)
#    └─ номер GC цикла
```

---

## Диагностика: /proc/\<pid\>/maps и smaps

```bash
# Карта всех регионов памяти
cat /proc/<pid>/maps
# address           perms offset dev inode  pathname
# 00400000-00600000 r-xp  ...        /app/server   ← код (RX, read+exec)
# 7f1234000000-...  rw-p  ...        [heap]
# 7f5678000000-...  rw-p  ...                       ← Go heap арены
# 7fff00000000-...  rw-p  ...        [stack]

# Детальная статистика по регионам (для оптимизации)
cat /proc/<pid>/smaps_rollup
# Rss:              512000 kB    ← реально в RAM
# Pss:              498000 kB    ← proportional (shared pages делятся)
# Private_Dirty:    480000 kB    ← частные изменённые страницы
# Shared_Dirty:       1024 kB    ← общие изменённые (shared libs)
# Anonymous:        480000 kB    ← не привязанные к файлу (heap, stack)
# Swap:                  0 kB

# Количество виртуальных регионов
cat /proc/<pid>/maps | wc -l
# Если > 65536 → ошибка при fork/exec в CGO

# Использование heap в Go через pprof
go tool pprof http://localhost:6060/debug/pprof/heap
# (top10 покажет где выделяется больше всего памяти)
```

---

## Interview-ready answer

Виртуальная память: каждый процесс имеет изолированное виртуальное адресное пространство; MMU транслирует через page tables (4 уровня на x86-64) в физическое. TLB кэширует трансляции; context switch инвалидирует TLB. Page fault: minor — страница уже в RAM, нужно добавить в page table (быстро); major — нужно читать с диска (медленно). mmap позволяет маппировать файлы и anonymous memory в адресное пространство; Go runtime использует anonymous mmap для heap арен. Page cache: ядро кэширует файловый I/O в свободной RAM, `fsync` принудительно сбрасывает dirty pages. Copy-on-write: после fork() обе копии используют одни физические страницы до первой записи. OOM killer выбирает по oom_score (пропорционально RSS); GOMEMLIMIT предотвращает OOMKilled для Go. Overcommit: `mmap`/`malloc` всегда успешны, физическая память выделяется при записи — поэтому VmSize > VmRSS.
