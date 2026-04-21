# Метрики, анализ и типичные ошибки

Эта заметка про то, какие метрики нужны для экспериментов, как они могут выглядеть в event pipeline и Prometheus, почему одних средних значений мало и какие ошибки чаще всего портят выводы.

## Содержание

- [Три группы метрик](#три-группы-метрик)
- [Primary metric](#primary-metric)
- [Guardrail metrics](#guardrail-metrics)
- [Diagnostic metrics](#diagnostic-metrics)
- [Product events](#product-events)
- [Operational metrics](#operational-metrics)
- [Prometheus examples](#prometheus-examples)
- [Analytics table examples](#analytics-table-examples)
- [Sample ratio mismatch](#sample-ratio-mismatch)
- [Novelty effect и сезонность](#novelty-effect-и-сезонность)
- [Multiple testing](#multiple-testing)
- [Stopping rule](#stopping-rule)
- [Segments](#segments)
- [Decision framework](#decision-framework)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

## Три группы метрик

Для эксперимента обычно нужны три группы:

1. Primary metric.
2. Guardrail metrics.
3. Diagnostic metrics.

Пример для checkout experiment:

| Группа | Примеры | Зачем |
|---|---|---|
| Primary | purchase conversion | понять, выиграл ли treatment |
| Guardrail | payment error rate, p95 latency, refund rate | не принять вредную фичу |
| Diagnostic | clicks by step, form validation errors | понять причину результата |

Важно:
- primary metric должна быть одна или очень маленький набор;
- guardrails должны быть известны до запуска;
- diagnostic metrics помогают интерпретации, но не должны превращаться в post-hoc поиск победы.

## Primary metric

`Primary metric` - главная метрика успеха.

Примеры:
- `signup_conversion`;
- `purchase_conversion`;
- `activation_rate`;
- `search_success_rate`;
- `retention_d7`;
- `revenue_per_user`.

Хорошая primary metric:
- связана с гипотезой;
- измеряется одинаково для control и treatment;
- достаточно чувствительна;
- не ломает долгосрочную ценность.

Плохая primary metric:
- легко накручивается;
- не связана с целью;
- выбирается после просмотра результата;
- конфликтует с пользовательским опытом.

Пример:
- увеличить clicks на кнопку можно агрессивным UI;
- но если purchase conversion и support tickets ухудшились, это плохой trade-off.

## Guardrail metrics

`Guardrail metrics` - ограничения, которые не должны ухудшиться.

Примеры product guardrails:
- refund rate;
- unsubscribe rate;
- support contact rate;
- cancellation rate;
- complaint rate.

Примеры technical guardrails:
- HTTP 5xx rate;
- p95/p99 latency;
- timeout rate;
- CPU/memory saturation;
- queue lag;
- DB error rate.

Guardrail нужен, потому что treatment может улучшить primary metric ценой вреда в другом месте.

Пример:

```text
New search ranking increased clicks by 3%,
but p95 latency grew from 180 ms to 900 ms
and search abandonment increased.
```

Такой результат нельзя просто принять.

## Diagnostic metrics

`Diagnostic metrics` объясняют, почему результат изменился.

Примеры:
- page view -> CTA click -> form start -> form submit -> purchase;
- validation error by field;
- time to first interaction;
- search query reformulation rate;
- checkout step drop-off.

Diagnostic metrics полезны для:
- поиска bottleneck в funnel;
- объяснения отрицательного результата;
- подготовки следующего эксперимента.

Риск:
- если смотреть десятки diagnostic metrics и выбрать одну "значимую", легко получить false positive.

## Product events

Product analytics обычно строится на events.

Минимальный набор для эксперимента:
- `experiment_exposure`;
- business outcome events;
- funnel events;
- error or rejection events.

Пример exposure:

```json
{
  "event_name": "experiment_exposure",
  "experiment_key": "checkout_v2",
  "variant": "treatment",
  "unit_id": "user_123",
  "unit_type": "user",
  "surface": "checkout",
  "timestamp": "2026-04-20T10:30:00Z"
}
```

Пример outcome:

```json
{
  "event_name": "purchase_completed",
  "user_id": "user_123",
  "order_id": "ord_456",
  "amount_usd": 49.99,
  "experiments": {
    "checkout_v2": "treatment"
  },
  "timestamp": "2026-04-20T10:33:20Z"
}
```

Тонкости:
- events должны иметь consistent timestamp;
- дедупликация нужна для retry;
- experiment context должен попадать в outcome events;
- event schema нужно версионировать;
- raw user ids могут требовать privacy controls.

## Operational metrics

Operational metrics нужны не для статистического вывода о conversion, а для здоровья системы.

Примеры:
- `http_requests_total`;
- `http_request_duration_seconds`;
- `experiment_evaluations_total`;
- `experiment_evaluation_duration_seconds`;
- `experiment_config_last_refresh_timestamp_seconds`;
- `experiment_config_refresh_errors_total`.

Что не делать:
- не добавлять `user_id` как label;
- не добавлять произвольный `experiment_key`, если ключей очень много;
- не использовать Prometheus как product analytics warehouse.

Prometheus хорош для:
- rate;
- latency;
- errors;
- saturation;
- alerting.

Warehouse/OLAP лучше для:
- conversion;
- cohorts;
- funnels;
- retention;
- revenue;
- statistical analysis.

## Prometheus examples

Evaluation count:

```text
experiment_evaluations_total{
  service="checkout-api",
  experiment_key="checkout_v2",
  variant="treatment",
  result="matched"
}
```

Evaluation latency:

```text
experiment_evaluation_duration_seconds_bucket{
  service="checkout-api",
  le="0.005"
}
```

Config refresh:

```text
experiment_config_refresh_errors_total{
  service="checkout-api",
  provider="internal"
}
```

Alert examples:

```promql
sum(rate(experiment_config_refresh_errors_total[5m])) > 0
```

```promql
histogram_quantile(
  0.95,
  sum by (le) (
    rate(experiment_evaluation_duration_seconds_bucket[5m])
  )
) > 0.02
```

Для guardrail HTTP 5xx по treatment лучше использовать logs/events/traces или low-cardinality metrics, если количество экспериментов ограничено:

```promql
sum by (variant) (
  rate(http_requests_total{
    route="/checkout",
    experiment_key="checkout_v2",
    status_code=~"5.."
  }[5m])
)
/
sum by (variant) (
  rate(http_requests_total{
    route="/checkout",
    experiment_key="checkout_v2"
  }[5m])
)
```

Предупреждение:
- label `experiment_key` в Prometheus допустим только если cardinality контролируется;
- для множества параллельных экспериментов лучше писать experiment context в logs/events и агрегировать в analytics системе.

## Analytics table examples

Упрощенная таблица exposures:

```sql
CREATE TABLE experiment_exposures (
    experiment_key TEXT,
    variant TEXT,
    unit_type TEXT,
    unit_id TEXT,
    exposed_at TIMESTAMP,
    surface TEXT
);
```

Упрощенная таблица outcomes:

```sql
CREATE TABLE purchase_events (
    user_id TEXT,
    order_id TEXT,
    amount_usd NUMERIC,
    purchased_at TIMESTAMP
);
```

Пример conversion query:

```sql
WITH exposed AS (
    SELECT
        experiment_key,
        variant,
        unit_id,
        MIN(exposed_at) AS first_exposed_at
    FROM experiment_exposures
    WHERE experiment_key = 'checkout_v2'
    GROUP BY experiment_key, variant, unit_id
),
converted AS (
    SELECT DISTINCT
        e.variant,
        e.unit_id
    FROM exposed e
    JOIN purchase_events p
      ON p.user_id = e.unit_id
     AND p.purchased_at >= e.first_exposed_at
     AND p.purchased_at < e.first_exposed_at + INTERVAL '7 days'
)
SELECT
    e.variant,
    COUNT(DISTINCT e.unit_id) AS exposed_users,
    COUNT(DISTINCT c.unit_id) AS converted_users,
    COUNT(DISTINCT c.unit_id)::DECIMAL / COUNT(DISTINCT e.unit_id) AS conversion_rate
FROM exposed e
LEFT JOIN converted c
  ON c.variant = e.variant
 AND c.unit_id = e.unit_id
GROUP BY e.variant;
```

Что важно:
- считать от первого exposure;
- ограничить attribution window;
- дедуплицировать events;
- не смешивать users и sessions без причины.

## Sample ratio mismatch

`Sample ratio mismatch` - ситуация, когда группы получились не в ожидаемой пропорции.

Пример:
- ожидали 50/50;
- получили 62/38.

Возможные причины:
- bug в bucketing;
- targeting применяется после assignment;
- один variant чаще падает до exposure logging;
- ad blockers режут events неравномерно;
- cache отдает один variant чаще.

SRM - серьезный сигнал. Если он есть, результат эксперимента может быть недостоверен.

## Novelty effect и сезонность

`Novelty effect` - пользователи реагируют на новое поведение сильнее в первые дни.

Пример:
- новый UI увеличил clicks в первый день;
- через неделю эффект исчез.

Сезонность:
- weekday/weekend;
- paydays;
- holidays;
- marketing campaigns;
- release calendar.

Практическое правило:
- не запускать и не завершать важный тест на слишком коротком окне;
- для weekly behavior дать тесту пройти полный недельный цикл;
- не смешивать запуск эксперимента с крупной marketing campaign, если это можно избежать.

## Multiple testing

Если смотреть много метрик, сегментов и вариантов, шанс случайной "победы" растет.

Пример:
- 20 сегментов;
- 15 метрик;
- 4 variants;
- где-то почти наверняка найдется красивый uplift.

Как снижать риск:
- заранее объявить primary metric;
- ограничить число planned segment analyses;
- относиться к post-hoc findings как к гипотезам для новых тестов;
- использовать корректировки или более строгий decision process для множества сравнений.

## Stopping rule

Плохой stopping rule:

```text
Каждый час смотрим dashboard и останавливаем, когда treatment стал зеленым.
```

Почему плохо:
- повышается false positive rate;
- команда ловит случайный шум;
- результат плохо воспроизводится.

Лучше:
- заранее определить минимальную длительность;
- заранее определить sample size или detectable effect;
- иметь emergency stop только для guardrail violations;
- не менять decision rule после старта.

## Segments

Segments полезны, но опасны.

Примеры:
- country;
- platform;
- new/returning users;
- account tier;
- traffic source.

Правила:
- segment analysis должен объяснять результат, а не заменять primary decision;
- planned segments лучше post-hoc segments;
- маленькие сегменты дают шум;
- если treatment помогает mobile и вредит web, нужен продуктовый decision, а не простое среднее.

## Decision framework

После теста варианты решения:

| Результат | Решение |
|---|---|
| Primary улучшилась, guardrails ok | rollout или следующий этап |
| Primary neutral, guardrails ok | не раскатывать, если нет другой причины |
| Primary улучшилась, guardrail ухудшился | fix и повторить или ограничить сегмент |
| Primary ухудшилась | stop treatment |
| SRM или logging bug | не доверять результату, перезапустить |
| Сильный эффект только в planned segment | возможно rollout только для сегмента |

Хороший experiment review отвечает:
- что была за гипотеза;
- кого включили;
- сколько было exposure;
- какой был эффект на primary metric;
- что с guardrails;
- были ли data quality issues;
- какое решение и почему.

## Типичные ошибки

- Использовать operational canary metrics как доказательство product uplift.
- Считать conversion без exposure event.
- Считать все sessions, когда assignment был по user.
- Игнорировать sample ratio mismatch.
- Делать вывод по маленькому noisy segment.
- Останавливать тест при первом положительном p-value.
- Искать победу среди десятков метрик после запуска.
- Забывать про latency, errors, refunds и support load.
- Тащить product analytics в Prometheus с high-cardinality labels.

## Interview-ready answer

Для A/B теста я заранее выбираю primary metric и guardrails. Product effect считаю в analytics warehouse по exposure events и outcome events, а Prometheus использую для operational health: latency, error rate, config refresh, evaluator latency. Важно проверять sample ratio mismatch, не останавливать тест при первом красивом результате, не менять метрики после старта и не делать выводы по случайно найденным сегментам. Decision должен учитывать не только uplift, но и guardrails, data quality и стоимость rollout.
