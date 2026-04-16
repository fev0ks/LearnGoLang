# Connection Pooling And Production Issues

Connection pool это ограниченный набор соединений приложения к базе.

## Содержание

- [Зачем нужен pool](#зачем-нужен-pool)
- [Что может пойти не так](#что-может-пойти-не-так)
- [Симптомы pool exhaustion](#симптомы-pool-exhaustion)
- [Что смотреть](#что-смотреть)
- [Долгие транзакции](#долгие-транзакции)
- [Как расследовать DB latency](#как-расследовать-db-latency)
- [Go-specific reminders](#go-specific-reminders)
- [Interview-ready summary](#interview-ready-summary)

## Зачем нужен pool

Открывать новое соединение на каждый запрос дорого.

Pool позволяет:
- переиспользовать connections;
- ограничить давление на БД;
- контролировать concurrency.

## Что может пойти не так

Pool слишком маленький:
- запросы ждут свободный connection;
- растет latency;
- приложение кажется "медленным", хотя БД может быть не перегружена.

Pool слишком большой:
- приложение давит на БД слишком большим parallelism;
- растет contention;
- БД тратит ресурсы на connections;
- tail latency может стать хуже.

## Симптомы pool exhaustion

- растет latency без роста CPU приложения;
- много goroutines ждут DB connection;
- timeout на запросах;
- p95 и p99 растут сильнее average;
- БД видит много active or idle connections.

## Что смотреть

В приложении:
- pool wait duration;
- open connections;
- in-use connections;
- idle connections;
- query duration;
- timeout count.

В БД:
- active connections;
- long-running transactions;
- locks;
- slow queries;
- deadlocks;
- replication lag.

## Долгие транзакции

Долгая транзакция вредна:
- держит locks;
- держит connection;
- мешает vacuum-like maintenance;
- увеличивает шанс deadlock.

Примеры плохого кода:
- открыть tx и потом делать HTTP call;
- читать большой stream внутри tx без необходимости;
- забыть commit или rollback.

## Как расследовать DB latency

Нормальный порядок:

1. Проверить, где wait: app pool или сама query
2. Посмотреть slow queries
3. Посмотреть locks и long transactions
4. Проверить query plan
5. Проверить индексы и cardinality
6. Проверить saturation самой БД

## Go-specific reminders

- всегда передавай `context.Context`;
- задавай timeouts;
- проверяй `rows.Close()`;
- проверяй `rows.Err()`;
- не открывай transaction без необходимости;
- не делай unbounded fan-out в БД.

## Interview-ready summary

Connection pool это не просто performance optimization, а backpressure boundary между приложением и БД. Если pool настроен плохо, сервис может деградировать даже при нормальном коде и нормальной базе.
