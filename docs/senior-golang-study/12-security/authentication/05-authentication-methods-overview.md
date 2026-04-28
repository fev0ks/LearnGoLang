# Методы аутентификации: сравнение

Аутентификация — это "кто ты?". Авторизация — "что тебе можно?". Сначала нужно понять первое, потом решать второе.

## Содержание

- [Три основных метода](#три-основных-метода)
- [Opaque sessions](#opaque-sessions)
- [JWT (JSON Web Token)](#jwt-json-web-token)
- [API keys](#api-keys)
- [Сравнительная таблица](#сравнительная-таблица)
- [Когда что выбирать](#когда-что-выбирать)
- [Многофакторная аутентификация](#многофакторная-аутентификация)

---

## Три основных метода

```
Opaque session token   — random ID, сервер хранит состояние в БД
JWT                    — self-contained токен, состояние закодировано внутри
API key                — долгоживущий секрет для machine-to-machine
```

---

## Opaque sessions

Сервер выдаёт случайный токен (session ID). Всё состояние хранится в БД. По токену — lookup в хранилище.

```
Browser                    Server                    DB
  │                          │                        │
  │  POST /login             │                        │
  │─────────────────────────▶│                        │
  │                          │ INSERT session(token)  │
  │                          │───────────────────────▶│
  │  Set-Cookie: sid=<token> │                        │
  │◀─────────────────────────│                        │
  │                          │                        │
  │  GET /profile            │                        │
  │  Cookie: sid=<token>     │                        │
  │─────────────────────────▶│                        │
  │                          │ SELECT WHERE token=... │
  │                          │───────────────────────▶│
  │                          │◀───────────────────────│
  │  200 OK                  │                        │
  │◀─────────────────────────│                        │
```

**Ревокация:** мгновенная — удалить строку из БД.  
**Состояние:** на сервере — можно хранить что угодно (роли, флаги, данные).  
**Стоимость:** каждый запрос = SELECT в БД (или Redis lookup).

Подробно: [02-sessions-and-session-security.md](./02-sessions-and-session-security.md)

---

## JWT (JSON Web Token)

Сервер выдаёт подписанный токен. Всё состояние закодировано внутри. Сервер проверяет подпись — без обращения к БД.

```
Browser                    Server
  │                          │
  │  POST /login             │
  │─────────────────────────▶│
  │  { access_token, ...}    │ (подпись проверена — нет запроса в БД)
  │◀─────────────────────────│
  │                          │
  │  GET /profile            │
  │  Authorization: Bearer   │
  │─────────────────────────▶│
  │                          │ verify signature → decode claims
  │  200 OK                  │ (нет запроса в БД)
  │◀─────────────────────────│
```

**Ревокация:** сложная — токен валиден до истечения `exp`. Нужен blacklist или короткий TTL.  
**Состояние:** в самом токене (claims) — легко горизонтально масштабировать.  
**Стоимость:** только crypto verify — без I/O на каждый запрос.

Подробно: [06-jwt.md](./06-jwt.md)

---

## API keys

Долгоживущий секрет для machine-to-machine доступа. Разработчик или сервис получает ключ один раз и использует в каждом запросе.

```
HTTP заголовок:
Authorization: Bearer sk-live-abc123xyz...

Или кастомный заголовок:
X-API-Key: sk-live-abc123xyz...
```

**Структура ключа** — читаемый префикс + случайная часть:
```
sk-live-abc123xyz...     ← production key
sk-test-abc123xyz...     ← test key
```
Префикс помогает найти утечку (grep в логах, git history scanner).

**Хранение:** так же как пароли — только хеш в БД:

```go
// При выпуске ключа — отдать пользователю plaintext, сохранить хеш
rawKey := "sk-live-" + generateRandom(32)
hash := sha256Hex(rawKey)
db.Exec("INSERT INTO api_keys (key_hash, user_id, name) VALUES ($1, $2, $3)",
    hash, userID, "My App")
// rawKey показать ОДИН РАЗ, потом недоступен

// При верификации
hash := sha256Hex(r.Header.Get("X-API-Key"))
key, err := db.QueryRow("SELECT user_id, scopes FROM api_keys WHERE key_hash = $1", hash)
```

**Ревокация:** удалить строку из БД.  
**Ротация:** выпустить новый ключ, дать grace period, удалить старый.  
**Scopes:** ключ может иметь ограниченные права (`read:users`, не `write:users`).

---

## Сравнительная таблица

| | Opaque session | JWT | API key |
|---|---|---|---|
| Хранение на сервере | да (БД/Redis) | нет | да (хеш в БД) |
| Ревокация | мгновенная | сложная (blacklist) | мгновенная |
| Масштабирование | I/O на запрос | только crypto | I/O на запрос |
| Для браузеров | да (cookie) | да (header/storage) | обычно нет |
| Для сервисов | редко | да | да |
| Видно состояние | нет | да (декодировать) | нет |
| Смена данных | немедленно | до истечения токена | немедленно |

**"Видно состояние"** — JWT можно декодировать без ключа (base64). Не класть туда sensitive данные.

---

## Когда что выбирать

**Opaque sessions:**
- браузерный клиент с cookie
- нужна мгновенная ревокация (logout должен работать сразу)
- stateful сервис, одна БД — storage не проблема
- E14: именно этот выбор для identity service

**JWT:**
- microservices — gateway верифицирует и пробрасывает claims внутренним сервисам
- stateless горизонтальное масштабирование без shared storage
- mobile/SPA клиенты без cookie
- короткоживущие access tokens (15 мин) + opaque refresh token

**API keys:**
- developer API (Stripe, GitHub, OpenAI)
- server-to-server (CI/CD, webhooks, integrations)
- долгоживущий доступ с явным управлением

**Гибридный паттерн (распространён в продакшне):**
```
Браузер ←→ Gateway: opaque session cookie
Gateway ←→ Сервисы: JWT (short-lived, signed by gateway)
CI/CD, webhooks: API keys
```

Внешние клиенты не видят JWT. Внутри кластера JWT позволяет каждому сервису верифицировать identity без обращения к БД.

---

## Многофакторная аутентификация

MFA добавляет второй фактор поверх пароля. Три категории:

| Категория | Примеры | Стойкость |
|---|---|---|
| Что знаешь | пароль, PIN | ← слабее |
| Что имеешь | TOTP (Authenticator app), SMS, hardware key | средняя |
| Кто ты | биометрия (Touch ID, Face ID) | ← сильнее |

**TOTP (Time-based One-Time Password)** — самый распространённый второй фактор:

```go
import "github.com/pquerna/otp/totp"

// При регистрации MFA: сгенерировать секрет
key, err := totp.Generate(totp.GenerateOpts{
    Issuer:      "MyApp",
    AccountName: user.Email,
})
// Показать QR-код пользователю (key.URL())
// Сохранить key.Secret() зашифрованным в БД

// При логине: проверить 6-значный код
valid := totp.Validate(code, user.TOTPSecret)
if !valid {
    return ErrInvalidMFACode
}
```

**Порядок факторов:** пароль → MFA → сессия. Не выдавать сессию до прохождения всех факторов.

**Recovery codes:** при потере устройства. Генерировать 8-10 одноразовых кодов при включении MFA, хранить хеши (как пароли), каждый код использовать только один раз.
