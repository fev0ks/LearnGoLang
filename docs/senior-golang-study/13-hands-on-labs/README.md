# Hands-On Labs

Сюда складывай практику, а не теорию.

Идеи лабораторных:
- реализовать worker pool с graceful shutdown;
- написать rate limiter несколькими способами;
- сделать mini event-driven pipeline;
- сравнить `pgx` и `database/sql` на одном кейсе;
- профилировать intentionally slow endpoint;
- реализовать idempotent consumer;
- собрать небольшой service template с observability и health checks.

Для каждой lab фиксируй:
- цель;
- ограничения;
- метрики успеха;
- что именно оказалось bottleneck;
- чему научился.

## Подборка

- [Testcontainers for Go](https://golang.testcontainers.org/)
- [RabbitMQ Tutorials](https://www.rabbitmq.com/tutorials)
- [NATS Docs](https://docs.nats.io/)
- [Apache Kafka Documentation](https://kafka.apache.org/documentation/)
- [Go Diagnostics](https://go.dev/doc/diagnostics)
- [Prometheus Overview](https://prometheus.io/docs/introduction/overview/)

## Вопросы

- какую лабораторную ты можешь сделать за 2-3 часа и получить максимум пользы;
- что именно ты измеряешь в lab, кроме "работает/не работает";
- как превратить lab в разговорный кейс для интервью;
- что в реализации было самым рискованным и как ты это проверил;
- как бы ты упростил lab для junior и усложнил для senior.
