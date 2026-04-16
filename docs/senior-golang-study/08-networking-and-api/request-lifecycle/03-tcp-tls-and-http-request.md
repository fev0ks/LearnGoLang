# TCP TLS And HTTP Request

Когда browser получил IP, он еще не "отправил HTTP". Сначала надо установить транспортное и, как правило, TLS-соединение.

## Содержание

- [1. Выбор протокола и порта](#1-выбор-протокола-и-порта)
- [2. Установка TCP соединения](#2-установка-tcp-соединения)
- [3. TLS handshake](#3-tls-handshake)
- [4. Что проверяет браузер](#4-что-проверяет-браузер)
- [5. Формирование HTTP запроса](#5-формирование-http-запроса)
- [6. Connection reuse](#6-connection-reuse)
- [7. Где здесь бывают проблемы](#7-где-здесь-бывают-проблемы)
- [Почему это важно](#почему-это-важно)
- [Что могут спросить на интервью](#что-могут-спросить-на-интервью)

## 1. Выбор протокола и порта

Для `https://google.com` обычно используется:
- TCP как транспорт;
- порт `443`;
- поверх него TLS;
- затем HTTP/2 или HTTP/3, в зависимости от negotiated protocol.

## 2. Установка TCP соединения

Для TCP требуется handshake:

1. client отправляет `SYN`
2. server отвечает `SYN-ACK`
3. client отправляет `ACK`

Только после этого появляется установленное TCP connection state.

Цена:
- минимум один round trip только на transport setup.

## 3. TLS handshake

Потом поднимается TLS:
- browser проверяет сертификат;
- сверяет hostname;
- договаривается о cipher suite и параметрах шифрования;
- получает session keys.

Здесь важны:
- certificate chain;
- trusted CA;
- срок действия сертификата;
- SNI, чтобы server понял, для какого hostname нужен сертификат;
- ALPN, чтобы договориться об HTTP/1.1, HTTP/2 или HTTP/3.

## 4. Что проверяет браузер

Browser должен убедиться:
- сертификат валиден;
- сертификат выписан на нужный host;
- цепочка доверия замыкается на trusted CA;
- сертификат не просрочен.

Если это не выполняется:
- пользователь увидит TLS warning или connection error;
- до application handler запрос вообще не дойдет.

## 5. Формирование HTTP запроса

Только после transport и TLS browser формирует полноценный request:

```http
GET / HTTP/2
Host: google.com
User-Agent: ...
Accept: text/html,...
Accept-Language: ...
Cookie: ...
```

На практике headers могут быть длиннее:
- cookies;
- sec-fetch headers;
- cache validators;
- compression negotiation, например `gzip`, `br`, `zstd`.

## 6. Connection reuse

Если browser уже имеет открытое соединение:
- новый request может пойти по reused connection;
- это экономит latency на TCP и TLS handshake.

Это особенно важно для:
- повторных navigation;
- загрузки множества subresources;
- HTTP/2 multiplexing.

## 7. Где здесь бывают проблемы

- packet loss на handshake;
- TLS certificate error;
- slow TLS negotiation;
- server не поддерживает ожидаемый protocol;
- connection timeout;
- proxy или firewall режет соединение.

## Почему это важно

Когда пользователь говорит "сайт долго открывается", проблема может быть:
- не в SQL;
- не в Go handler;
- а в handshake до того, как backend вообще увидел request.

## Что могут спросить на интервью

- зачем нужен TCP three-way handshake;
- что проверяется в TLS;
- зачем нужны SNI и ALPN;
- почему keep-alive и connection reuse снижают latency;
- чем отличается latency первого запроса от latency запроса по reused connection.
