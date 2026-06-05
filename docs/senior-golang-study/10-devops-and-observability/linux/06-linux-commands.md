# Linux: команды для диагностики production

Практический справочник команд для troubleshooting backend-сервисов. Каждая команда — с реальным примером вывода. Теоретическая основа — в файлах `01–05` этого раздела.

## Содержание

- [Процессы](#процессы)
- [Файловые дескрипторы и открытые файлы](#файловые-дескрипторы-и-открытые-файлы)
- [Память](#память)
- [CPU](#cpu)
- [Сеть и сокеты](#сеть-и-сокеты)
- [Диск и I/O](#диск-и-io)
- [Системные события и логи](#системные-события-и-логи)
- [Системные вызовы: strace](#системные-вызовы-strace)
- [Быстрый troubleshooting workflow](#быстрый-troubleshooting-workflow)

---

## Процессы

### ps — снимок процессов

```bash
# Все процессы в BSD-формате (удобно для скриптов)
ps aux
```
```
USER       PID %CPU %MEM    VSZ   RSS TTY  STAT START   TIME COMMAND
root         1  0.0  0.0  21532  3512 ?    Ss   Apr22   0:01 /sbin/init
www-data  1234  1.2  2.1 812340 86420 ?    Sl   09:00   0:42 /app/myservice
www-data  1235  0.0  0.1  14568  4200 ?    S    09:00   0:00 /app/myservice (worker)
postgres  2100  0.3  1.5 298540 61200 ?    Ss   Apr20   5:12 postgres: main
```
- `VSZ` — виртуальная память (включает неиспользуемые mmap-регионы)
- `RSS` — резидентная (реально в RAM). У Go-сервиса VSZ >> RSS — это нормально.
- `STAT`: `S` — sleeping, `R` — running, `Z` — zombie, `D` — uninterruptible I/O wait

```bash
# Найти процессы по имени
pgrep -la myservice
```
```
1234 /app/myservice --config=/etc/myservice/config.yaml
1235 /app/myservice --worker
```

```bash
# Дерево процессов — видно parent/child отношения
pstree -p 1234
```
```
myservice(1234)─┬─{myservice}(1236)
                ├─{myservice}(1237)
                ├─{myservice}(1238)
                └─{myservice}(1239)
```
У Go-бинарника потоки = горутины. `{myservice}` — OS threads (обычно GOMAXPROCS штук + несколько системных).

```bash
# Сколько тредов у процесса
cat /proc/1234/status | grep -E 'Threads|VmRSS|VmSize'
```
```
VmSize:   812340 kB
VmRSS:     86420 kB
Threads:       12
```

```bash
# Следить за процессом в реальном времени
top -p 1234
```
```
top - 10:15:03 up 2 days,  1:23,  1 user,  load average: 0.42, 0.38, 0.35
Tasks:   1 total,   0 running,   1 sleeping
%Cpu(s):  1.2 us,  0.3 sy,  0.0 ni, 98.5 id
MiB Mem :  7836.6 total,   923.4 free,  4210.0 used,  2703.2 buff/cache

  PID USER     PR  NI    VIRT    RES    SHR S  %CPU  %MEM     TIME+    COMMAND
 1234 www-data 20   0  793MB  84.4MB   5.1MB S   1.2   1.1   0:42.33  myservice
```

---

## Файловые дескрипторы и открытые файлы

Теория — в [02-file-descriptors-and-io.md](./02-file-descriptors-and-io.md).

### lsof — список открытых файлов

```bash
# Все открытые файлы и соединения процесса
lsof -p 1234
```
```
COMMAND  PID      USER   FD   TYPE     DEVICE SIZE/OFF     NODE NAME
myserv  1234  www-data  cwd    DIR      8,1     4096   131073  /app
myserv  1234  www-data  txt    REG      8,1  9437184   262144  /app/myservice
myserv  1234  www-data    0r   REG      8,1        0   524288  /dev/null
myserv  1234  www-data    1w   REG      8,1   102400   786432  /var/log/myservice.log
myserv  1234  www-data    2w   REG      8,1   102400   786432  /var/log/myservice.log
myserv  1234  www-data    3u  IPv4   1234567      0t0      TCP  *:8080 (LISTEN)
myserv  1234  www-data    4u  IPv4   1234568      0t0      TCP  10.0.1.5:54322->10.0.1.10:5432 (ESTABLISHED)
myserv  1234  www-data    5u  IPv4   1234569      0t0      TCP  10.0.1.5:54323->10.0.1.10:5432 (ESTABLISHED)
myserv  1234  www-data    6u  IPv4   1234570      0t0      TCP  10.0.1.5:54324->10.0.1.20:6379 (ESTABLISHED)
```
- `FD` 0,1,2 — stdin/stdout/stderr
- `3u` — fd 3, открыт для чтения и записи (u = read+write)
- `(LISTEN)` — серверный сокет
- `(ESTABLISHED)` — активные соединения (здесь: 2 к PostgreSQL, 1 к Redis)

```bash
# Подсчитать количество открытых fd
lsof -p 1234 | wc -l
```
```
248
```

```bash
# Просмотреть fd напрямую через /proc
ls -la /proc/1234/fd | head -20
```
```
total 0
lrwx------ 1 www-data www-data 64 Apr 23 09:00 0 -> /dev/null
lrwx------ 1 www-data www-data 64 Apr 23 09:00 1 -> /var/log/myservice.log
lrwx------ 1 www-data www-data 64 Apr 23 09:00 2 -> /var/log/myservice.log
lrwx------ 1 www-data www-data 64 Apr 23 09:00 3 -> socket:[1234567]
lrwx------ 1 www-data www-data 64 Apr 23 09:00 4 -> socket:[1234568]
```

```bash
# Текущий лимит fd для процесса
cat /proc/1234/limits | grep -i 'open files'
```
```
Limit                     Soft Limit  Hard Limit  Units
Max open files            65536       65536       files
```

```bash
# Системный лимит (для нового процесса)
ulimit -n
# 1024 (дефолт в большинстве систем — слишком мало для production!)
```

```bash
# Сколько fd занято во всей системе
cat /proc/sys/fs/file-nr
```
```
4896    0    9223372036854775807
# занято  свободно  максимум
```

```bash
# Какой процесс слушает на порту 8080
lsof -i :8080
# или
ss -tlnp | grep 8080
```
```
LISTEN 0  128  *:8080  *:*  users:(("myservice",pid=1234,fd=3))
```

---

## Память

Теория — в [01-virtual-memory.md](./01-virtual-memory.md).

```bash
# Общая картина памяти системы
free -h
```
```
               total        used        free      shared  buff/cache   available
Mem:           7.7Gi       4.1Gi       902Mi       123Mi       2.7Gi       3.3Gi
Swap:          2.0Gi         0B        2.0Gi
```
- `available` — сколько реально можно выделить (free + reclaimable cache)
- `buff/cache` занят page cache — это нормально, ядро отдаст при необходимости
- Swap > 0 — проблема для latency-sensitive сервисов

```bash
# Детальная статистика памяти
cat /proc/meminfo | head -20
```
```
MemTotal:        8024064 kB
MemFree:          923456 kB
MemAvailable:    3379200 kB
Buffers:          156840 kB
Cached:          2532968 kB
SwapCached:            0 kB
Active:          3210484 kB
Inactive:        2012348 kB
AnonPages:       2189076 kB
Mapped:           453720 kB
Shmem:            127016 kB
KReclaimable:     387584 kB
Slab:             498432 kB
```

```bash
# Карта памяти конкретного процесса
cat /proc/1234/smaps_rollup
```
```
00400000-ffffffff r--p 00000000 00:00 0  [rollup]
Rss:               86420 kB
Pss:               74210 kB
Shared_Clean:       4200 kB
Shared_Dirty:          0 kB
Private_Clean:      3820 kB
Private_Dirty:     78400 kB
Referenced:        86420 kB
Anonymous:         82200 kB
AnonHugePages:     20480 kB
```
- `Rss` — суммарная резидентная память
- `Pss` (Proportional Set Size) — честнее: shared страницы делятся на число процессов
- `Anonymous` — heap, stack (не файлы)

```bash
# Динамика памяти и swap (обновление каждую секунду)
vmstat 1 5
```
```
procs -----------memory---------- ---swap-- -----io---- -system-- ------cpu-----
 r  b   swpd   free   buff  cache   si   so    bi    bo   in   cs us sy id wa
 1  0      0 923456 156840 2532968    0    0     2     8  420  890  3  1 96  0
 0  0      0 922100 156840 2534100    0    0     0    24  380  820  1  0 99  0
 0  0      0 921800 156840 2534200    0    0     0     0  410  860  2  1 97  0
```
- `si`/`so` (swap in/out) > 0 — активный swap, проблема!
- `wa` (CPU iowait) высокий — процессы ждут диска
- `cs` (context switches) — переключения контекста

```bash
# Убийца OOM — кого убил?
dmesg | grep -i 'oom\|killed' | tail -10
```
```
[1234567.123] Out of memory: Kill process 5678 (myservice) score 892 or sacrifice child
[1234567.124] Killed process 5678 (myservice) total-vm:812340kB, anon-rss:786432kB, file-rss:2048kB
```

```bash
# OOM score процесса (0-1000; чем выше — тем вероятнее убьётся первым)
cat /proc/1234/oom_score
# 142

# Вручную снизить OOM score (нужны права root)
echo -500 > /proc/1234/oom_score_adj
```

---

## CPU

```bash
# Загрузка CPU по ядрам (нажать 1 в top для раскрытия)
top
```
```
top - 10:20:01 up 2 days,  1:28,  1 user,  load average: 2.14, 1.87, 1.62
Tasks: 182 total,   2 running, 180 sleeping
%Cpu0  :  45.3 us,  4.2 sy,  0.0 ni, 50.5 id,  0.0 wa
%Cpu1  :  38.1 us,  3.8 sy,  0.0 ni, 58.1 id,  0.0 wa
%Cpu2  :   2.1 us,  0.9 sy,  0.0 ni, 97.0 id,  0.0 wa
%Cpu3  :   1.8 us,  0.8 sy,  0.0 ni, 97.4 id,  0.0 wa
```
- `us` — user space, `sy` — kernel, `wa` — ожидание I/O
- load average > кол-ва ядер → очередь runnable-процессов растёт

```bash
# Детальная статистика CPU по ядрам
mpstat -P ALL 1 3
```
```
Average:     CPU    %usr   %nice    %sys %iowait    %irq   %soft  %idle
Average:     all    12.3    0.0     1.8     0.2     0.1     0.3   85.3
Average:       0    45.3    0.0     4.2     0.0     0.2     0.5   49.8
Average:       1    38.1    0.0     3.8     0.0     0.2     0.4   57.5
Average:       2     2.1    0.0     0.9     0.5     0.0     0.1   96.4
```

```bash
# CPU-профиль по процессам (top по CPU за период)
pidstat -u 1 5
```
```
Average:      UID       PID    %usr %system  %guest   %wait    %CPU   CPU  Command
Average:     1000      1234   35.2     3.1     0.0     0.1    38.3     0  myservice
Average:     1000      2100    2.1     0.4     0.0     0.0     2.5     1  postgres
```

```bash
# Количество переключений контекста у процесса
cat /proc/1234/status | grep ctxt
```
```
voluntary_ctxt_switches:     142380   ← процесс сам отдаёт CPU (ждёт I/O)
nonvoluntary_ctxt_switches:    1823   ← вытеснен планировщиком (много = CPU contention)
```

```bash
# Горячие функции в реальном времени (требует perf)
perf top -p 1234
```
```
Samples: 12K of event 'cycles', 4000 Hz, Event count (approx.): 2134567890
Overhead  Shared Object      Symbol
  18.34%  myservice          runtime.mallocgc
  12.21%  myservice          runtime.gcAssistAlloc
   9.87%  myservice          encoding/json.Marshal
   7.12%  [kernel]           __copy_user_nocache
   5.43%  myservice          net/http.(*ServeMux).ServeHTTP
```
GC и `mallocgc` наверху — частые аллокации, стоит запустить pprof.

---

## Сеть и сокеты

Теория — в [03-tcp-sockets.md](./03-tcp-sockets.md).

### ss — статистика сокетов (замена netstat)

```bash
# Все TCP-сокеты с процессами
ss -tnp
```
```
State    Recv-Q  Send-Q  Local Address:Port    Peer Address:Port   Process
LISTEN   0       128          0.0.0.0:8080          0.0.0.0:*      users:(("myservice",pid=1234,fd=3))
LISTEN   0       128          0.0.0.0:5432          0.0.0.0:*      users:(("postgres",pid=2100,fd=5))
ESTAB    0       0       10.0.1.5:54322      10.0.1.10:5432        users:(("myservice",pid=1234,fd=4))
ESTAB    0       0       10.0.1.5:54323      10.0.1.10:5432        users:(("myservice",pid=1234,fd=5))
ESTAB    0       0       10.0.1.5:43210      10.0.2.15:8080        users:(("myservice",pid=1234,fd=8))
TIME-WAIT 0      0       10.0.1.5:8080       10.0.2.20:52341
```

```bash
# Краткая сводка по состояниям TCP
ss -s
```
```
Total: 342
TCP:   287 (estab 241, closed 18, orphaned 0, timewait 28)

Transport Total     IP        IPv6
RAW	      0         0         0
UDP	      8         6         2
TCP	      269       220       49
```

```bash
# Все ESTABLISHED соединения к PostgreSQL (порт 5432)
ss -tnp dst :5432
```
```
State   Recv-Q Send-Q Local Address:Port  Peer Address:Port  Process
ESTAB   0      0      10.0.1.5:54322     10.0.1.10:5432     users:(("myservice",pid=1234,fd=4))
ESTAB   0      0      10.0.1.5:54323     10.0.1.10:5432     users:(("myservice",pid=1234,fd=5))
```

```bash
# Подсчёт соединений по состоянию
ss -tan | awk 'NR>1 {print $1}' | sort | uniq -c | sort -rn
```
```
241 ESTAB
 28 TIME-WAIT
 18 CLOSE-WAIT    ← если много — утечка: приложение не закрывает соединения
  4 LISTEN
  2 SYN-SENT
```

```bash
# Accept backlog: сколько запросов ждёт accept()
ss -tlnp
```
```
State  Recv-Q  Send-Q  Local Address:Port
LISTEN     0      128       0.0.0.0:8080     ← Recv-Q=0 (очередь пуста)
LISTEN    43      128       0.0.0.0:9090     ← Recv-Q=43 (backlog заполняется!)
```
Recv-Q > 0 у LISTEN = сервер не успевает вызывать accept(). Увеличить GOMAXPROCS или пул воркеров.

### tcpdump — захват трафика

```bash
# Трафик на порту 8080 (первые 100 пакетов)
tcpdump -i eth0 -n 'tcp port 8080' -c 100
```
```
10:25:01.123456 IP 10.0.2.20.52341 > 10.0.1.5.8080: Flags [S], seq 123456789, win 65535
10:25:01.123789 IP 10.0.1.5.8080 > 10.0.2.20.52341: Flags [S.], seq 987654321, ack 123456790, win 65535
10:25:01.124012 IP 10.0.2.20.52341 > 10.0.1.5.8080: Flags [.], ack 1, win 512
10:25:01.124234 IP 10.0.2.20.52341 > 10.0.1.5.8080: Flags [P.], seq 1:145, ack 1, win 512
```
Флаги: `S` = SYN, `S.` = SYN-ACK, `.` = ACK, `P` = PSH (данные), `F` = FIN, `R` = RST

```bash
# Захватить в файл для анализа в Wireshark
tcpdump -i eth0 -n 'tcp port 5432' -w /tmp/postgres.pcap

# Записать с полным содержимым пакетов
tcpdump -i eth0 -n -s 0 'tcp port 8080' -w /tmp/http.pcap
```

```bash
# Только RST-пакеты (аномальные сбросы соединений)
tcpdump -i eth0 -n 'tcp[tcpflags] & tcp-rst != 0'
```

### curl с таймингами

```bash
# Detailed timing breakdown HTTP запроса
curl -o /dev/null -s -w \
"dns_lookup:    %{time_namelookup}s\n\
tcp_connect:   %{time_connect}s\n\
tls_handshake: %{time_appconnect}s\n\
ttfb:          %{time_starttransfer}s\n\
total:         %{time_total}s\n\
http_code:     %{http_code}\n" \
http://myservice:8080/api/orders
```
```
dns_lookup:    0.001234s
tcp_connect:   0.002345s
tls_handshake: 0.000000s
ttfb:          0.045678s
total:         0.046123s
http_code:     200
```
`ttfb` (time to first byte) = время обработки на сервере + сетевой RTT.

---

## Диск и I/O

```bash
# Использование дисков
df -h
```
```
Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1        50G   38G  9.5G  80%  /
/dev/sdb1       200G  145G   50G  74%  /data
tmpfs           3.9G  123M  3.8G   4%  /run
```

```bash
# Топ директорий по размеру
du -sh /var/log/* | sort -rh | head -10
```
```
4.2G    /var/log/myservice
1.8G    /var/log/nginx
512M    /var/log/postgresql
```

```bash
# I/O активность по дискам в реальном времени
iostat -xz 1 3
```
```
Device  r/s   w/s  rkB/s  wkB/s  await  r_await  w_await  util
sda     2.1  45.3   84.0  362.4    1.2      0.8      1.3   8.3%
sdb     0.3  120.5   4.8  962.0    4.5      1.2      4.7  45.2%
```
- `await` — среднее время ожидания I/O (мс). > 10ms для SSD — проблема.
- `util` — загрузка устройства. > 80% — насыщение.

```bash
# Какой процесс генерирует I/O (нужен root или CAP_SYS_ADMIN)
iotop -o -P
```
```
Total DISK READ:       0.00 B/s | Total DISK WRITE:     24.56 M/s
  PID  PRIO  USER  DISK READ  DISK WRITE  SWAPIN   IO>  COMMAND
 2100    be  postgres   0.00 B/s  18.45 M/s  0.00 %  2.3 %  postgres: WAL writer
 1234    be  www-data   0.00 B/s   4.12 M/s  0.00 %  0.5 %  myservice
```

---

## Системные события и логи

```bash
# Последние сообщения ядра (OOM, hardware errors)
dmesg -T | tail -30
```
```
[Tue Apr 23 09:15:23 2026] TCP: request_sock_TCP: Possible SYN flooding on port 8080. Sending cookies.
[Tue Apr 23 09:18:45 2026] Out of memory: Kill process 5678 (myservice) score 890 or sacrifice child
[Tue Apr 23 09:18:45 2026] Killed process 5678 (myservice) total-vm:812340kB, anon-rss:786000kB
```

```bash
# Логи systemd-сервиса
journalctl -u myservice -n 100 --no-pager
```
```
Apr 23 09:00:01 host systemd[1]: Started myservice.
Apr 23 09:00:02 host myservice[1234]: level=info msg="server started" addr=":8080"
Apr 23 09:15:41 host myservice[1234]: level=error msg="db connection failed" err="context deadline exceeded"
Apr 23 09:18:45 host systemd[1]: myservice.service: Main process exited, code=killed, status=9/KILL
Apr 23 09:18:46 host systemd[1]: myservice.service: Failed with result 'signal'.
```

```bash
# Стримить логи в реальном времени (как tail -f)
journalctl -u myservice -f

# Логи за последний час
journalctl -u myservice --since "1 hour ago"

# Только ошибки
journalctl -u myservice -p err

# Логи с конкретного времени
journalctl -u myservice --since "2026-04-23 09:00:00" --until "2026-04-23 10:00:00"
```

```bash
# Сколько раз сервис рестартовал
journalctl -u myservice | grep 'Started myservice' | wc -l
# 5  ← CrashLoop на продакшне
```

```bash
# История OOM-убийств
journalctl -k | grep -i 'killed process\|oom'
```

```bash
# Использование дискового пространства логами journald
journalctl --disk-usage
# Archived and active journals take up 1.2G in the file system.

# Очистить старые журналы (оставить последние 2 недели)
journalctl --vacuum-time=2weeks
```

---

## Системные вызовы: strace

Теория — в [02-file-descriptors-and-io.md](./02-file-descriptors-and-io.md).

```bash
# Трассировка запущенного процесса (attach по PID)
strace -p 1234 -f -e trace=network,read,write 2>&1 | head -50
```
```
[pid  1234] epoll_wait(5, [{EPOLLIN, {u32=8, u64=8}}], 128, -1) = 1
[pid  1234] read(8, "GET /api/orders HTTP/1.1\r\nHost: m"..., 4096) = 243
[pid  1234] write(4, "\0\0\0\x1c\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0", 25) = 25
[pid  1234] read(4, "\0\0\0\x1c\0\0\0\x02\0\0\0\0\0\0\0\0", 4096) = 28
[pid  1234] write(8, "HTTP/1.1 200 OK\r\nContent-Type: ap"..., 156) = 156
```

```bash
# Сводка по системным вызовам: что занимает время
strace -p 1234 -c -f 2>&1
# ^C через несколько секунд
```
```
% time     seconds  usecs/call     calls    errors syscall
------ ----------- ----------- --------- --------- ----------------
 45.23    0.045230          45      1012           epoll_wait
 23.12    0.023120          10      2312       124 futex
 15.34    0.015340           5      3068           read
  8.91    0.008910           6      1485           write
  3.45    0.003450           3      1150           nanosleep
  2.12    0.002120          15       141           openat
```
`futex` занимает много → mutex contention. `epoll_wait` — ожидание событий (нормально).

```bash
# Трассировка только сигналов
strace -p 1234 -e trace=signal
```
```
rt_sigaction(SIGTERM, {sa_handler=0x4a5b60, sa_flags=SA_RESTORER, sa_restorer=0x7f...}, NULL, 8) = 0
--- SIGTERM {si_signo=SIGTERM, si_code=SI_USER, si_pid=1, si_uid=0} ---
rt_sigreturn({mask=[]})                 = 0
```

---

## Быстрый troubleshooting workflow

### Сервис не отвечает

```bash
# 1. Процесс вообще жив?
pgrep -la myservice
# Нет → смотреть journalctl

# 2. Слушает ли на порту?
ss -tlnp | grep 8080
# Нет → процесс упал или не успел стартовать

# 3. Есть ли соединения?
ss -tnp | grep 8080

# 4. Логи последних событий
journalctl -u myservice -n 50 --no-pager

# 5. Был ли OOMKilled?
dmesg -T | grep -i killed | tail -5
```

### Высокое потребление памяти

```bash
# 1. Сколько реально съел процесс?
cat /proc/1234/status | grep -E 'VmRSS|VmSize|VmSwap'

# 2. Динамика роста (раз в секунду)
while true; do
  date
  cat /proc/1234/status | grep VmRSS
  sleep 1
done

# 3. Детали по регионам памяти
cat /proc/1234/smaps_rollup

# 4. Активен ли swap?
vmstat 1 5 | awk '{print $7, $8}'  # si, so
```

### CPU spike

```bash
# 1. Кто жрёт CPU?
top -b -n1 | head -20

# 2. Где внутри процесса?
perf top -p 1234 -g  # с call graph

# 3. Много ли context switch'ей?
cat /proc/1234/status | grep ctxt_switches

# 4. GC pressure? (только для Go — через pprof)
curl -s http://localhost:6060/debug/pprof/goroutine?debug=1 | head -20
```

### Проблемы с сетью / высокая latency

```bash
# 1. Состояние соединений
ss -tnp | grep ':5432\|:6379\|:8080'

# 2. Много CLOSE_WAIT? (утечка: не закрываем соединения)
ss -tan | grep CLOSE-WAIT | wc -l

# 3. Очереди на backlog?
ss -tlnp   # Recv-Q у LISTEN

# 4. DNS работает?
time dig +short myservice-db.svc.cluster.local

# 5. Потери пакетов?
ping -c 100 10.0.1.10 | tail -3
```

### Кончились файловые дескрипторы

```bash
# Симптом в логах: "too many open files"

# 1. Текущее использование
cat /proc/1234/limits | grep 'open files'
ls /proc/1234/fd | wc -l

# 2. Что открыто?
lsof -p 1234 | awk '{print $5}' | sort | uniq -c | sort -rn
# Много IPv4 → соединения; много REG → файлы

# 3. Посмотреть на CLOSE_WAIT соединения (не закрыты)
lsof -p 1234 | grep CLOSE_WAIT

# 4. Временно поднять лимит
ulimit -n 65536
# Постоянно — в /etc/security/limits.conf или systemd unit:
# LimitNOFILE=65536
```

### Быстрая таблица симптом → команда

| Симптом | Команда |
|---|---|
| Процесс умер | `journalctl -u svc -n 50` + `dmesg \| grep killed` |
| OOMKilled | `dmesg -T \| grep -i oom` + `cat /proc/PID/status \| grep VmRSS` |
| CPU 100% | `top -p PID` + `perf top -p PID` |
| Память растёт | `vmstat 1` + `cat /proc/PID/smaps_rollup` |
| Slow queries / high latency | `ss -tnp` + `tcpdump -i eth0 'tcp port 5432'` |
| Too many open files | `lsof -p PID \| wc -l` + `cat /proc/PID/limits` |
| Порт уже занят | `ss -tlnp \| grep :8080` |
| CLOSE_WAIT накапливаются | `ss -tan \| grep CLOSE-WAIT \| wc -l` |
| Диск заполнен | `df -h` + `du -sh /var/log/* \| sort -rh \| head` |
| Высокий iowait | `iostat -xz 1` + `iotop -o` |
