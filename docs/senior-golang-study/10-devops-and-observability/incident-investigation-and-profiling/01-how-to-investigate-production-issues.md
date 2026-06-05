# How To Investigate Production Issues

Хорошее расследование инцидента идет по слоям, а не по интуиции.

## Плохой подход

- сразу лезть в код;
- сразу винить базу;
- смотреть один случайный график;
- чинить до локализации проблемы.

## Нормальный порядок

1. Понять symptom
- растут ошибки;
- растет latency;
- падает throughput;
- растет backlog;
- жалуются пользователи.

2. Понять scope
- одна ручка или весь сервис;
- один pod или все инстансы;
- один регион или все регионы;
- после деплоя или без видимого изменения.

3. Проверить три сигнала
- metrics;
- logs;
- traces.

4. Разделить проблему по слоям
- edge и network;
- gateway and proxy;
- application;
- database and cache;
- async systems.

## Что смотреть сначала

Metrics:
- error rate;
- p95 and p99 latency;
- RPS;
- saturation CPU, memory, goroutines, pools.

Logs:
- spikes по error_kind;
- correlation по request id и trace id;
- новые error patterns.

Traces:
- какой span стал длинным;
- где основной wait;
- это DB, downstream call, queueing или app logic.

## Полезный mental model

Сначала ответь на три вопроса:
- проблема до приложения или внутри приложения;
- проблема в sync path или async path;
- проблема общая или локальная для одного компонента.

## После локализации

Только потом уже:
- rollback;
- config change;
- feature flag off;
- scaling;
- hotfix.

## Хороший interview ответ

Сильный ответ не "я открываю логи", а:
- "я сначала ограничу blast radius и пойму scope"
- "потом сверю metrics, logs и traces"
- "затем локализую проблему по слоям и только после этого выберу действие"
