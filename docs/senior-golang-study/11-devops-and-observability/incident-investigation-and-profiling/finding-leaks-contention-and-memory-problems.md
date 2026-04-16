# Finding Leaks, Contention And Memory Problems

Под "утечкой" в Go часто имеют в виду не только literal memory leak, но и любой ресурс, который растет бесконтрольно.

## Что реально течет в Go чаще всего

- goroutines;
- timers and tickers;
- contexts and child operations;
- memory из-за удержания ссылок;
- queues and buffers;
- connections.

## Как искать goroutine leak

Сигналы:
- goroutine count растет и не возвращается;
- service idle, а горутин все больше;
- heap растет вместе с числом горутин.

Что смотреть:
- goroutine profile;
- stack traces зависших горутин;
- места без cancellation;
- receive/send на каналах без завершения;
- forgotten worker loops.

## Как искать memory problem

Сигналы:
- heap постоянно растет;
- GC становится чаще;
- tail latency растет вместе с allocation pressure;
- процесс доходит до OOM.

Что смотреть:
- heap profile;
- alloc profile;
- large retained objects;
- map и slice growth;
- caching without bounds.

## Как искать lock contention

Сигналы:
- CPU не обязательно высокий, но latency растет;
- throughput падает под нагрузкой;
- goroutines много ждут.

Что смотреть:
- mutex profile;
- block profile;
- shared hot lock;
- oversized critical sections.

## Как искать perf regression

Нормальный путь:
- сравнить baseline и current profile;
- посмотреть p95 and p99;
- отделить CPU issue от wait issue;
- проверить rollout or config change;
- проверить рост payload, cardinality или fan-out.

## Interview-ready summary

Если кратко:
- traces показывают, где медленно по пути запроса;
- profiles показывают, что медленно внутри процесса;
- leaks это часто не "дырка в памяти", а бесконтрольный рост горутин, буферов и ссылок;
- для lock contention нужны mutex and block profiles, а не только CPU graph.
