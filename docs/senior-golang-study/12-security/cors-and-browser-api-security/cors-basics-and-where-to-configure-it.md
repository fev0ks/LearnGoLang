# CORS Basics And Where To Configure It

Эта заметка нужна, потому что `CORS` часто понимают очень расплывчато.

## Содержание

- [Самая короткая интуиция](#самая-короткая-интуиция)
- [Что такое origin](#что-такое-origin)
- [Что именно делает CORS](#что-именно-делает-cors)
- [Что CORS не делает](#что-cors-не-делает)
- [Simple request и preflight](#simple-request-и-preflight)
- [Где это обычно настраивают](#где-это-обычно-настраивают)
- [Когда CORS нужен, а когда нет](#когда-cors-нужен-а-когда-нет)
- [Частые ошибки](#частые-ошибки)
- [Practical Rule](#practical-rule)

## Самая короткая интуиция

Допустим:
- frontend живет на `https://app.example.com`
- API живет на `https://api.example.com`

С точки зрения браузера это разные origins.

Если frontend JS хочет вызвать API, браузер проверяет `CORS` policy.

То есть вопрос `CORS` такой:
- можно ли коду из одного origin читать ответ другого origin?

А не такой:
- можно ли вообще послать TCP/HTTP запрос в backend?

## Что такое origin

`Origin` — это комбинация:
- scheme
- host
- port

Например:
- `https://app.example.com`
- `https://api.example.com`
- `http://localhost:3000`
- `http://localhost:8080`

Это разные origins.

## Что именно делает CORS

Сервер или proxy отвечает специальными headers:
- `Access-Control-Allow-Origin`
- `Access-Control-Allow-Methods`
- `Access-Control-Allow-Headers`
- `Access-Control-Allow-Credentials`
- `Access-Control-Expose-Headers`

Браузер смотрит на них и решает:
- можно ли JS-коду прочитать ответ;
- можно ли послать credentialed request;
- можно ли использовать определенные custom headers и methods.

## Что CORS не делает

Очень важно:

`CORS` не:
- аутентифицирует клиента;
- не защищает от `curl`;
- не защищает от backend-to-backend вызовов;
- не заменяет CSRF protection;
- не заменяет rate limiting;
- не заменяет DDoS protection.

То есть `CORS` контролирует поведение браузера, а не “вообще доступ к API”.

## Simple request и preflight

### Simple request

Некоторые браузерные запросы идут сразу.

### Preflight

Если запрос сложнее:
- custom headers;
- `PUT`, `PATCH`, `DELETE`;
- credentials;
- certain content types,

то браузер сначала шлет:

```text
OPTIONS /api/...
```

с вопросом:
- можно ли вообще делать такой cross-origin request?

Сервер или gateway должен ответить нужными `Access-Control-*` headers.

Если ответ не совпал с ожиданием браузера:
- сам браузер заблокирует JS доступ к ответу.

## Где это обычно настраивают

Есть три нормальных места.

### 1. Reverse proxy / API gateway / ingress

Подходит, когда:
- policy единая для всего API;
- origins и methods predictable;
- не хочется дублировать `CORS` логику в каждом сервисе.

Плюсы:
- централизованная настройка;
- меньше дублирования;
- проще сопровождать browser-facing boundary.

Минусы:
- если разные endpoints требуют разных `CORS` правил, конфиг может стать неудобным;
- gateway не всегда знает тонкости доменной политики.

### 2. В приложении

Подходит, когда:
- правила зависят от route;
- часть API публичная, часть нет;
- у разных endpoint-ов разные allowed origins/headers/methods.

Плюсы:
- полная гибкость;
- policy ближе к конкретному handler/use case.

Минусы:
- легко размазать одинаковую `CORS` логику по многим сервисам;
- выше риск inconsistent behavior.

### 3. Смешанный подход

Например:
- gateway закрывает базовый `CORS` по умолчанию;
- отдельные чувствительные endpoints уточняют policy в app.

Но это уже надо делать аккуратно, чтобы не получить конфликт правил.

## Когда CORS нужен, а когда нет

### Нужен

Когда:
- у тебя browser frontend;
- frontend и API на разных origins;
- JS в браузере должен читать API response.

### Обычно не нужен

Когда:
- это server-to-server traffic;
- internal gRPC/HTTP between services;
- browser тут вообще не участвует.

## Частые ошибки

### 1. `Access-Control-Allow-Origin: *` вместе с credentials

Это обычно плохая идея и часто просто несовместимо с browser expectations.

Если нужны cookies или credentialed requests:
- allow-origin обычно должен быть конкретным, а не `*`.

### 2. “Раз есть CORS, значит API защищен”

Нет.

Атакующий может:
- использовать `curl`;
- использовать backend script;
- дергать endpoint вне browser environment.

`CORS` не блокирует это.

### 3. Путать CORS и CSRF

Это разные вещи.

`CORS`:
- управляет cross-origin reading rules в браузере.

`CSRF`:
- про то, что браузер может автоматически отправить cookies/session в нежелательный запрос.

То есть строгий `CORS` сам по себе не решает `CSRF` полностью.

### 4. Не обрабатывать preflight

Если `OPTIONS` path забыли:
- frontend “ломается”;
- backend может быть жив, но браузерный сценарий не работает.

## Practical Rule

Запомнить полезно так:

- `CORS` — это browser boundary policy;
- он нужен для frontend-to-API cross-origin сценариев;
- он не заменяет auth, CSRF protection, WAF, rate limiting или DDoS controls;
- если policy единая — удобно держать ее в gateway/proxy;
- если policy сильно зависит от endpoint-а — возможно, лучше настраивать в приложении.
