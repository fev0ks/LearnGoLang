# HTTP Servers in Go

Обзор подходов к написанию HTTP-серверов: от stdlib до фреймворков.

## Материалы

- [01 stdlib net/http](./01-stdlib-net-http.md) — ServeMux Go 1.22+, middleware pattern, когда достаточно stdlib
- [02 chi](./02-chi.md) — идиоматичный минимальный роутер, subrouters, middleware stack
- [03 gin](./03-gin.md) — популярный фреймворк: binding, validation, gin.Context
- [04 echo](./04-echo.md) — альтернатива gin с error-return handlers и другой архитектурой
- [05 fiber](./05-fiber.md) — fasthttp-based: производительность и несовместимость с net/http
- [06 Сравнение и выбор](./06-comparison.md) — таблица решений, gorilla/mux legacy, что важно в production

## Вопросы

- почему для нового сервиса стоит выбрать chi, а не gin
- что потеряешь при переходе на fiber и когда это оправдано
- как написать middleware совместимую со stdlib и chi/gin одновременно
- чем `gin.Context` отличается от `context.Context` и почему это важно
- почему gorilla/mux не стоит выбирать для нового проекта
- как Go 1.22 изменил аргументы в пользу stdlib router
