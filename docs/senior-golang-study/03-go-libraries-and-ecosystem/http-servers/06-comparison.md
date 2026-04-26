# Сравнение и выбор HTTP-фреймворка

## Таблица сравнения

| | stdlib (1.22+) | chi | gin | echo | fiber |
|---|---|---|---|---|---|
| net/http совместимость | да (это и есть net/http) | да | нет | нет | нет |
| Path params | да (`{id}`) | да (`:id`) | да (`:id`) | да (`:id`) | да (`:id`) |
| Handler signature | `func(w, r)` | `func(w, r)` | `func(*gin.Context)` | `func(echo.Context) error` | `func(*fiber.Ctx) error` |
| Встроенный binding | нет | нет | да (JSON/form/query) | да (JSON/form/query) | да (BodyParser) |
| Встроенная validation | нет | нет | да (go-playground) | нет (нужно регистрировать) | нет |
| Middleware из коробки | нет | да (chi/middleware) | да (Logger, Recovery) | да (полный набор) | да (полный набор) |
| Subrouters | нет | да (Route/Mount) | да (Group) | да (Group) | да (Group) |
| Centralized error handling | нет | нет | нет | да (HTTPErrorHandler) | да (ErrorHandler) |
| Зависимости | 0 | 0 (только stdlib) | ~10 | ~5 | ~15 |
| Движок | net/http | net/http | httprouter | net/http | fasthttp |
| Производительность | baseline | ~baseline | +10-20% | ~baseline | +300-500% |

---

## gorilla/mux: почему не выбирать сейчас

`gorilla/mux` — исторически один из самых популярных Go-роутеров. Встречается в огромном количестве production-кода.

**Проблема**: gorilla/mux перешёл в **maintenance mode** в 2022 году. Активной разработки нет. Баги фиксируются, но новые фичи не добавляются.

Что это означает на практике:
- В существующем коде — нормально, не трогай то, что работает
- Для нового проекта — выбирай chi (схожая философия, активно развивается)
- Для миграции — chi — drop-in replacement: `gorilla/mux` и `chi` оба используют `net/http` совместимые handlers

```go
// gorilla/mux (legacy)
r := mux.NewRouter()
r.HandleFunc("/users/{id}", getUser).Methods("GET")
id := mux.Vars(r)["id"]

// chi (идиоматичная замена)
r := chi.NewRouter()
r.Get("/users/{id}", getUser)
id := chi.URLParam(r, "id")
```

Миграция gorilla → chi обычно занимает несколько часов на типичный сервис.

---

## Как выбирать

```
Новый сервис?
│
├─ Нужен минимум зависимостей, нет сложного binding?
│   └─ < 20 эндпоинтов, простая маршрутизация → stdlib net/http 1.22+
│
├─ Нужен нормальный роутер без lock-in на фреймворк?
│   └─ chi
│       ✓ совместим с net/http
│       ✓ 0 внешних зависимостей
│       ✓ subrouters, middleware composable
│
├─ Нужен быстрый старт с батарейками (binding, validation)?
│   ├─ gin   — команда знает, нет предпочтений по error handling
│   └─ echo  — хочется `return error` паттерн
│
└─ Измерен performance bottleneck в HTTP-слое?
    └─ fiber  — только если реально нужно и принята несовместимость с net/http
```

---

## Что реально важно в production

Benchmark throughput (req/sec) — наименее важная метрика при выборе фреймворка. Важнее:

**Ecosystem lock-in**
- gin/echo/fiber — нельзя напрямую использовать net/http middleware
- chi/stdlib — любая net/http библиотека работает без адаптеров
- OpenTelemetry, oauth2, prometheus — всё это net/http

**Debugging и observability**
- Стандартный pprof endpoint: `http.Handle("/debug/pprof/", ...)` — работает с chi/stdlib, с gin/echo/fiber нужны адаптеры или замены
- Stack traces и error context — одинаково

**Найм и онбординг**
- gin знает большинство Go-разработчиков
- stdlib + chi легко объяснить за 5 минут любому Go-разработчику
- fiber — нишевый, найти людей с опытом сложнее

**Тестируемость**
- chi/stdlib: `httptest.NewRecorder()` + `httptest.NewRequest()` — стандартно
- gin: `gin.TestMode` + `httptest` — работает, но нужна инициализация gin контекста
- echo: похоже на gin

---

## Что спрашивают на интервью

**"Какой HTTP-фреймворк ты бы выбрал для нового Go-сервиса и почему?"**

Ответ показывает понимание trade-offs, а не знание синтаксиса. Хороший ответ содержит:

1. Зависит от контекста — размер команды, требования к производительности, нужен ли binding
2. Для большинства сервисов: chi (совместим с net/http, минимум зависимостей, активно поддерживается)
3. Если нужен binding из коробки и команда знает gin — gin нормальный выбор, но нужно понимать что теряешь net/http совместимость
4. stdlib достаточен для внутренних сервисов с Go 1.22+
5. fiber — только если реально измерен performance bottleneck и принята несовместимость

**"Почему gorilla/mux не стоит выбирать для нового проекта?"**
- Maintenance mode с 2022 — нет активного развития
- chi — идиоматичная замена с теми же принципами

**"Что потеряешь при переходе с chi на gin?"**
- net/http middleware несовместимы — нужна замена или адаптеры
- context.WithValue → gin.Context.Set/Get
- Стандартный httptest flow усложняется

**"Почему fiber — контроверсный выбор?"**
- fasthttp принципиально несовместим с net/http экосистемой
- OpenTelemetry, prometheus, oauth2 — всё требует адаптеров или замен
- fiber.Ctx переиспользуется — легко data race при передаче в goroutine
- Большинству сервисов не нужна эта производительность

---

## Рекомендация по умолчанию

```
Internal service, simple API    → stdlib net/http 1.22+
Production service, new project → chi
Team knows gin, external API    → gin (knowing trade-offs)
Error-return style preference   → echo
Measured HTTP perf bottleneck   → fiber (with awareness)
Legacy codebase                 → keep gorilla/mux, migrate when touching code
```
