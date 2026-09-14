# Нагрузочное тестирование backend-систем

Нагрузочный тест отвечает на вопрос, какую работу система выполняет с приемлемым
качеством, где заканчивается её запас и как она восстанавливается после перегрузки.
Для этого нужны реалистичный поток операций, измерения на всех слоях и понятные
критерии успеха. Один график RPS этого не показывает.

## Материалы

1. [Задачи, виды тестов и выбор инструментов](./01-strategy-and-test-types.md) — load, stress, spike, soak, выбор окружения и место тестов в CI.
2. [Профиль нагрузки и модели генерации](./02-workload-models.md) — RPS, виртуальные пользователи, open/closed model, данные, кэши и повторы.
3. [Практический тест API с k6](./03-k6-practical-guide.md) — сценарий чтения, плавный рост нагрузки, проверки ответа, thresholds и ограничения генератора.
4. [Метрики, поиск узкого места и оценка ёмкости](./04-results-and-bottlenecks.md) — p95/p99, очереди, Go runtime, проверка гипотез и отчёт.
5. [Нагрузочные эксперименты в production](./05-production-testing.md) — synthetic, replay, shadow traffic, изоляция, план запуска, аварийная остановка и восстановление.
6. [Инструменты и генерация тысяч RPS](./06-tools-and-load-generation.md) — k6, Vegeta, wrk2, Locust, JMeter, Gatling и ghz; одна машина, распределённый запуск и управляемые сервисы.

---

## Как читать

Для первого знакомства читать по порядку. Если вопрос именно про живой production,
начать с пятой статьи, затем вернуться к моделям нагрузки и интерпретации результатов.
Для практики взять третью статью, выполнить небольшой smoke test на своём стенде и
разобрать результат по четвёртой.
Для выбора инструмента и генерации тысяч или десятков тысяч RPS читать шестую
статью: в ней сравниваются сценарии применения и способы масштабирования генератора.

Числа в примерах — учебные допущения, а не измеренная производительность этого
репозитория. Порог latency, допустимую долю ошибок и размер ступеней нагрузки
определяют требования конкретного сервиса.

---

## Связанные материалы

- [Race detector, fuzzing и benchmarks](../11-race-fuzz-and-benchmarks.md) — проверка кода и стоимости отдельных операций.
- [Профилирование Go](../../01-go-core/profiling/README.md) — инструменты для проверки гипотез о CPU, памяти и ожиданиях.
- [Практические метрики](../../10-devops-and-observability/prometheus-and-metrics/practical-metric-patterns/README.md) — метрики API, зависимостей и Go runtime.
- [Реагирование на инциденты](../../10-devops-and-observability/incident-response-and-investigation/README.md) — действия при деградации живого сервиса.
- [SLO, SLI и error budget](../../05-system-design/reliability-patterns/08-slo-sli-error-budgets.md) — цели качества и допустимый бюджет ошибок.
- [Chaos engineering](../../05-system-design/reliability-patterns/10-chaos-engineering.md) — эксперименты с отказами компонентов.

---

## Официальные источники

- [Grafana k6: виды нагрузочных тестов](https://grafana.com/docs/k6/latest/testing-guides/test-types/).
- [Grafana k6: открытая и закрытая модели](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/).
- [Grafana k6: thresholds](https://grafana.com/docs/k6/latest/using-k6/thresholds/).
- [Locust: возможности и модель сценариев](https://docs.locust.io/en/stable/what-is-locust.html).
- [Vegeta: HTTP load testing](https://github.com/tsenart/vegeta).
- [Istio: traffic mirroring](https://istio.io/latest/docs/tasks/traffic-management/mirroring/).
- [Google SRE: handling overload](https://sre.google/sre-book/handling-overload/).

Семантика используемых API сверена с официальной документацией 13 сентября 2026 года.
Для воспроизводимого запуска сохранять точную версию инструмента из своего окружения.
