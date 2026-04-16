# DDoS Protection

Эта заметка нужна, чтобы понимать `DDoS protection` не как абстрактную галочку, а как конкретный класс perimeter-защиты.

## Содержание

- [Самая короткая интуиция](#самая-короткая-интуиция)
- [Почему это perimeter-задача](#почему-это-perimeter-задача)
- [Какие бывают DDoS-атаки](#какие-бывают-ddos-атаки)
- [Чем DDoS protection отличается от rate limiting](#чем-ddos-protection-отличается-от-rate-limiting)
- [Что обычно делает DDoS protection слой](#что-обычно-делает-ddos-protection-слой)
- [Какую проблему это решает для backend](#какую-проблему-это-решает-для-backend)
- [Как DDoS защита сочетается с другими слоями](#как-ddos-защита-сочетается-с-другими-слоями)
- [Как выглядит нормальная layered защита](#как-выглядит-нормальная-layered-защита)
- [Что важно уметь объяснить на интервью](#что-важно-уметь-объяснить-на-интервью)
- [Practical Rule](#practical-rule)

## Самая короткая интуиция

`DDoS` = `Distributed Denial of Service`.

Цель атаки:
- не украсть данные;
- не обойти auth;
- а сделать сервис недоступным или сильно деградировавшим.

То есть атакующий хочет:
- занять канал;
- занять соединения;
- перегрузить edge/LB/app;
- выбить сервис по latency или availability.

## Почему это perimeter-задача

`DDoS protection` почти всегда должна стоять раньше backend-сервисов.

Почему:
- если мусорный трафик уже дошел до приложения, ты уже потратил часть CPU, памяти, connection slots и bandwidth;
- backend — слишком дорогая точка для первой линии защиты;
- на edge проще отбросить заведомо вредный traffic раньше, чем он займет внутренние ресурсы.

Полезная mental model:

```text
internet traffic
  -> edge / CDN / cloud perimeter
  -> load balancer / proxy
  -> gateway / ingress
  -> backend service
```

Чем раньше ты останавливаешь атаку, тем меньше blast radius.

## Какие бывают DDoS-атаки

### Volumetric

Это атаки на объем:
- слишком много пакетов;
- слишком много байтов;
- забивают канал или инфраструктуру на уровне сети.

Что страдает:
- bandwidth;
- edge capacity;
- network appliances.

Это часто не про приложение как таковое, а про инфраструктуру до него.

### Protocol-level

Это атаки на сетевые или транспортные особенности:
- SYN flood;
- connection exhaustion;
- malformed protocol traffic.

Что страдает:
- load balancer;
- TCP stack;
- connection tables;
- reverse proxy.

### L7 / application-layer DDoS

Это уже похоже на “обычные HTTP запросы”, но в разрушительном количестве.

Примеры:
- огромное число `GET /search`;
- дорогие запросы к тяжелому endpoint;
- flood на login/register/search API;
- трафик, который выглядит как “легитимный HTTP”, но по объему/паттерну разрушителен.

Это особенно неприятно, потому что:
- traffic может выглядеть почти нормальным;
- защита сложнее, чем просто фильтр по IP.

## Чем DDoS protection отличается от rate limiting

Это важное различие.

### DDoS protection

Решает perimeter-level задачу:
- как не дать разрушительному traffic вообще убить систему.

Обычно живет на:
- CDN;
- edge;
- cloud perimeter;
- L4/L7 DDoS appliances.

### Rate limiting

Решает API/business-level задачу:
- как ограничить честный и нечестный usage одного endpoint-а.

Обычно живет на:
- gateway;
- ingress;
- application;
- specific API route.

Пример:
- `100 req/min per IP on create endpoint` — это rate limiting;
- “в нас внезапно летит 500k rps мусорного traffic” — это уже DDoS problem.

То есть rate limit помогает против части abuse, но не заменяет DDoS protection.

## Что обычно делает DDoS protection слой

### 1. Early drop suspicious traffic

Самая базовая задача:
- отбрасывать traffic до того, как он дойдет до origin.

### 2. Traffic shaping

Например:
- ограничить бурст;
- ввести challenge;
- перераспределить или absorb load на edge.

### 3. Network and protocol filtering

Особенно важно для:
- SYN floods;
- malformed packets;
- weird connection behavior.

### 4. Reputation-based filtering

Например:
- suspicious ASNs;
- known bad IP ranges;
- obvious malicious sources.

### 5. Shielding expensive origins

Если origin дорогой:
- тяжелый search backend;
- payment API;
- login flow;

то perimeter защита особенно важна, потому что дорогой backend ломается быстрее.

## Какую проблему это решает для backend

Без perimeter DDoS защиты backend сталкивается с такими рисками:
- рост latency;
- saturation thread/connection pools;
- timeout cascade;
- отказ health checks;
- auto-scaling не успевает;
- полная потеря доступности.

С нормальной perimeter-защитой:
- значительная часть мусорного traffic вообще не доходит до backend;
- edge или cloud perimeter absorbit нагрузку;
- внутренний трафик остается ближе к нормальному профилю.

## Как DDoS защита сочетается с другими слоями

### CDN / Edge

Очень хороший первый слой:
- absorb traffic;
- cache static content;
- challenge suspicious clients;
- block obvious volumetric attacks.

### WAF

Полезен рядом, но это не одно и то же.

`WAF`:
- ловит подозрительные application patterns;
- защищает от типовых web attack vectors.

`DDoS protection`:
- в первую очередь защищает availability и capacity.

### API Gateway

Gateway помогает:
- quotas;
- API-specific rate limits;
- auth;
- policy controls.

Но это не главный DDoS shield.

### Application

Приложение тоже может помогать:
- дешево отвечать на overload;
- ограничивать дорогие path;
- иметь graceful degradation.

Но оно не должно быть первой линией обороны.

## Как выглядит нормальная layered защита

Примерно так:

```text
edge / CDN / cloud DDoS shield
  -> reverse proxy / LB
  -> gateway / ingress rate limits
  -> application-level abuse controls
```

Это важно, потому что одна мера почти никогда не закрывает все сценарии.

## Что важно уметь объяснить на интервью

- `DDoS protection` — это про availability, а не про auth.
- Ее лучше ставить раньше приложения.
- Volumetric и L7 DDoS — разные вещи.
- Rate limiting полезен, но это не полноценная замена DDoS protection.
- Backend должен быть защищен несколькими слоями, а не одним magical control.

## Practical Rule

Если коротко:
- `DDoS protection` нужна, чтобы не пустить разрушительный traffic внутрь системы;
- это perimeter concern;
- edge/CDN/cloud shield почти всегда лучшее место для такой защиты;
- gateway и app могут помогать, но не должны быть первой и единственной линией обороны.
