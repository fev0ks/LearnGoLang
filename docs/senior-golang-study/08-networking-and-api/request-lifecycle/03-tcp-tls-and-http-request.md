# TCP, TLS и HTTP-запрос

Когда browser получил IP, он ещё не "отправил HTTP". Сначала — транспортное соединение, TLS, и только потом application data.

## Содержание

- [TCP three-way handshake](#tcp-three-way-handshake)
- [TLS 1.2 vs TLS 1.3: стоимость в RTT](#tls-12-vs-tls-13-стоимость-в-rtt)
- [TLS 1.3 0-RTT resumption](#tls-13-0-rtt-resumption)
- [Что проверяет браузер в сертификате](#что-проверяет-браузер-в-сертификате)
- [HTTP/1.1 vs HTTP/2 vs HTTP/3](#http11-vs-http2-vs-http3)
- [Connection reuse и keep-alive](#connection-reuse-и-keep-alive)
- [Где здесь бывают проблемы](#где-здесь-бывают-проблемы)
- [Interview-ready answer](#interview-ready-answer)

## TCP three-way handshake

```text
Client          Server
  │── SYN ──────►│
  │◄── SYN-ACK ──│   ← 1 RTT
  │── ACK ───────►│
  │              │  ← можно начинать TLS
```

Цена: минимум **1 RTT** до того, как TCP connection установлен. При latency 50 ms до сервера — только на TCP уходит 50 ms.

TCP — stream-oriented протокол с гарантией порядка и доставки. Потеря пакета вызывает retransmit и блокирует все данные после него (Head-of-Line blocking на уровне TCP).

## TLS 1.2 vs TLS 1.3: стоимость в RTT

### TLS 1.2 — 2 RTT после TCP

```text
Client                          Server
  │── ClientHello ─────────────►│  cipher suites, random
  │◄── ServerHello + Cert ──────│  [RTT 1] server выбрал cipher, отдал cert
  │    + ServerHelloDone        │
  │── ClientKeyExchange ────────►│  pre-master secret (RSA) или DH key share
  │   + ChangeCipherSpec        │
  │   + Finished                │
  │◄── ChangeCipherSpec ─────────│  [RTT 2]
  │    + Finished               │
  │              ───────────────── application data
```

Итого: **TCP (1 RTT) + TLS 1.2 (2 RTT) = 3 RTT** до первого байта ответа.

### TLS 1.3 — 1 RTT после TCP

TLS 1.3 объединил key exchange и handshake в один round trip: client сразу посылает key share в ClientHello.

```text
Client                          Server
  │── ClientHello + key_share ─►│  сразу отправляет DH-часть
  │◄── ServerHello + key_share  │  [RTT 1]
  │    + Certificate + Finished │  server уже может дешифровать
  │── Finished ────────────────►│
  │              ───────────────── application data
```

Итого: **TCP (1 RTT) + TLS 1.3 (1 RTT) = 2 RTT** до первого байта.

Также убраны устаревшие cipher suites (RC4, MD5, SHA-1) и RSA key exchange (нет forward secrecy).

### Суммарная стоимость соединения

| Сценарий | RTT до первого байта |
|---|---|
| TLS 1.2, новое соединение | 3 RTT |
| TLS 1.3, новое соединение | 2 RTT |
| TLS 1.3, session resumption | 1 RTT |
| HTTP/3 (QUIC), новое | 1 RTT |
| HTTP/3 (QUIC), resumption | 0 RTT |

## TLS 1.3 0-RTT resumption

При повторном подключении клиент может отправить **early data** (application data) вместе с ClientHello — до завершения handshake.

```text
Client                          Server
  │── ClientHello + early_data ►│  0-RTT: данные уже летят
  │◄── ServerHello + Finished ──│  [RTT 1] подтверждение
```

**Ограничение**: 0-RTT уязвим к **replay attack** — злоумышленник может переслать перехваченный 0-RTT пакет. Сервер должен принять его снова, если не имеет механизма deduplication (one-time token, replay cache).

Правило: 0-RTT допустим только для **идемпотентных** запросов (GET). Никогда для POST/PUT/DELETE, изменяющих состояние.

## Что проверяет браузер в сертификате

1. **Signature chain**: сертификат подписан промежуточным CA, тот — root CA из trust store браузера.
2. **Hostname match**: `CN` или `SAN` (Subject Alternative Name) совпадает с хостом. Wildcard `*.example.com` покрывает один уровень.
3. **Validity period**: `notBefore` ≤ now ≤ `notAfter`. Истёкший сертификат → hard error.
4. **Revocation**: CRL или OCSP. Браузеры часто используют OCSP stapling — сервер сам прикладывает свежий OCSP response к handshake.
5. **SNI**: client посылает hostname в TLS ClientHello, чтобы сервер знал, какой сертификат отдать (один IP, много доменов).
6. **ALPN**: client перечисляет протоколы (`h2`, `http/1.1`), сервер выбирает. Именно так браузер договаривается об HTTP/2.

## HTTP/1.1 vs HTTP/2 vs HTTP/3

### HTTP/1.1

- Одно TCP-соединение = один запрос за раз (HOL blocking на application уровне).
- Браузер открывает **6 соединений на host** как workaround.
- `Connection: keep-alive` — reuse соединения, не нужен повторный handshake.
- Pipelining формально существует, но сломан из-за proxy-серверов, почти не используется.

### HTTP/2

- **Multiplexing**: несколько независимых streams поверх одного TCP-соединения. Браузер может слать 100 запросов параллельно.
- **HPACK**: header compression. Заголовки индексируются — повторные запросы на 40–60% меньше.
- **Server push**: сервер сам отправляет ресурсы без явного запроса (deprecated в большинстве браузеров из-за race с cache).
- **Проблема**: TCP HOL blocking. Если один TCP-пакет потерян — все streams ждут retransmit. При 1% packet loss HTTP/2 может быть медленнее HTTP/1.1 (из-за 6 параллельных соединений у 1.1).

### HTTP/3 / QUIC

- Работает поверх **UDP**. QUIC реализует свой stream-multiplexing и reliable delivery.
- Потеря пакета блокирует только тот stream, которому он принадлежит. Остальные продолжают работу.
- **0-RTT соединение** для повторных подключений (QUIC TLS 1.3 0-RTT).
- **Лучше на нестабильных сетях** (mobile, Wi-Fi) — быстрее восстанавливается после смены IP (connection migration по connection ID, а не IP:port).
- Проблема: часть NAT/firewall блокируют UDP 443. Браузеры fallback на TCP+TLS.

```text
HTTP/1.1:  [Req1][─────────Resp1─────────][Req2][───Resp2───]
           ↑ один запрос за раз, HOL blocking

HTTP/2:    [Req1][Req2][Req3]────►  (multiplexed streams)
           [─Resp1─][─Resp2─][─Resp3─]  но при packet loss все ждут

HTTP/3:    [Req1][Req2][Req3]────►  (QUIC streams over UDP)
           [─Resp2─][─Resp3─]  Resp1 ждёт retransmit, остальные идут
```

## Connection reuse и keep-alive

Первый запрос к хосту платит TCP + TLS. Последующие — бесплатно, если соединение живо.

```text
Первый запрос:  DNS + TCP(1RTT) + TLS1.3(1RTT) + HTTP = 2+ RTT overhead
Повторный:      HTTP only = минимальная latency
```

**HTTP/2 multiplexing** делает reuse ещё ценнее: все параллельные запросы к хосту идут по одному соединению.

На backend стороне Go `net/http` поддерживает keep-alive по умолчанию. Для outgoing клиентов важно reuse `http.Client` (не создавать новый на каждый запрос) — иначе каждый запрос платит handshake.

```go
// правильно: один клиент на весь процесс
var client = &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     90 * time.Second,
    },
    Timeout: 10 * time.Second,
}

// неправильно: новый клиент — новое соединение каждый раз
func badCall() {
    c := &http.Client{}  // теряет connection pool
    c.Get("https://api.example.com/data")
}
```

## Где здесь бывают проблемы

- **packet loss на handshake**: TCP SYN или TLS ClientHello потерян → retransmit timeout (1s, потом 2s, 4s...).
- **certificate expired**: сразу hard error, до handler не дойдёт. Alert через monitoring cert expiry.
- **SNI mismatch**: клиент указал один host, сервер вернул сертификат для другого → certificate error.
- **TLS version mismatch**: клиент требует TLS 1.3, сервер умеет только 1.2 → не согласовали → failure.
- **proxy/firewall DPI**: deep packet inspection может блокировать необычные TLS extensions (quic, ECH).
- **QUIC blocked by UDP firewall**: HTTP/3 не работает, нужен fallback. Cloudflare/Google делают alt-svc + fallback автоматически.

## Interview-ready answer

TCP требует 1 RTT handshake. TLS 1.2 добавляет 2 RTT, итого 3 RTT до данных. TLS 1.3 сократил до 1 RTT (key share в ClientHello), итого 2 RTT. 0-RTT session resumption позволяет послать данные без RTT overhead, но уязвим к replay — только для идемпотентных запросов. HTTP/2 решает HOL blocking на application уровне через multiplexing, но TCP HOL blocking остаётся при потере пакетов. HTTP/3/QUIC работает поверх UDP со своим stream-multiplexing, потеря пакета блокирует только один stream. SNI нужен, чтобы сервер знал какой сертификат отдать при shared IP; ALPN — чтобы согласовать h2 vs http/1.1 в TLS handshake.
