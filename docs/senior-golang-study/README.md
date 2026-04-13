# Senior Golang Study

Эта папка для системной подготовки к вакансиям уровня Senior Go Developer.

Цель структуры:
- разложить подготовку по темам, которые обычно проверяют на senior-собеседованиях;
- собирать не просто ссылки, а свои конспекты, сравнения, trade-offs и готовые ответы;
- держать рядом теорию, практику и типовые design-разборы.

Как использовать:
- в каждом разделе добавляй отдельные заметки по темам, например `scheduler.md`, `postgres-indexes.md`, `kafka-vs-rabbitmq.md`;
- для каждой темы фиксируй: когда это применять, какие есть компромиссы, типовые ошибки, что спросить интервьюеру;
- сложные темы лучше вести в формате `problem -> options -> trade-offs -> decision`.

Рекомендуемый порядок прохождения:
1. `01-go-core`
2. `09-concurrency-and-performance`
3. `16-go-version-differences`
4. `04-architecture-and-patterns`
5. `05-system-design`
6. `06-databases`
7. `07-message-brokers-and-streaming`
8. `10-testing-and-quality`
9. `11-devops-and-observability`
10. `12-security`
11. `13-interview-practice`

Разделы:
- `00-roadmap` - приоритеты, план подготовки, чек-листы
- `01-go-core` - язык, runtime, memory model, idiomatic Go
- `02-go-stdlib-and-tools` - стандартная библиотека и инструменты Go
- `03-go-libraries-and-ecosystem` - популярные библиотеки и сравнения
- `04-architecture-and-patterns` - архитектура backend-сервисов
- `05-system-design` - high-level и low-level design
- `06-databases` - SQL, NoSQL, индексы, транзакции, масштабирование
- `07-message-brokers-and-streaming` - очереди, стриминг, delivery semantics
- `08-networking-and-api` - HTTP, gRPC, API contracts, retries, timeouts
- `09-concurrency-and-performance` - конкурентность, профилирование, оптимизация
- `10-testing-and-quality` - тестирование, линтеры, качество кода
- `11-devops-and-observability` - CI/CD, Docker, Kubernetes, monitoring
- `12-security` - безопасность приложений и инфраструктуры
- `13-interview-practice` - вопросы, ответы, сторителлинг, мок-интервью
- `14-hands-on-labs` - практические мини-проекты и drill-задачи
- `15-notes-and-links` - быстрые заметки, ссылки, статьи, backlog тем
- `16-go-version-differences` - ключевые изменения между версиями Go и влияние на кодовую базу

## Базовая подборка

Эти источники стоит держать как общий минимум по всей подготовке:
- [Go Documentation](https://go.dev/doc)
- [Go Language Specification](https://go.dev/ref/spec)
- [The Go Memory Model](https://go.dev/ref/mem)
- [A Guide to the Go Garbage Collector](https://go.dev/doc/gc-guide)
- [Google SRE Resources](https://sre.google/resources/)
- [AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/framework/welcome.html)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/current/index.htm)
- [Redis Docs](https://redis.io/docs/latest/)
- [Apache Kafka Documentation](https://kafka.apache.org/documentation/)
- [RabbitMQ Documentation](https://www.rabbitmq.com/docs)
- [NATS Docs](https://docs.nats.io/)
- [gRPC Documentation](https://grpc.io/docs/)
- [OpenTelemetry Docs](https://opentelemetry.io/docs/)
- [Prometheus Documentation](https://prometheus.io/docs/introduction/overview/)
- [Kubernetes Concepts](https://kubernetes.io/docs/concepts/index.html)
- [OWASP Cheat Sheet Series](https://cheatsheetseries.owasp.org/)

## Сквозные вопросы

Эти вопросы стоит уметь проходить почти в любом разделе:
- какие trade-offs у этого решения;
- что сломается под ростом нагрузки;
- где здесь bottleneck по latency, throughput и operability;
- как это мониторить и дебажить в production;
- как обеспечить backward compatibility;
- как протестировать не только happy path, но и деградацию;
- как решение поменяется при росте команды или требований.
