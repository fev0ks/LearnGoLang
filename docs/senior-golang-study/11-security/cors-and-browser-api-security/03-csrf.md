# CSRF

CSRF (Cross-Site Request Forgery) — атака при которой браузер жертвы отправляет запрос на ваш сервер от имени аутентифицированного пользователя без его ведома.

## Содержание

- [Как работает атака](#как-работает-атака)
- [Когда CSRF актуален](#когда-csrf-актуален)
- [Методы защиты](#методы-защиты)
- [SameSite cookie](#samesite-cookie)
- [Double Submit Cookie](#double-submit-cookie)
- [Synchronizer Token (CSRF-токен)](#synchronizer-token-csrf-токен)
- [Origin / Referer проверка](#origin--referer-проверка)
- [Какой метод выбрать](#какой-метод-выбрать)

---

## Как работает атака

```
1. Пользователь залогинен на bank.example.com
   → браузер хранит session cookie

2. Пользователь заходит на evil.example.com
   → страница содержит скрытую форму или img-тег:
   
   <form action="https://bank.example.com/transfer" method="POST">
     <input name="to" value="attacker">
     <input name="amount" value="10000">
   </form>
   <script>document.forms[0].submit()</script>

3. Браузер автоматически отправляет запрос на bank.example.com
   → cookie сессии прикрепляется автоматически
   → сервер видит аутентифицированный запрос и выполняет перевод
```

Ключевой момент: браузер сам прикрепляет cookie к cross-site запросам — это не баг браузера, это его штатное поведение.

---

## Когда CSRF актуален

**CSRF опасен, если:**
- аутентификация через cookie (session cookie, JWT в cookie)
- сервер обслуживает браузерные клиенты
- есть мутирующие операции (POST, PUT, DELETE, PATCH)

**CSRF НЕ актуален, если:**
- аутентификация только через `Authorization: Bearer <token>` в заголовке — браузер не прикрепляет заголовки автоматически к cross-site запросам
- pure API без браузерных клиентов
- только GET-запросы (но GET не должен мутировать состояние)

---

## Методы защиты

| Метод | Надёжность | Сложность | Подходит для |
|---|---|---|---|
| `SameSite=Strict/Lax` cookie | высокая | низкая | большинство случаев |
| Double Submit Cookie | высокая | средняя | SPA без серверного состояния |
| Synchronizer Token | очень высокая | высокая | традиционные web-приложения |
| Origin/Referer check | средняя | низкая | дополнительный слой |

---

## SameSite cookie

Самый простой и современный способ. Браузер не отправляет cookie в cross-site запросах.

```go
http.SetCookie(w, &http.Cookie{
    Name:     "session",
    Value:    sessionToken,
    HttpOnly: true,
    Secure:   true,
    SameSite: http.SameSiteLaxMode,   // или StrictMode
    Path:     "/",
})
```

**`SameSite=Strict`** — cookie не отправляется вообще ни в каких cross-site запросах, включая переходы по ссылкам. Хорошо для банков и критичных приложений, но ломает UX: переход с email-ссылки не будет аутентифицированным.

**`SameSite=Lax`** — cookie отправляется при навигации (переход по ссылке) но не при фоновых запросах (форм POST, img, fetch). Рекомендуемый дефолт для большинства приложений.

**`SameSite=None`** — cookie отправляется везде, но требует `Secure=true`. Нужен для встроенных виджетов и OAuth redirect flows.

```go
// Middleware: установить SameSite для всех ответов с cookie
func SecureCookieMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Нельзя перехватить SetCookie после факта, поэтому
        // договориться в команде: всегда использовать хелпер
        next.ServeHTTP(w, r)
    })
}

func setSessionCookie(w http.ResponseWriter, token string) {
    http.SetCookie(w, &http.Cookie{
        Name:     "session_id",
        Value:    token,
        Path:     "/",
        HttpOnly: true,       // не доступен через JS
        Secure:   true,       // только HTTPS
        SameSite: http.SameSiteLaxMode,
        MaxAge:   86400,
    })
}
```

**Ограничение:** SameSite работает только в современных браузерах. Для критичных приложений комбинируют с дополнительным методом.

---

## Double Submit Cookie

Сервер устанавливает случайный CSRF-токен в cookie, клиент дублирует его в заголовке или теле запроса. Сервер проверяет совпадение.

Атакующий с другого домена не может прочитать cookie (Same-Origin Policy), значит не может подделать значение в заголовке.

```go
// Генерация CSRF-токена
func generateCSRFToken() (string, error) {
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return base64.URLEncoding.EncodeToString(b), nil
}

// Middleware: установить CSRF-cookie и проверять при мутирующих запросах
func CSRFMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // GET/HEAD/OPTIONS — только установить cookie если нет
        if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
            if _, err := r.Cookie("csrf_token"); err != nil {
                token, err := generateCSRFToken()
                if err != nil {
                    http.Error(w, "internal error", http.StatusInternalServerError)
                    return
                }
                http.SetCookie(w, &http.Cookie{
                    Name:     "csrf_token",
                    Value:    token,
                    Path:     "/",
                    Secure:   true,
                    SameSite: http.SameSiteStrictMode,
                    // HttpOnly: false — JS должен читать этот cookie!
                })
            }
            next.ServeHTTP(w, r)
            return
        }

        // POST/PUT/PATCH/DELETE — проверить токен
        cookie, err := r.Cookie("csrf_token")
        if err != nil {
            http.Error(w, "missing csrf cookie", http.StatusForbidden)
            return
        }

        // Клиент дублирует значение в заголовке X-CSRF-Token
        header := r.Header.Get("X-CSRF-Token")
        if header == "" {
            // Или в теле формы
            header = r.FormValue("csrf_token")
        }

        if !hmac.Equal([]byte(cookie.Value), []byte(header)) {
            http.Error(w, "csrf validation failed", http.StatusForbidden)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

**Клиент (SPA):**

```javascript
// Читаем CSRF token из cookie
function getCookie(name) {
    const value = `; ${document.cookie}`;
    const parts = value.split(`; ${name}=`);
    if (parts.length === 2) return parts.pop().split(';').shift();
}

// Добавляем в каждый мутирующий запрос
fetch('/api/transfer', {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCookie('csrf_token'),  // дублируем cookie в заголовок
    },
    body: JSON.stringify({ to: 'user-2', amount: 100 }),
});
```

---

## Synchronizer Token (CSRF-токен)

Сервер генерирует CSRF-токен, привязывает его к сессии и вставляет в HTML-форму. При submit проверяет что токен из формы совпадает с токеном сессии.

```go
type SessionStore interface {
    Get(r *http.Request) (*Session, error)
    Save(w http.ResponseWriter, r *http.Request, s *Session) error
}

type Session struct {
    UserID    string
    CSRFToken string
}

// В хэндлере который рендерит форму
func (h *Handler) ShowTransferForm(w http.ResponseWriter, r *http.Request) {
    session, _ := h.sessions.Get(r)

    // Генерировать токен если нет
    if session.CSRFToken == "" {
        token, _ := generateCSRFToken()
        session.CSRFToken = token
        h.sessions.Save(w, r, session)
    }

    // Вставить в шаблон
    tmpl.Execute(w, map[string]any{
        "CSRFToken": session.CSRFToken,
    })
}
```

```html
<form action="/transfer" method="POST">
    <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
    <input type="text" name="to">
    <input type="number" name="amount">
    <button type="submit">Перевести</button>
</form>
```

```go
// В хэндлере обработки POST
func (h *Handler) HandleTransfer(w http.ResponseWriter, r *http.Request) {
    session, err := h.sessions.Get(r)
    if err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    formToken := r.FormValue("csrf_token")
    if !hmac.Equal([]byte(session.CSRFToken), []byte(formToken)) {
        http.Error(w, "invalid csrf token", http.StatusForbidden)
        return
    }

    // Ротировать токен после использования
    newToken, _ := generateCSRFToken()
    session.CSRFToken = newToken
    h.sessions.Save(w, r, session)

    // Обработать перевод...
}
```

**Важно:** сравнивать токены через `hmac.Equal` (constant-time), не через `==` — чтобы избежать timing attack.

---

## Origin / Referer проверка

Дополнительный слой поверх основного метода. Проверить что запрос пришёл с ожидаемого origin.

```go
func CheckOrigin(allowedOrigins []string) func(http.Handler) http.Handler {
    allowed := make(map[string]struct{}, len(allowedOrigins))
    for _, o := range allowedOrigins {
        allowed[o] = struct{}{}
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if r.Method == http.MethodGet || r.Method == http.MethodOptions {
                next.ServeHTTP(w, r)
                return
            }

            origin := r.Header.Get("Origin")
            if origin == "" {
                // Браузеры всегда шлют Origin для cross-origin запросов.
                // Отсутствие Origin — либо same-origin, либо не браузер.
                // Для большинства API это OK, но для строгих форм можно отклонить.
                next.ServeHTTP(w, r)
                return
            }

            if _, ok := allowed[origin]; !ok {
                http.Error(w, "forbidden origin", http.StatusForbidden)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

**Ограничение:** Origin/Referer можно убрать через proxy, браузерные расширения или некоторые конфигурации. Не полагаться на это как на единственную защиту.

---

## Какой метод выбрать

**SPA на том же домене + cookie auth:**
```
SameSite=Lax + Double Submit Cookie
```

**Server-side rendered HTML (шаблоны):**
```
SameSite=Lax + Synchronizer Token (горутинен, стандарт)
```

**API только с Bearer token в заголовке:**
```
Ничего не нужно — Authorization header не отправляется автоматически
```

**Максимальная защита (банки, критичные формы):**
```
SameSite=Strict + Synchronizer Token + Origin check
```

**Библиотека для Go:** [gorilla/csrf](https://github.com/gorilla/csrf) реализует Synchronizer Token Pattern с HMAC-подписью токена и готовым middleware.

```go
import "github.com/gorilla/csrf"

csrfMiddleware := csrf.Protect(
    []byte("32-byte-auth-key"),
    csrf.Secure(true),
    csrf.SameSite(csrf.SameSiteLaxMode),
)

mux := http.NewServeMux()
// ...
http.ListenAndServe(":8080", csrfMiddleware(mux))

// В шаблоне: {{ .CSRFField }} вставляет скрытый input
// В хэндлере: csrf.Token(r) возвращает токен для JS/API
```
