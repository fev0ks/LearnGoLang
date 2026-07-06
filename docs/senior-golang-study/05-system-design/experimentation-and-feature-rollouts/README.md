# Experimentation And Feature Rollouts

Этот раздел про A/B testing, feature flags, canary rollout, dark launch и похожие подходы, которые помогают менять продукт без "релиза вслепую".

Материалы полезны для backend/system design интервью, потому что эксперименты почти всегда затрагивают несколько слоев: UI, backend, assignment service, метрики, аналитику, privacy, observability и rollback.

## Как читать

1. Начни с видов экспериментов и rollout-подходов.
2. Затем отдельно разберись с feature flags (`02`). Если готовишься к Go system design interview — прочитай также `02a` с примерами реализации клиента.
3. После этого посмотри, как проектировать корректный A/B test.
4. Затем изучи общую реализацию experimentation на UI и backend.
5. В конце прочитай про метрики, анализ и типичные ловушки.

## Материалы

- [01. Виды экспериментов и rollout-подходов](./01-experimentation-and-rollout-types.md)
- [02. Feature flags на практике](./02-feature-flags-in-practice.md)
- [02a. Go implementation: feature flag client](./02a-feature-flags-golang-client.md)
- [03. Дизайн A/B теста и assignment](./03-ab-test-design-and-assignment.md)
- [04. Реализация на UI и backend](./04-ui-backend-implementation.md)
- [05. Метрики, анализ и типичные ошибки](./05-metrics-analysis-and-pitfalls.md)

## Короткая карта понятий

```mermaid
flowchart LR
    Idea["Product hypothesis"] --> Flag["Feature flag"]
    Flag --> Rollout["Gradual rollout"]
    Flag --> Experiment["A/B test"]
    Rollout --> Safety["Operational safety"]
    Experiment --> Decision["Product decision"]
    Decision --> FullRollout["100% rollout"]
    Decision --> Stop["Disable feature"]
```

Главная мысль:
- `feature flag` отвечает на вопрос "кому включить поведение";
- `rollout` отвечает на вопрос "как безопасно раскатывать";
- `A/B test` отвечает на вопрос "стало ли лучше по метрикам";
- `observability` отвечает на вопрос "не сломали ли мы систему";
- `analytics` отвечает на вопрос "что изменилось для пользователя и бизнеса".

## Interview-ready answer

**1. Что отличает настоящую систему A/B тестирования от `if user in group A`?**

- Стабильное распределение пользователей по группам, контроль конфликтов между экспериментами, единое логирование exposure events, заранее выбранные primary/guardrail метрики и быстрый rollback через feature flag.

**2. Чем canary отличается от A/B теста?**

- Canary и gradual rollout — про operational risk («не ломает ли новая версия сервис»); A/B тест — про проверку продуктовой гипотезы статистически корректным способом. Одно не заменяет другое.
