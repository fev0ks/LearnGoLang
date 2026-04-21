# TCP Сокеты и сетевой стек Linux

Понимание TCP на уровне ядра объясняет: почему сервер не принимает новые соединения; откуда тысячи TIME_WAIT; как SO_REUSEPORT позволяет нескольким воркерам принимать на одном порту; почему HTTP/2 может быть медленнее HTTP/1.1 при потерях.

## Содержание

- [Socket lifecycle: syscalls](#socket-lifecycle-syscalls)
- [TCP connection states](#tcp-connection-states)
- [Listen backlog: accept queue и SYN queue](#listen-backlog-accept-queue-и-syn-queue)
- [TIME_WAIT: почему их тысячи и это нормально](#time_wait-почему-их-тысячи-и-это-нормально)
- [CLOSE_WAIT: симптом утечки соединений](#close_wait-симптом-утечки-соединений)
- [SO_REUSEADDR и SO_REUSEPORT](#so_reuseaddr-и-so_reuseport)
- [Socket buffers: send и receive](#socket-buffers-send-и-receive)
- [Nagle's algorithm и TCP_NODELAY](#nagles-algorithm-и-tcp_nodelay)
- [TCP Keep-alive](#tcp-keep-alive)
- [Sysctl: тюнинг сетевого стека](#sysctl-тюнинг-сетевого-стека)
- [Диагностика: ss и /proc/net/tcp](#диагностика-ss-и-procnettcp)
- [Go: сетевые параметры](#go-сетевые-параметры)
- [Interview-ready answer](#interview-ready-answer)

---

## Socket lifecycle: syscalls

Сервер:
```c
// Создать сокет (получаем fd)
int sockfd = socket(AF_INET, SOCK_STREAM, 0);

// Привязать к адресу:порту
bind(sockfd, &addr, sizeof(addr));

// Начать принимать соединения (backlog = очередь)
listen(sockfd, 128);

// Достать соединение из очереди (блокирует, если очередь пуста)
int connfd = accept(sockfd, &client_addr, &addrlen);

// Читать/писать
read(connfd, buf, size);
write(connfd, buf, size);

// Закрыть
close(connfd);
```

Клиент:
```c
int sockfd = socket(AF_INET, SOCK_STREAM, 0);

// Инициировать TCP handshake
connect(sockfd, &server_addr, sizeof(server_addr));

// После успешного connect — соединение установлено
write(sockfd, request, size);
read(sockfd, response, size);
close(sockfd);
```

---

## TCP connection states

```text
                    [SYN_SENT] ←── client вызвал connect()
                         │
                  отправлен SYN
                         │
                    [SYN_RCVD] ←── server получил SYN, отправил SYN-ACK
                         │
                  получен ACK
                         ▼
                   [ESTABLISHED] ←── данные можно передавать
                         │
              ┌──────────┴──────────┐
              │ активное закрытие    │ пассивное закрытие
              ▼                     ▼
        [FIN_WAIT_1]          [CLOSE_WAIT] ←── FIN получен, но app ещё не close()
              │                     │
        [FIN_WAIT_2]           [LAST_ACK]
              │                     │
         [TIME_WAIT]          [CLOSED]
              │
  (2*MSL ≈ 60s)
              │
          [CLOSED]
```

**Активное закрытие** — сторона, которая вызвала `close()` первой, проходит FIN_WAIT → TIME_WAIT.

---

## Listen backlog: accept queue и SYN queue

При вызове `listen(sockfd, backlog)` ядро создаёт две очереди:

```text
Клиент отправляет SYN
         │
         ▼
┌─────────────────────┐
│    SYN queue        │  ← полностью открытые SYN_RCVD соединения
│   (backlog limit)   │     ограничена tcp_max_syn_backlog
└────────┬────────────┘
         │ получен ACK от клиента → соединение полностью установлено
         ▼
┌─────────────────────┐
│    Accept queue     │  ← ESTABLISHED соединения, ждущие accept()
│   (backlog limit)   │     ограничена min(backlog, net.core.somaxconn)
└────────┬────────────┘
         │ вызван accept()
         ▼
  приложение обрабатывает
```

**Переполнение accept queue**: при высокой нагрузке и медленном `accept()` — очередь заполняется. Новые соединения **молча отбрасываются** (no response) или сбрасываются с RST. Клиент видит connect timeout или connection refused.

```bash
# Размер очереди для сокета
ss -lnt | grep :8080
# Recv-Q = текущий размер accept queue
# Send-Q = backlog (максимальный размер)

# Переполнение accept queue (нарастающий счётчик)
cat /proc/net/netstat | grep ListenOverflows
netstat -s | grep "listen queue"
```

```bash
# /proc/sys/net/core/somaxconn — системный лимит backlog
cat /proc/sys/net/core/somaxconn  # обычно 128!

# Для высоконагруженных серверов:
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535
```

В Go:
```go
ln, _ := net.Listen("tcp", ":8080")
// Backlog берётся из /proc/sys/net/core/somaxconn
// Чтобы явно задать — нужен net.ListenConfig + syscall.SetsockoptInt
```

---

## TIME_WAIT: почему их тысячи и это нормально

TIME_WAIT — состояние после активного закрытия соединения. Длится **2*MSL** (Maximum Segment Lifetime), обычно **60 секунд** на Linux.

**Зачем TIME_WAIT**:
1. **Надёжная доставка последнего ACK**: если FIN+ACK от сервера потерялся, он перепосылает FIN. Клиент в TIME_WAIT ещё может ответить ACK. Без TIME_WAIT — клиент уже не знает об этом соединении.
2. **Исключение "блуждающих пакетов"**: старые TCP сегменты с тем же 4-tuple (src_ip:src_port:dst_ip:dst_port) могут приходить с задержкой. 2*MSL гарантирует, что все старые сегменты истекли до повторного использования 4-tuple.

**Когда тысячи TIME_WAIT — нормально**: HTTP/1.1 без keep-alive, или сервер, закрывающий соединения после ответа (активное закрытие). При 1000 req/s и 60s TTL — в TIME_WAIT будет ~60000 соединений. Это не проблема: они не занимают горутины и почти не потребляют памяти (~0.3 KB каждое).

**Когда TIME_WAIT — проблема**:
- Закончились ephemeral ports (`ip_local_port_range`): обычно 32768–60999 = 28232 портов. При скорости 1000 RPS к одному backend — port exhaustion через ~28 секунд.
- Решение: `SO_REUSEADDR` (переиспользовать порт в TIME_WAIT), увеличить `ip_local_port_range`.

```bash
# Сколько TIME_WAIT
ss -ant | grep TIME-WAIT | wc -l

# Диапазон ephemeral портов
cat /proc/sys/net/ipv4/ip_local_port_range  # 32768 60999

# Расширить диапазон
sysctl -w net.ipv4.ip_local_port_range="1024 65535"

# Разрешить переиспользование TIME_WAIT сокетов для новых соединений
sysctl -w net.ipv4.tcp_tw_reuse=1
# (только для исходящих соединений с правильным timestamp)
```

---

## CLOSE_WAIT: симптом утечки соединений

**CLOSE_WAIT** — состояние после получения FIN от удалённой стороны, но до вызова `close()` локальным приложением.

```text
Удалённая сторона закрыла соединение:
  Remote → FIN → Local
  Local → ACK → Remote
  Состояние Local: CLOSE_WAIT
  (ждём пока приложение вызовет close())
```

**Тысячи CLOSE_WAIT** — это **всегда баг в приложении**: приложение получило FIN (например, сервер закрыл соединение), но не закрывает `conn.Close()`.

Типичные причины в Go:
```go
// Не закрыт response body
resp, err := client.Get(url)
if err != nil { return }
// забыли defer resp.Body.Close() ← CLOSE_WAIT нарастает

// Не закрыт net.Conn
conn, _ := net.Dial("tcp", addr)
// забыли conn.Close() ← CLOSE_WAIT
```

```bash
# Диагностика CLOSE_WAIT
ss -ant | grep CLOSE-WAIT | wc -l
# Если растёт со временем и не уменьшается — утечка

# Кому принадлежат CLOSE_WAIT сокеты
ss -antp | grep CLOSE-WAIT
# Покажет PID и имя процесса
```

---

## SO_REUSEADDR и SO_REUSEPORT

### SO_REUSEADDR

Позволяет привязаться к порту, если старый сокет в TIME_WAIT. Стандартно используется всеми серверами:

```go
// Go автоматически устанавливает SO_REUSEADDR при net.Listen
ln, _ := net.Listen("tcp", ":8080")
```

Без SO_REUSEADDR: перезапуск сервера с коротким downtime даст `bind: address already in use` если старый сокет ещё в TIME_WAIT.

### SO_REUSEPORT (Linux 3.9+)

Несколько независимых сокетов могут быть привязаны к одному порту. Ядро **балансирует** входящие соединения между ними через hash (src_ip:src_port).

```text
Без SO_REUSEPORT:
  Server socket (fd=3) :8080
      └─ все accept() в одной точке (bottleneck)

С SO_REUSEPORT:
  Goroutine/Process 1: socket :8080 (fd=3)
  Goroutine/Process 2: socket :8080 (fd=4)
  Goroutine/Process 3: socket :8080 (fd=5)
      ← ядро распределяет соединения между ними
```

Применение:
- Nginx с несколькими воркер-процессами (каждый биндится на один порт).
- Kubernetes: несколько Pod'ов, принимающих на одном NodePort (через kube-proxy).
- Go: при использовании `SO_REUSEPORT` несколько горутин могут параллельно делать `accept()`.

```go
// Явная установка SO_REUSEPORT в Go
lc := net.ListenConfig{
    Control: func(network, address string, c syscall.RawConn) error {
        return c.Control(func(fd uintptr) {
            syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET,
                unix.SO_REUSEPORT, 1)
        })
    },
}
ln, _ := lc.Listen(ctx, "tcp", ":8080")
```

---

## Socket buffers: send и receive

Каждый TCP-сокет имеет два буфера в ядре:
- **Receive buffer (SO_RCVBUF)**: входящие данные от сети, ещё не прочитанные приложением.
- **Send buffer (SO_SNDBUF)**: данные, отправленные приложением, но ещё не подтверждённые (ACK).

```text
Sender app → write() → send buffer → TCP stack → network → TCP stack → recv buffer → read() → Receiver app
                             │                                     │
                        ждёт ACK                            window advertisement
```

**TCP receive window**: получатель сообщает отправителю, сколько данных готов принять (`receive buffer size - unread data`). Если буфер заполнен → receive window = 0 → отправитель останавливается (zero window probe).

```bash
# Показать размеры буферов сокета
ss -tnm | head -5
# Recv-Q: данные в receive buffer, ещё не прочитанные приложением
# Send-Q: данные в send buffer, не подтверждённые

# Системные дефолты
cat /proc/sys/net/core/rmem_default    # receive buffer default
cat /proc/sys/net/core/rmem_max        # receive buffer max
cat /proc/sys/net/core/wmem_default    # send buffer default
cat /proc/sys/net/core/wmem_max        # send buffer max

# TCP auto-tuning (динамически подбирает размер буфера)
cat /proc/sys/net/ipv4/tcp_rmem
# min default max
# 4096 131072 6291456

# Увеличить для высокопроизводительных сетей
sysctl -w net.core.rmem_max=16777216
sysctl -w net.core.wmem_max=16777216
sysctl -w net.ipv4.tcp_rmem="4096 87380 16777216"
```

---

## Nagle's algorithm и TCP_NODELAY

**Nagle's algorithm** (RFC 896, 1984): не отправляй маленькие пакеты, если есть неподтверждённые данные. Аккумулируй данные пока:
- отправленные данные не подтверждены (ACK) **и**
- размер данных < MSS (Maximum Segment Size, обычно ~1460 bytes).

```text
Без Nagle (TCP_NODELAY):        С Nagle (default):
write("H")   → [H]             write("H")
write("i")   → [i]             write("i")   → ждём
write("!")   → [!]             write("!")   → ждём
                                ← ACK пришёл → отправляем [Hi!] вместе
```

**Когда Nagle вреден**:
- Interactive протоколы: SSH, Redis, SQL клиент. Наглая буферизация = задержка команды.
- HTTP/1.1 pipelining: маленький финальный chunk задерживается.
- gRPC с маленькими сообщениями.

**Когда Nagle полезен**:
- Bulk transfer: меньше пакетов → меньше overhead.
- Редкие write() с большими данными — алгоритм не влияет.

```go
// Отключить Nagle в Go (рекомендуется для API серверов)
conn.(*net.TCPConn).SetNoDelay(true)

// Go http.Server отключает Nagle автоматически для HTTP соединений
// (net/http устанавливает TCP_NODELAY при accept)
```

**TCP_CORK** (обратная Nagle): принудительно буферизовать данные и отправить одним пакетом. Используется в `sendfile()` для HTTP-ответов (headers + body вместе).

---

## TCP Keep-alive

Механизм для обнаружения dead connections: если соединение idle, ядро периодически отправляет probe-пакеты.

```text
Соединение idle tcp_keepalive_time секунд (default: 7200s = 2 часа!)
    → отправить keep-alive probe
    → если нет ответа: повторить tcp_keepalive_intvl секунд
    → после tcp_keepalive_probes попыток без ответа → соединение = dead
    → приложение получит ECONNRESET при следующем read/write
```

**Проблема дефолтов**: 2 часа — слишком долго для обнаружения dead TCP-соединений. При NAT timeout (обычно 5–30 минут) соединение умирает на уровне NAT, но приложение не знает.

```go
// Более агрессивные keep-alive в Go
conn, _ := net.DialTCP("tcp", nil, addr)
conn.SetKeepAlive(true)
conn.SetKeepAlivePeriod(30 * time.Second)
// Первый probe через 30s, потом системные tcp_keepalive_intvl / tcp_keepalive_probes

// http.Transport настройка
transport := &http.Transport{
    DialContext: (&net.Dialer{
        Timeout:   30 * time.Second,
        KeepAlive: 30 * time.Second,  // keep-alive interval
    }).DialContext,
    IdleConnTimeout:       90 * time.Second,
    TLSHandshakeTimeout:   10 * time.Second,
}
```

Системные параметры:
```bash
sysctl net.ipv4.tcp_keepalive_time     # 7200 → понизить до 60-300
sysctl net.ipv4.tcp_keepalive_intvl    # 75   → интервал между probe
sysctl net.ipv4.tcp_keepalive_probes   # 9    → число попыток
```

---

## Sysctl: тюнинг сетевого стека

```bash
# === Backlog ===
net.core.somaxconn = 65535           # max accept queue size
net.ipv4.tcp_max_syn_backlog = 65535 # max SYN queue size

# === Ephemeral ports ===
net.ipv4.ip_local_port_range = 1024 65535

# === TIME_WAIT ===
net.ipv4.tcp_tw_reuse = 1        # переиспользовать TIME_WAIT для исходящих
net.ipv4.tcp_fin_timeout = 15    # FIN_WAIT_2 timeout (default 60s)

# === Буферы ===
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 16384 16777216

# === Keep-alive ===
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_keepalive_intvl = 30
net.ipv4.tcp_keepalive_probes = 5

# Применить без перезагрузки
sysctl -w net.core.somaxconn=65535
# Постоянно: /etc/sysctl.d/99-network.conf
```

В Kubernetes эти параметры наследуются от ноды (не от контейнера). Для Pod-level tuning нужен privileged init container или sysctl в Pod spec (только белый список).

---

## Диагностика: ss и /proc/net/tcp

```bash
# Состояния соединений сервера
ss -ant
# State    Recv-Q  Send-Q  Local Address:Port  Peer Address:Port

# Только listening
ss -lnt

# С именами процессов
ss -antp

# Количество по состояниям
ss -ant | awk '{print $1}' | sort | uniq -c | sort -rn

# Быстрая сводка
netstat -s | grep -E "failed|overflow|reset|retransmit"

# Детальная статистика TCP
cat /proc/net/netstat | awk '(f==0) {name=$0; f=1} (f==1) {for(i=1;i<=NF;i++) print name" "i" "$i; f=0}' | grep -E "ListenOverflow|TCPTimeWait|PassiveOpens"

# Retransmits (показывает проблемы с packet loss)
ss -tni | grep -E "retrans|cwnd|rtt"
```

**Что смотреть при "сервер не отвечает на новые соединения"**:
1. `ss -lnt` — Recv-Q на listening сокете. Если равен Send-Q → accept queue полна.
2. `cat /proc/net/netstat | grep ListenOverflow` — переполнение.
3. `ss -ant | grep ESTABLISHED | wc -l` — количество установленных соединений.
4. `ulimit -n` в процессе — не исчерпаны ли fd.

---

## Go: сетевые параметры

```go
// http.Server: настройки с учётом TCP-уровня
srv := &http.Server{
    Addr: ":8080",

    // ReadTimeout включает время на установку соединения + чтение запроса
    ReadTimeout:  10 * time.Second,
    WriteTimeout: 30 * time.Second,

    // Idle keep-alive соединений (0 = Go default)
    IdleTimeout: 120 * time.Second,
}

// http.Transport для outgoing calls
transport := &http.Transport{
    // Размер пула idle соединений
    MaxIdleConns:        200,
    MaxIdleConnsPerHost: 50,

    // Закрыть idle соединение через
    IdleConnTimeout: 90 * time.Second,

    // Отключить Nagle (Go устанавливает это автоматически для http)
    DialContext: (&net.Dialer{
        Timeout:   30 * time.Second,
        KeepAlive: 30 * time.Second,
    }).DialContext,
}

// Посмотреть состояние пула
transport.CloseIdleConnections()  // принудительно закрыть idle
```

Типичные сетевые ошибки и причины:

| Ошибка | Причина |
|---|---|
| `connection refused` | Сервер не слушает порт, или backlog переполнен с RST |
| `i/o timeout` | Нет ответа в течение deadline |
| `connection reset by peer` | Удалённая сторона послала RST (упала, crashed, wrong state) |
| `broken pipe` | Запись в закрытое соединение |
| `too many open files` | Исчерпан fd limit (ulimit -n) |
| `dial tcp: lookup: no such host` | DNS не резолвится |
| `use of closed network connection` | Попытка использовать закрытый conn (race condition) |

---

## Interview-ready answer

TCP backlog — две очереди: SYN queue (SYN_RCVD, ждут ACK) и accept queue (ESTABLISHED, ждут accept()). Переполнение accept queue → silently drop или RST → клиент видит timeout. `net.core.somaxconn` ограничивает максимум. TIME_WAIT нужен чтобы гарантировать доставку последнего ACK и истечение "блуждающих пакетов" в сети; длится 2*MSL ≈ 60s; тысячи TIME_WAIT — нормально для active-close сервера. CLOSE_WAIT — это баг: приложение получило FIN но не вызвало close(); расти не должен. SO_REUSEPORT: несколько сокетов биндятся на один порт, ядро балансирует между ними — использует nginx для multi-worker. Nagle's algorithm буферизует маленькие пакеты; TCP_NODELAY отключает — нужен для API серверов и Redis клиентов. Socket buffers (SO_RCVBUF/SO_SNDBUF) определяют throughput на высоколатентных каналах. В Go `net.Listen` автоматически ставит SO_REUSEADDR; HTTP сервер отключает Nagle; `IdleConnTimeout` в Transport управляет временем жизни keep-alive соединений.
