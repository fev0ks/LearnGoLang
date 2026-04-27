# Authentication

Практические тонкости реализации аутентификации в backend-сервисах: безопасное хранение паролей, управление сессиями, OAuth 2.0 / OIDC и аудит-логирование.

## Как читать

1. Начать с паролей — это основа любой password-based auth.
2. Разобраться с сессиями — как выпускать, хранить и ревокировать безопасно.
3. OAuth и OIDC — для интеграции с Google/Apple/GitHub.
4. Аудит-лог — обязательный элемент production-готовой auth.

## Материалы

- [01. Password Hashing](./01-password-hashing.md) — bcrypt vs argon2id, cost, 72-байт лимит, timing-safe compare, прозрачный апгрейд
- [02. Sessions и безопасность сессий](./02-sessions-and-session-security.md) — cookie атрибуты, хранение хеша токена, session fixation, idle vs absolute timeout, ревокация
- [03. OAuth 2.0 и OIDC](./03-oauth2-and-oidc.md) — Authorization Code + PKCE, state, nonce, верификация ID Token, account linking
- [04. Auth Audit Logging](./04-auth-audit-logging.md) — stable fields, что логировать и что нельзя, email hashing, SQL для анализа

## Что важно уметь объяснить

- почему bcrypt/argon2id, а не SHA-256 для паролей;
- зачем хранить только хеш session token, а не сам токен;
- что такое session fixation и как ротация сессии защищает от неё;
- чем `state` в OAuth отличается от CSRF-токена (решает ту же задачу, но для OAuth redirect);
- зачем `nonce` в OIDC и от какой атаки он защищает;
- почему нельзя использовать `email` как primary key для OAuth identity;
- что должно быть в аудит-логе и чего никогда не должно быть.
