# System Design

Здесь собирай design-разборы и шаблоны ответа на system design интервью.

Основные блоки:
- сбор требований и оценка нагрузки;
- SLA, SLO, latency budget;
- stateless vs stateful components;
- кэширование, rate limiting, backpressure;
- consistency, replication, partitioning;
- HA, failover, disaster recovery;
- idempotency и duplicate handling;
- observability как часть дизайна, а не постфактум.

Подпакеты:
- [External Request Flows](./external-request-flows/README.md)

Хорошие тренировочные кейсы:
- URL shortener;
- notification service;
- rate limiter;
- task processing platform;
- chat or realtime events service;
- metrics ingestion pipeline.

## Подборка

- [Google SRE Resources](https://sre.google/resources/)
- [The Site Reliability Workbook](https://sre.google/workbook/preface/)
- [AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/framework/welcome.html)
- [Azure Cloud Design Patterns](https://learn.microsoft.com/en-us/azure/architecture/patterns/)
- [Kubernetes Concepts](https://kubernetes.io/docs/concepts/index.html)

## Вопросы

- какие требования ты уточнишь первыми до того, как рисовать архитектуру;
- где в системе будут single points of failure;
- как изменится дизайн при росте в 10 раз по write traffic;
- где уместен cache, а где он ломает consistency;
- как ты будешь защищать систему от retry storm и thundering herd;
- какие метрики и алерты нужны уже в первой версии;
- как ты объяснишь, почему выбрал именно такой storage и messaging layer.
