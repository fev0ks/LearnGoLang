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
- [Experimentation And Feature Rollouts](./experimentation-and-feature-rollouts/README.md)
- [Interview Cases](./interview-cases/README.md) — разборы популярных задач по фазам интервью

## Interview Cases

Полные разборы с уточнением требований, оценкой нагрузки, архитектурой и трейдоффами:

- [00. Как проходить System Design Interview](./interview-cases/00-how-to-approach.md) — фреймворк, тайминг, что оценивает интервьюер
- [01. URL Shortener](./interview-cases/01-url-shortener.md)
- [02. Notification Service](./interview-cases/02-notification-service.md)
- [03. Rate Limiter](./interview-cases/03-rate-limiter.md)
- [04. Chat / Messaging](./interview-cases/04-chat-messaging.md)
- [05. Task Queue](./interview-cases/05-task-queue.md)
- [06. Uber / Ride-Sharing](./interview-cases/06-uber-ride-sharing.md) — H3, matching, geo at scale
- [07. YouTube / Video Platform](./interview-cases/07-youtube-video-platform.md) — transcode pipeline, HLS, CDN
- [08. Twitter / Social Feed](./interview-cases/08-twitter-social-feed.md) — hybrid fan-out, celebrity problem
- [09. Netflix / Streaming](./interview-cases/09-netflix-streaming.md) — Open Connect CDN, per-title encoding
- [10. Google Drive](./interview-cases/10-google-drive.md) — content-addressed chunking, sync, conflict resolution
- [11. Payment System](./interview-cases/11-payment-system.md) — double-entry, idempotency, Saga + Outbox

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
