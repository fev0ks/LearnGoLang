# Проектирование REST API

REST — архитектурный стиль, а не протокол. Это значит, что "REST API" не означает автоматически правильный API: можно отправлять POST вместо GET, класть глаголы в URL и путать query с body — и всё это будет работать. Но клиенты будут страдать, а ошибки проектирования будут множиться.

Эта статья — практическое руководство по правильному проектированию REST API на основе реального опыта. Разобраны типичные ошибки, которые встречаются даже в production-системах, и показано, как их исправить.

## Содержание

- [Ресурсы, а не действия](#ресурсы-а-не-действия)
- [Правила именования URL](#правила-именования-url)
- [HTTP-методы: семантика важна](#http-методы-семантика-важна)
- [Path, Query, Body: когда что](#path-query-body-когда-что)
- [Типичные ошибки](#типичные-ошибки)
  - [1. Глаголы в URL](#1-глаголы-в-url)
  - [2. Суффикс /list в конце коллекции](#2-суффикс-list-в-конце-коллекции)
  - [3. ID ресурса в query param вместо path](#3-id-ресурса-в-query-param-вместо-path)
  - [4. POST для read-запросов](#4-post-для-read-запросов)
  - [5. Непоследовательность singular/plural](#5-непоследовательность-singularplural)
  - [6. camelCase и snake_case в URL](#6-camelcase-и-snake_case-в-url)
  - [7. Аббревиатуры в URL](#7-аббревиатуры-в-url)
  - [8. Действие как отдельный endpoint вместо PATCH](#8-действие-как-отдельный-endpoint-вместо-patch)
  - [9. Дробление одного ресурса по разным сервисам](#9-дробление-одного-ресурса-по-разным-сервисам)
- [Особые случаи](#особые-случаи)
  - [State machine переходы](#state-machine-переходы)
  - [Поиск и фильтрация](#поиск-и-фильтрация)
  - [Bulk-операции](#bulk-операции)
  - [gRPC-gateway: ограничения GET с body](#grpc-gateway-ограничения-get-с-body)
- [Проектирование Response](#проектирование-response)
  - [Единый конверт ответа](#единый-конверт-ответа)
  - [Коллекции и пагинация](#коллекции-и-пагинация)
  - [Даты и временные метки](#даты-и-временные-метки)
  - [Деньги и суммы](#деньги-и-суммы)
  - [Статусы и состояния](#статусы-и-состояния)
  - [Неструктурированные данные](#неструктурированные-данные)
  - [Nullability и опциональные поля](#nullability-и-опциональные-поля)
  - [userId в теле запроса для авторизованных эндпоинтов](#userid-в-теле-запроса-для-авторизованных-эндпоинтов)
  - [Опечатки и именование полей](#опечатки-и-именование-полей)
- [Чеклист перед выпуском](#чеклист-перед-выпуском)
- [Interview-ready answer](#interview-ready-answer)

---

## Ресурсы, а не действия

Главная идея REST: **URL — это адрес ресурса, а не команда**.

Ресурс — это существительное. Сущность или коллекция сущностей, с которой работает API. Действие над ресурсом выражается HTTP-методом, а не словом в URL.

```
# RPC-style (не REST)
POST /createUser
POST /getUserById
POST /deleteUser
POST /updateUserEmail

# REST-style
POST   /users           — создать
GET    /users/{id}      — получить
DELETE /users/{id}      — удалить
PATCH  /users/{id}      — обновить
```

Разница принципиальная: в RPC-style каждая операция — отдельный endpoint с глаголом. В REST — один ресурс (`/users/{id}`) с разными HTTP-методами, каждый из которых имеет чёткую семантику.

### Иерархия ресурсов

Если ресурс принадлежит другому ресурсу — это отражается в URL:

```
/users/{userId}/orders          — заказы конкретного пользователя
/users/{userId}/orders/{id}     — конкретный заказ пользователя
/orders/{id}/items              — позиции конкретного заказа
/orders/{id}/items/{itemId}     — конкретная позиция
```

Правило: не делайте иерархию глубже 3 уровней — URL становится нечитаемым. Если ресурс доступен из нескольких точек, часто лучше сделать flat-ресурс с фильтрацией:

```
# Вместо /users/{userId}/orders/{id}/items/{itemId}
GET /order-items/{id}           — flat, проще
GET /order-items?orderId={id}   — с фильтрацией
```

---

## Правила именования URL

| Правило | Неверно | Верно |
|---|---|---|
| Существительные, не глаголы | `/getUser`, `/createOrder` | `/users`, `/orders` |
| Коллекции во множественном числе | `/user`, `/order` | `/users`, `/orders` |
| Один стандарт: plural везде | `/user/{id}/orders` | `/users/{id}/orders` |
| kebab-case, не camelCase | `/contactInfo`, `/orderSummary` | `/contact-info`, `/order-summary` |
| kebab-case, не snake_case | `/promo_code`, `/order_items` | `/promo-codes`, `/order-items` |
| Без аббревиатур | `/rec/bundle/acc/`, `/pr-transfer` | `/recommendations/bundles/accommodations/` |
| ID ресурса в path, не query | `GET /orders?id=123` | `GET /orders/123` |
| Без суффикса /list | `/orders/list` | `/orders` |
| Версия в prefix | `/orders/v1` | `/v1/orders` |

---

## HTTP-методы: семантика важна

| Метод | Назначение | Идемпотентный | Безопасный | Body |
|---|---|---|---|---|
| `GET` | Получить ресурс или коллекцию | ✓ | ✓ | — |
| `POST` | Создать ресурс | ✗ | ✗ | ✓ |
| `PUT` | Заменить ресурс целиком | ✓ | ✗ | ✓ |
| `PATCH` | Частично обновить ресурс | ✓* | ✗ | ✓ |
| `DELETE` | Удалить ресурс | ✓ | ✗ | — |

**Идемпотентный** — повторный вызов с теми же параметрами даёт тот же результат.  
**Безопасный** — не изменяет состояние сервера.

### GET

Только для чтения. Никогда не изменяет состояние. Может кешироваться браузером и CDN.

```
GET /users/123          → 200 + объект пользователя
GET /users              → 200 + массив пользователей
GET /users?active=true  → 200 + отфильтрованный массив
```

### POST

Создаёт новый ресурс. Не идемпотентен — два одинаковых запроса создадут два ресурса.

```
POST /orders
Body: { "userId": "123", "items": [...] }

→ 201 Created
  Location: /orders/456
  Body: { "id": "456", ... }
```

### PUT

Полная замена ресурса. Клиент передаёт весь объект. Если поле не передано — оно затирается. Идемпотентен.

```
PUT /users/123
Body: { "name": "Alice", "email": "alice@example.com", "phone": null }

→ 200 OK  (или 204 No Content)
```

### PATCH

Частичное обновление. Клиент передаёт только изменяемые поля.

```
PATCH /users/123
Body: { "phone": "+7-999-123-45-67" }

→ 200 OK + обновлённый объект  (или 204 No Content)
```

### DELETE

Удаляет ресурс. Идемпотентен: удаление уже удалённого ресурса возвращает 404 (или 204 — зависит от соглашения).

```
DELETE /users/123  →  204 No Content
DELETE /users/123  →  404 Not Found  (повторный)
```

---

## Path, Query, Body: когда что

Неправильный выбор параметра — одна из самых частых ошибок. Правило простое:

| Тип параметра | Когда использовать | Пример |
|---|---|---|
| **Path** | Идентификатор конкретного ресурса | `/orders/{id}` |
| **Query** | Фильтры, сортировка, пагинация, опциональные параметры | `?status=active&page=2&sort=created_at` |
| **Body** | Данные для создания/изменения ресурса | `POST /orders { items: [...] }` |

```
# Неверно: ID ресурса в query
GET /orders?id=123
GET /orders/checkout?orderId=456

# Верно: ID в path
GET /orders/123
GET /orders/456/checkout
```

```
# Неверно: фильтр в path
GET /orders/active
GET /orders/page/2

# Верно: фильтр в query
GET /orders?status=active
GET /orders?page=2
```

```
# Неверно: данные создания в query
POST /users?name=Alice&email=alice@example.com

# Верно: данные в body
POST /users
Body: { "name": "Alice", "email": "alice@example.com" }
```

---

## Типичные ошибки

### 1. Глаголы в URL

**Антипаттерн:**

```
POST /orders/prebook
POST /orders/refresh
POST /orders/confirm
GET  /orders/check
GET  /users/find
POST /bundles/update
POST /bundles/details
```

Проблема: API превращается в набор RPC-методов. Глаголы не дают ответа на вопрос «какой ресурс здесь?», семантика метода теряется, endpoint'ы множатся бесконтрольно.

**Как правильно:**

Найти существительное, которое описывает результат действия:

```
# prebook создаёт предбронирование
POST /orders/{id}/prebooking

# refresh обновляет цены — это PATCH на ресурс prices
PATCH /offers/{id}/prices

# confirm подтверждает платёж
POST /payments/{id}/confirmation  (или PATCH /payments/{id})

# check возвращает статус — это GET на ресурс
GET /orders/{id}/status

# find — это GET с фильтром
GET /users?email=alice@example.com

# update hotel room — это PATCH на ресурс
PATCH /offers/{offerId}/hotel-room

# details — это GET на вложенный ресурс
GET /offers/{offerId}/accommodation/{accId}
```

Если действие сложно выразить через существительное (оно существует, просто надо поискать) — допустимы sub-resource actions. Об этом в разделе [State machine переходы](#state-machine-переходы).

---

### 2. Суффикс /list в конце коллекции

**Антипаттерн:**

```
GET /orders/list
GET /users/quiz/session/list
GET /orders/payment/plan/draft/list
GET /bundles/history/list
```

Проблема: коллекция и так коллекция. `/orders` уже означает «список заказов». Суффикс `/list` — шум, который выглядит как рудимент RPC-мышления (`listOrders()`).

Дополнительная проблема: теперь `/orders` и `/orders/list` — разные URL, один из которых может вернуть 404, а другой — данные. Для клиента это сюрприз.

**Как правильно:**

```
GET /orders                           — список заказов
GET /users/{id}/quiz-sessions         — список квиз-сессий пользователя
GET /orders/{id}/payment-plan/drafts  — список вариантов плана
GET /bundles/history                  — история (если history сам является ресурсом)
```

Коллекция — это plural noun без суффиксов.

---

### 3. ID ресурса в query param вместо path

**Антипаттерн:**

```
GET /orders/checkout?orderId=abc123
GET /orders/summary?orderId=abc123
GET /orders/status?orderId=abc123
GET /orders/voucher?orderId=abc123
GET /promo-codes?id=xyz789
```

Проблема: query параметр — опциональный. Это значит API должен обрабатывать случай «без orderId», а клиент должен знать, что именно этот параметр обязателен. Кеширование работает хуже: CDN неохотно кешируют URL с query params. URL не выражает иерархию.

**Как правильно:**

ID конкретного ресурса — всегда в path:

```
GET /orders/{id}/checkout
GET /orders/{id}/summary
GET /orders/{id}/status
GET /orders/{id}/voucher
GET /promo-codes/{id}
```

Query params — только для опциональных фильтров и параметров выборки:

```
GET /orders?status=active&page=2          — фильтр коллекции
GET /orders/{id}?include=items,payments   — опциональное расширение ответа
```

---

### 4. POST для read-запросов

**Антипаттерн:**

```
POST /bundles           — получить список бандлов
POST /bundles/details   — получить детали бандла
POST /quiz/page-info    — получить информацию о странице
```

Почему это происходит: разработчики хотят передать сложный объект в теле запроса, а GET с body — плохая практика (многие прокси и серверы его игнорируют). Решение выглядит очевидным: POST.

Проблемы:
- POST не идемпотентен — клиент не знает, можно ли повторить запрос
- Браузер и CDN не кешируют POST-ответы
- Семантика POST ("создать что-то") нарушена
- Клиент не ожидает, что `POST /bundles` вернёт список, а не создаст бандл

**Как правильно:**

Вариант 1 — переосмыслить параметры. Часто "сложный объект" — это просто несколько query params:

```
# Вместо POST /bundles с body { searchCriteriaId, filters... }
GET /bundles?search-criteria-id=abc&resort=val-thorens&dates=2025-01-15
```

Вариант 2 — если параметры реально сложные, использовать "search resource":

```
# Создать сохранённый поиск (POST — создание ресурса)
POST /searches
Body: { "criteria": { "resort": "...", "dates": {...}, "guests": 2 } }
→ 201 Created: { "searchId": "xyz" }

# Получить результаты по ID поиска (GET — чтение)
GET /searches/xyz/results
```

Вариант 3 — для ad-hoc поиска без сохранения: принять компромисс и документировать явно, что это read-операция:

```
# Некоторые команды допускают POST для поиска, называя ресурс явно
POST /bundle-searches
Body: { "criteria": {...} }
→ 200 OK: { "bundles": [...] }
```

---

### 5. Непоследовательность singular/plural

**Антипаттерн:**

```
GET  /bundle/saved/{id}          — singular
GET  /bundles/{id}/pr-transfer   — plural
POST /rec/bundle                 — singular
GET  /rec/bundles/pr-transfer    — plural (тот же ресурс!)
```

Проблема: клиент вынужден помнить, какой endpoint использует plural, а какой — singular. Это особенно болезненно при автогенерации SDK.

**Как правильно:**

Один стандарт для всего API. Большинство style guide рекомендуют **plural везде**:

```
GET    /bundles              — коллекция
GET    /bundles/{id}         — конкретный элемент
GET    /bundles/{id}/hotels  — вложенная коллекция
GET    /bundles/{id}/hotels/{hotelId}   — вложенный элемент
DELETE /bundles/{id}/transfers/{transferId}
```

Singular допустим только для singleton-ресурса — ресурса, существующего в единственном экземпляре в контексте:

```
GET /users/{id}/profile      — у пользователя один профиль
GET /orders/{id}/receipt     — у заказа один чек
PUT /settings                — глобальные настройки системы (один экземпляр)
```

---

### 6. camelCase и snake_case в URL

**Антипаттерн:**

```
POST /order/details/contactInfo
GET  /admin/promo_code
GET  /user/quiz/sessionList
POST /quiz/session/pageInfo
```

Проблема: URL case-sensitive на большинстве платформ. camelCase в URL — нестандартно, выглядит как ошибка. snake_case — нестандартно для путей (snake_case принято для query params в некоторых стилях, но не для path сегментов).

RFC 3986 не запрещает camelCase, но HTTP best practices и большинство крупных API (GitHub, Stripe, Google, Twilio) используют **kebab-case**.

**Как правильно:**

Только **kebab-case** в path сегментах:

```
POST /orders/{id}/contact-info
GET  /admin/promo-codes
GET  /users/{id}/quiz-sessions
POST /quiz/sessions/page-info
```

Query params — отдельная история. snake_case (`?sort_by=created_at`) или camelCase (`?sortBy=createdAt`) оба встречаются, главное — консистентность внутри API. GitHub, Stripe используют snake_case.

---

### 7. Аббревиатуры в URL

**Антипаттерн:**

```
/rec/bundle           — rec = recommendations?
/bundle/acc/details   — acc = accommodation? account?
/bundle/pr-transfer   — pr = private? pricing?
```

Проблема: аббревиатуры создают «тайное знание». Новый разработчик, видя `/rec/bundle/acc/`, не знает что это, пока не прочтёт документацию или не спросит коллегу. Это нарушает принцип self-describing API.

Особенно проблематично когда аббревиатура имеет несколько возможных расшифровок.

**Как правильно:**

Полные слова, читаемые как документация:

```
/recommendations/bundles
/recommendations/bundles/{id}/accommodations
/recommendations/bundles/{id}/private-transfers
```

Да, URL станет длиннее. Это нормально. Длина URL не влияет на производительность (разница в несколько байт). Читаемость влияет на скорость разработки, онбординг и число ошибок.

Исключение — общепринятые аббревиатуры (`api`, `id`, `url`, `http`, `cdn`, `sku`).

---

### 8. Действие как отдельный endpoint вместо PATCH

**Антипаттерн:**

```
POST /admin/promo-codes/active       — включить/выключить промокод
POST /orders/payment/plan            — установить план оплаты
POST /users/email                    — установить email
```

Проблема: вместо изменения атрибута ресурса создаётся отдельный endpoint с семантикой "выполнить действие". Это снова RPC-style. Клиент должен знать специальный URL для каждого атрибута.

**Как правильно:**

Для изменения атрибута ресурса — `PATCH`:

```
# Включить/выключить промокод
PATCH /admin/promo-codes/{id}
Body: { "active": true }

# Изменить план оплаты заказа
PATCH /orders/{id}/payment
Body: { "plan": "installment", "installments": 3 }

# Установить email пользователя
PATCH /users/{id}
Body: { "email": "alice@example.com" }
```

Когда действие всё же нужно как endpoint (state machine, side effects) — смотри раздел [State machine переходы](#state-machine-переходы).

---

### 9. Дробление одного ресурса по разным сервисам

**Антипаттерн:**

```
# Четыре разных сервиса, один ресурс /orders
BookingService:  GET /orders/checkout
PaymentService:  POST /orders/payment/prepare
AddonService:    GET /orders/addon/skipass
AssistantService: GET /assistant/user/orders
```

Проблема: для работы с одним заказом клиент обращается к 4 разным "сервисам" с разными базовыми путями. Фронтенд вынужден знать внутреннюю структуру бэкенда. При ошибке непонятно, какой сервис виноват.

Это проблема не REST, а проектирования API Gateway / BFF (Backend for Frontend). Внутренняя декомпозиция на микросервисы — это деталь реализации, которая не должна протекать в публичный API.

**Как правильно:**

Публичный API должен выглядеть как единый ресурс, независимо от того, сколько сервисов его обслуживают:

```
# Единый ресурс /orders
GET  /orders/{id}                    — данные заказа
GET  /orders/{id}/checkout           — checkout-информация
POST /orders/{id}/payment            — инициировать оплату
GET  /orders/{id}/addons/skipasses   — доступные аддоны
GET  /orders/{id}/addons/insurance   — страховки

# Отдельная коллекция для просмотра всех заказов
GET /orders?userId={id}              — все заказы пользователя
```

API Gateway агрегирует вызовы к нескольким внутренним сервисам и отдаёт клиенту единый response. Это задача BFF или Aggregation layer.

---

## Особые случаи

### State machine переходы

Некоторые бизнес-операции — не просто изменение атрибута. Это переход состояния со своей логикой, валидацией и side effects. PATCH не всегда подходит.

Паттерн: **sub-resource action** — существительное, описывающее результат действия.

```
# Заказ: черновик → подтверждённый
POST /orders/{id}/confirmation

# Оплата: создана → подтверждена
POST /payments/{id}/confirmation

# Аккаунт: активный → заблокированный
POST /users/{id}/suspension

# Квиз: начат → завершён (создание ресурса "завершение")
POST /quiz-sessions/{id}/completion
```

Почему POST, а не PATCH? Потому что это не просто изменение поля `status`. Это событие с side effects: отправка письма, списание денег, запуск воркера. POST явно говорит «это операция с последствиями».

Почему существительное, а не глагол? Потому что `confirmation` — это ресурс (подтверждение), а не команда. Это позволяет позже добавить `GET /orders/{id}/confirmation` для получения деталей подтверждения.

**Квиз-сессия как пошаговый процесс:**

Пошаговые процессы (мастер, квиз, checkout) — частный случай state machine. Хороший паттерн — моделировать шаги как ресурсы:

```
# Создать сессию
POST /quiz-sessions
→ { "sessionId": "abc", "currentStep": { ... } }

# Получить следующий шаг (создание ресурса "ответ")
POST /quiz-sessions/{id}/answers
Body: { "questionId": "q1", "answer": "option_b" }
→ { "nextStep": { ... } }

# Завершить (создание ресурса "завершение")
POST /quiz-sessions/{id}/completion
→ { "resultsId": "xyz" }
```

Каждый шаг — это создание нового вложенного ресурса (ответ, завершение). Навигация назад:

```
DELETE /quiz-sessions/{id}/answers/{answerId}   — отменить последний ответ
```

---

### Поиск и фильтрация

Простой поиск — query params:

```
GET /products?q=ski+boots&category=footwear&price_max=300&in_stock=true
GET /users?email=alice@example.com
GET /orders?status=pending&created_after=2025-01-01
```

Сложный поиск с сохранением — отдельный ресурс:

```
# Сохранить поисковый запрос
POST /searches
Body: {
  "filters": {
    "resort": "val-thorens",
    "dates": { "from": "2025-02-01", "to": "2025-02-08" },
    "guests": { "adults": 2, "children": 1 }
  }
}
→ 201: { "searchId": "abc123" }

# Получить результаты (с пагинацией)
GET /searches/abc123/results?page=1&per_page=20

# Обновить критерии (PUT = полная замена)
PUT /searches/abc123
Body: { "filters": { ... } }

# Получить историю поисков
GET /searches?userId={id}
```

Поиск по всему сайту:

```
GET /search?q=query&type=products,users&page=1
```

---

### Bulk-операции

Когда нужно создать/удалить/обновить несколько ресурсов за один запрос:

```
# Bulk-создание
POST /orders/{id}/items/bulk
Body: { "items": [{ ... }, { ... }] }

# Bulk-удаление
DELETE /orders/{id}/items
Body: { "ids": ["item-1", "item-2"] }   (body в DELETE спорен, но допустим)

# Альтернатива через query
DELETE /orders/{id}/items?ids=item-1,item-2

# Bulk-обновление (patch коллекции)
PATCH /products
Body: { "updates": [{ "id": "p1", "price": 150 }, { "id": "p2", "price": 200 }] }
```

Для асинхронных bulk-операций — паттерн job:

```
POST /import-jobs
Body: { "type": "products", "data": [...] }
→ 202 Accepted: { "jobId": "job-123" }

GET /import-jobs/job-123
→ { "status": "processing", "progress": 45 }
```

---

### gRPC-gateway: ограничения GET с body

gRPC-gateway транслирует HTTP → gRPC. Здесь важное ограничение: **GET-запросы не могут иметь body**. HTTP/1.1 технически это допускает, но прокси, браузеры и многие клиентские библиотеки игнорируют body в GET-запросах.

Поэтому в API с grpc-gateway часто возникает искушение использовать POST для запросов, которые семантически являются read-операциями, но требуют передачи сложных параметров.

**Правильные решения:**

1. Перевести параметры в query string (работает для большинства случаев):
```protobuf
rpc GetBundles(GetBundlesRequest) returns (BundlesResponse) {
  option (google.api.http) = {
    get: "/v1/bundles"
    // поля GetBundlesRequest автоматически маппятся в query params
  };
}
```

2. Для действительно сложных параметров — использовать search resource (POST создаёт search, GET получает результаты).

3. Принять компромисс и использовать POST с явным именем "search" или "query":
```protobuf
rpc SearchBundles(SearchBundlesRequest) returns (BundlesResponse) {
  option (google.api.http) = {
    post: "/v1/bundles/search"
    body: "*"
  };
}
```

Использование глагола `search` в данном случае — допустимое исключение, зафиксированное в Google API Design Guide.

---

## Проектирование Response

URL и методы — то, что видит разработчик при вызове API. Response — то, с чем он работает каждый день. Плохая структура ответа приводит к defensive coding на клиенте: куча проверок, магические числа, преобразования типов. Хорошая структура самодокументируется и не удивляет.

### Единый конверт ответа

**Антипаттерн — разные структуры для разных эндпоинтов:**

```json
// GET /orders/123 — голый объект
{ "orderId": "123", "status": "done", "total": 9900 }

// GET /orders — обёртка с полем list
{ "list": [ {...}, {...} ] }

// GET /orders/123/payment-plan — другая обёртка
{ "type": "full", "plan": { ... } }

// POST /orders — снова голый объект
{ "orderId": "456", "status": "new" }
```

Клиент вынужден знать, какой эндпоинт возвращает что. Автогенерация SDK и типов страдает.

**Как правильно — единый envelope:**

```json
// Все ответы имеют одну структуру
{
  "data": { ... },      // полезная нагрузка — объект или массив
  "meta": { ... },      // опционально: пагинация, версия, ttl
  "error": null         // или объект ошибки
}
```

```json
// Одиночный ресурс
{
  "data": {
    "id": "123",
    "status": "done",
    "total": { "amount": 9900, "currency": "EUR" }
  }
}

// Коллекция
{
  "data": [ {...}, {...} ],
  "meta": {
    "total": 42,
    "page": 1,
    "per_page": 20,
    "total_pages": 3
  }
}

// Ошибка
{
  "data": null,
  "error": {
    "code": "ORDER_NOT_FOUND",
    "message": "Order with id 123 not found"
  }
}
```

Envelope — это контракт. Клиент всегда знает, где искать данные и где ошибку. Можно добавить `meta` с новыми полями без нарушения backward compatibility.

Некоторые зрелые API (GitHub, Stripe) возвращают голые объекты для простых ресурсов и добавляют envelope только для коллекций. Это тоже валидный подход — главное, **консистентность**.

---

### Коллекции и пагинация

**Антипаттерн — три разных способа вернуть список:**

```json
// Вариант 1: поле list
{ "list": [ {...}, {...} ] }

// Вариант 2: поле orders
{ "orders": [ {...}, {...} ] }

// Вариант 3: поле options
{ "options": [ {...}, {...} ] }
```

Разные названия для одного и того же концепта — "список элементов". Клиент вынужден каждый раз смотреть в документацию.

**Как правильно — единое имя поля:**

Если используется envelope: поле `data` всегда содержит массив для коллекций.

Если нет envelope: поле всегда называется по имени ресурса во множественном числе.

```json
// GET /orders
{ "orders": [ ... ] }

// GET /users/123/quiz-sessions
{ "quiz_sessions": [ ... ] }

// GET /orders/123/payment-plan/drafts
{ "drafts": [ ... ] }
```

**Пагинация — обязательна для любой коллекции, которая может вырасти:**

```json
{
  "orders": [ ... ],
  "pagination": {
    "total": 150,
    "page": 2,
    "per_page": 20,
    "total_pages": 8,
    "has_next": true,
    "has_prev": true
  }
}
```

Коллекция без пагинации — это бомба замедленного действия. Когда в базе 10 записей — работает. Когда 100 000 — клиент ждёт минуту и получает timeout.

**Особый случай — cursor-based пагинация:**

```json
{
  "offers": [ ... ],
  "pagination": {
    "next_cursor": "eyJpZCI6MTIzfQ==",
    "prev_cursor": null,
    "has_next": true
  }
}
```

Cursor лучше offset-пагинации для реального времени (новые записи не смещают страницу) и для больших датасетов.

**Дополнительные поля коллекции — тоже в meta, не в корень:**

```json
// Антипаттерн — поля коллекции перемешаны с данными
{
  "list": [ ... ],
  "expireAt": 1700000000,
  "searchContext": { ... }
}

// Правильно — метаданные отдельно
{
  "offers": [ ... ],
  "meta": {
    "expires_at": "2025-01-15T10:30:00Z",
    "search_context": { ... }
  }
}
```

---

### Даты и временные метки

Это одна из самых частых причин конфликтов между бэкендом и фронтендом.

**Антипаттерн — unix timestamp как int64:**

```json
{
  "created_at": 1700000000,
  "expires_at": 1700086400,
  "paid_at": 1700100000,
  "date_of_birth": "1990-05-15"
}
```

Проблемы:
- `1700000000` — это секунды или миллисекунды? Нужно знать контракт.
- `new Date(1700000000)` в JS вернёт 1970 год (нужно `* 1000`). `new Date(1700000000000)` — правильно. Без документации неизвестно.
- Нет информации о timezone. UTC? Local? Не понятно.
- Смешивание форматов в одном response: `created_at: 1700000000` (unix) и `date_of_birth: "1990-05-15"` (строка). Клиент обрабатывает по-разному.

**Как правильно — ISO 8601 везде:**

```json
{
  "created_at": "2025-01-15T10:30:00Z",
  "expires_at": "2025-01-16T10:30:00Z",
  "paid_at": "2025-01-15T14:22:00Z",
  "date_of_birth": "1990-05-15"
}
```

ISO 8601 — стандарт. Парсится нативно в любом языке. Timezone явная (суффикс `Z` = UTC). Читается человеком в логах.

Единственный аргумент за unix — меньше байт. Это не проблема: JSON и так не бинарный формат, 4 лишних байта не имеют значения.

**Именование — единый стиль:**

```json
// Антипаттерн: mixed naming
{ "expiredAt": ..., "expireAt": ..., "payedAt": ... }

// Правильно: snake_case, глагол в прошедшем времени для событий,
// в будущем — для планируемых точек
{
  "created_at": "...",   // событие в прошлом
  "updated_at": "...",   // событие в прошлом
  "paid_at": "...",      // событие в прошлом (не payedAt — неправильное прошедшее)
  "expires_at": "...",   // точка в будущем (не expiredAt — это уже случилось)
  "due_at": "..."        // срок в будущем
}
```

---

### Деньги и суммы

**Антипаттерн — float для денег:**

```json
{ "total": 99.99 }
```

Никогда не используйте `float`/`double` для денег. Проблема классическая:

```
0.1 + 0.2 = 0.30000000000000004
```

Это не гипотетическая проблема. Это реальные баги в реальных платёжных системах.

**Правильно — minor units + currency:**

```json
{
  "total": {
    "amount": 9999,
    "currency": "EUR",
    "exponent": 2
  }
}
```

`amount: 9999` при `exponent: 2` означает `99.99 EUR`. Фронтенд делает `amount / 10^exponent` для отображения. Никаких float-операций с деньгами.

**Антипаттерн — поля с аббревиатурами и неочевидными именами:**

```json
{
  "acc": 5000,
  "publicTransfer": 1200,
  "privateTransfer": 800,
  "total": 7000,
  "finalTotal": 6500
}
```

`acc` — accommodation? account? В чём разница между `total` и `finalTotal`? Фронтенд гадает.

**Правильно — явные имена и структура:**

```json
{
  "breakdown": {
    "accommodation":       { "amount": 5000, "currency": "EUR" },
    "public_transfer":     { "amount": 1200, "currency": "EUR" },
    "private_transfer":    { "amount":  800, "currency": "EUR" },
    "addons":              { "amount":  500, "currency": "EUR" }
  },
  "subtotal":  { "amount": 7500, "currency": "EUR" },
  "discount":  { "amount":  500, "currency": "EUR" },
  "total":     { "amount": 7000, "currency": "EUR" }
}
```

`subtotal` — до скидок. `total` — итог. Нет двух полей "total" с разным смыслом.

---

### Статусы и состояния

**Антипаттерн — internal states протекают в API:**

```json
{ "status": "BookingStartFailed" }
{ "status": "BookingFinishedWithError" }
{ "status": "PaymentCanceled" }
```

Клиент не должен знать о внутренней декомпозиции бэкенда. Разница между `BookingFailed` и `BookingFinishedWithError` — деталь реализации. Фронтенд всё равно покажет пользователю "Что-то пошло не так".

**Правильно — user-facing статусы:**

```json
// Маппинг на клиентские состояния на бэкенде
{ "status": "failed" }          // BookingFailed | BookingFinishedWithError | BookingStartFailed
{ "status": "pending" }         // BookingStarted | BookingPending
{ "status": "cancelled" }       // Canceled | CancelPending (или separate is_cancelling flag)
{ "status": "confirmed" }       // BookingFinished + Payed = Done
```

Клиентских статусов должно быть столько, сколько уникальных UI-состояний. Не больше.

**Антипаттерн — boolean flags вместо enum:**

```json
{
  "isNotReady": true,
  "isDraft": false,
  "isSaved": true,
  "hasPrTransferOffers": false
}
```

4 булевых флага создают 16 комбинаций состояний. Большинство комбинаций невалидны, но клиент вынужден обрабатывать все. Что означает `isNotReady: true, isDraft: true`?

**Правильно — enum статус + специфичные флаги с чётким смыслом:**

```json
{
  "status": "draft",              // enum: draft | ready | saved | expired
  "availability": "has_options"   // enum: has_options | no_options | not_applicable
}
```

**Антипаттерн — inline ошибка в теле успешного ответа:**

```json
// HTTP 200 OK
{
  "bundleId": "abc",
  "status": "error",
  "error": "Provider timeout"
}
```

Клиент должен проверять HTTP-статус И поле `error`. Это нарушает контракт HTTP: 200 должен означать успех.

**Правильно — ошибка через HTTP-статус + структурированный error body:**

```json
// HTTP 422 Unprocessable Entity
{
  "error": {
    "code": "PROVIDER_TIMEOUT",
    "message": "Could not fetch prices from provider",
    "details": { "provider": "ratehawk", "retry_after": 5 }
  }
}
```

Исключение: частичные ошибки в bulk-операциях, где часть записей успешна, а часть нет. Тогда inline ошибки в элементах массива — допустимо.

---

### Неструктурированные данные

**Антипаттерн — `object` / `any` в ответе:**

```json
{
  "stats": { ... },          // любой объект
  "lifts": { ... },          // любой объект
  "tripadvisorInfo": { ... } // любой объект
}
```

Эти поля — чёрные ящики. Фронтенд не знает, что внутри, без изучения кода или документации. Типизация в TypeScript невозможна. Нет контракта — нет гарантий.

**Почему это происходит:** данные разные для каждого провайдера, не хочется делать несколько типов. Или данные меняются часто.

**Как правильно:**

Вариант 1 — типизировать union:

```json
{
  "tripadvisor": {
    "rating": 4.5,
    "review_count": 1234,
    "url": "https://tripadvisor.com/..."
  }
}
```

Вариант 2 — если поставщиков много, provider-specific блок + discriminator:

```json
{
  "supplier_data": {
    "provider": "tripadvisor",
    "tripadvisor": { "rating": 4.5, "reviews": 1234 }
  }
}
```

Вариант 3 — если данные реально динамические (конфигурация фичей, A/B), выделить в отдельный эндпоинт с явной документацией что там может быть.

`object` / `any` в ответе API — это техдолг, который платит фронтенд.

---

### Nullability и опциональные поля

**Проблема proto3 defaults:**

В proto3 все поля имеют дефолтное значение (0 для int, "" для string, false для bool). Это означает, что клиент не может отличить "поле не задано" от "поле задано как 0/false/пустая строка".

В JSON-ответе это проявляется так:

```json
// Пользователь не указал телефон, или телефон пустая строка?
{ "phone": "" }

// Скидка 0, или скидки нет?
{ "discount_amount": 0 }

// Питомца нет, или поле не заполнено?
{ "has_pet": false }
```

**Как правильно:**

Используйте `null` для "не задано", отсутствие поля для "не применимо":

```json
// Поле есть, но значение не указано
{ "phone": null }

// Поля нет вообще — значит не применимо в этом контексте
{ }

// Поле есть и заполнено
{ "phone": "+7-999-123-45-67" }
```

В gRPC/Protobuf для nullable значений используйте `optional` или `google.protobuf.StringValue` / `google.protobuf.Int64Value`. Не используйте magic values типа `0` или `""` для "не задано" — это создаёт неявные соглашения.

---

### userId в теле запроса для авторизованных эндпоинтов

**Антипаттерн:**

```json
// POST /orders/payment/confirm
{
  "order_id": "abc",
  "user_id": "xyz",        // ← зачем?
  "payment_intent_id": "pi_..."
}
```

Если эндпоинт требует авторизацию (JWT/cookie), `user_id` уже есть в токене. Передавать его в теле:
1. **Запутывает**: кто sender — тот в токене или тот в body? Что если они разные?
2. **Небезопасно**: клиент может передать чужой userId и получить доступ к чужим данным (если бэкенд не валидирует совпадение с токеном).
3. **Дублирует источник правды**: два места откуда берётся userId — токен и body.

**Правильно:**

`userId` для авторизованных эндпоинтов берётся **только из токена** на бэкенде. В контракте API его нет.

```json
// POST /orders/payment/confirm
// Authorization: Bearer <token>   ← userId здесь
{
  "order_id": "abc",
  "payment_intent_id": "pi_..."
}
```

Единственное исключение — эндпоинты для admin/service-to-service, где действие выполняется от имени другого пользователя. Тогда параметр называется явно: `target_user_id`.

---

### Опечатки и именование полей

Опечатки в API — это навсегда. Сломать контракт сложнее, чем добавить новое поле, поэтому опечатка живёт годами.

**Примеры типичных ошибок:**

| Неверно | Верно | Проблема |
|---|---|---|
| `payedAt` | `paid_at` | "payed" — устаревшая форма, правильно "paid" |
| `contactLasName` | `contact_last_name` | опечатка: пропущена `t` |
| `expiredAt` (для будущего времени) | `expires_at` | прошедшее время для будущей точки |
| `expireAt` (непоследовательно) | `expires_at` | должно быть единообразно |
| `acc` | `accommodation` | аббревиатура без документации |
| `finalTotal` | `total` (+ отдельное `subtotal`) | неочевидное разграничение |

**Правила именования полей JSON:**

```
snake_case для полей — отраслевой стандарт (GitHub, Stripe, Google)

Временные поля:
  - события прошлого: created_at, updated_at, paid_at, cancelled_at
  - точки будущего:   expires_at, due_at, scheduled_at

Булевые поля:
  - is_ / has_ / can_ префикс: is_active, has_pet, can_pay_now
  - не использовать отрицание: is_not_ready → лучше is_ready: false

ID полей:
  - всегда суффикс _id: order_id, user_id, session_id
  - не сокращать: не uid, не oid
```

---

## Чеклист перед выпуском

**URL:**
- [ ] Все сегменты — существительные, не глаголы
- [ ] Коллекции во множественном числе, plural везде консистентно
- [ ] kebab-case для multi-word сегментов
- [ ] ID ресурса в path, не в query
- [ ] Нет суффикса `/list`
- [ ] Нет аббревиатур (кроме общепринятых: `id`, `api`, `url`)
- [ ] Глубина иерархии не больше 3 уровней

**HTTP-методы:**
- [ ] GET только для чтения, не изменяет состояние
- [ ] POST для создания нового ресурса
- [ ] PATCH для частичного обновления
- [ ] PUT только если клиент передаёт полный объект
- [ ] Нет POST там, где семантика явно GET

**Параметры:**
- [ ] Обязательные идентификаторы — в path
- [ ] Фильтры, сортировка, пагинация — в query
- [ ] Данные для создания/изменения — в body

**Коды ответов:**
- [ ] 200 — успех с телом
- [ ] 201 — ресурс создан (с `Location` header)
- [ ] 204 — успех без тела (DELETE, некоторые PATCH)
- [ ] 400 — невалидный запрос клиента
- [ ] 401 — нет аутентификации
- [ ] 403 — нет прав (аутентифицирован, но нельзя)
- [ ] 404 — ресурс не найден
- [ ] 409 — конфликт (ресурс уже существует, concurrent edit)
- [ ] 422 — запрос технически валиден, но бизнес-логика отказывает
- [ ] 500 — ошибка сервера

**Response — структура:**
- [ ] Единый envelope или единое соглашение для всех ответов
- [ ] Коллекции: одно имя поля (data[] или plural noun), никогда не `list`
- [ ] Пагинация есть во всех коллекциях, которые могут вырасти
- [ ] Метаданные (expiry, context) в `meta`, не в корень рядом с данными
- [ ] Нет `object`/`any` полей без задокументированного контракта

**Response — поля:**
- [ ] Все даты в ISO 8601 (`2025-01-15T10:30:00Z`), не unix timestamp
- [ ] Единый стиль дат: `created_at`, `expires_at`, `paid_at` (не `payedAt`, не `expireAt`)
- [ ] Деньги в minor units + currency объект, не float, не аббревиатуры (`accommodation`, не `acc`)
- [ ] Статусы — user-facing, без internal деталей реализации
- [ ] Boolean flags заменены enum где есть 3+ состояния
- [ ] Нет inline ошибок в теле 200-ответа
- [ ] userId не передаётся в body авторизованных эндпоинтов
- [ ] Один стиль именования полей в JSON (snake_case везде)
- [ ] Проверены опечатки во всех именах полей

---

## Interview-ready answer

> **Как спроектировать хороший REST API?**

REST API строится вокруг ресурсов — существительных, а не глаголов. URL `/orders/123` — адрес конкретного заказа. Действие выражается HTTP-методом: GET — прочитать, POST — создать, PATCH — частично обновить, DELETE — удалить.

Три главных правила именования: **plural nouns** для коллекций (`/orders`, не `/order/list`), **kebab-case** (`/contact-info`, не `/contactInfo`), **ID ресурса в path** (`/orders/123`, не `/orders?id=123`).

Частые ошибки: глаголы в URL (`/orders/confirm` → лучше `POST /orders/123/confirmation`), POST вместо GET для read-операций, суффикс `/list` на коллекциях, аббревиатуры в путях.

Особые случаи — state machine transitions — решаются через sub-resource: `POST /payments/{id}/confirmation` — это создание ресурса-подтверждения, не просто вызов метода.

Хороший REST API самодокументируется: клиент, видя `GET /users/123/orders?status=pending`, понимает его без документации.

> **Какие типичные проблемы в дизайне response?**

Три главных: **даты как unix timestamp** вместо ISO 8601 (клиент не знает, секунды или миллисекунды, какой timezone), **деньги как float** вместо minor units + currency (float precision даёт баги в платёжном коде), **internal статусы в API** вместо user-facing (клиент получает `BookingFinishedWithError` и не знает, что показать пользователю).

Остальные: отсутствие пагинации на коллекциях (бомба замедленного действия), непоследовательная структура ответов (где-то `list`, где-то `orders`, где-то голый массив), `object`/`any` поля без схемы (TypeScript-типизация невозможна), userId в теле авторизованных запросов (security smell и дублирование source of truth).
