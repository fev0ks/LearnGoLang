# CORS Middleware Example

Эта заметка разбирает типичный самописный `CORS` middleware для Go.

Ниже идея не в том, чтобы “найти идеальный middleware на все случаи”, а в том, чтобы понимать:
- что в таком коде обычно правильно;
- что в нем опасно;
- где нужен allowlist, а где допустим более общий policy.

## Пример исходной идеи

Типичный middleware выглядит так:

```go
func allowCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

			if reqHdrs := r.Header.Get("Access-Control-Request-Headers"); reqHdrs != "" {
				w.Header().Set("Access-Control-Allow-Headers", reqHdrs)
			} else {
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}

			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		h.ServeHTTP(w, r)
	})
}
```

На первый взгляд это выглядит “почти правильно”, и это действительно рабочая отправная точка.  
Но в таком виде там есть несколько важных security и behavior pitfalls.

## Что означают все эти поля простыми словами

Ниже самый важный момент: многие куски CORS-кода выглядят как набор магических строк, пока не разберешь, что именно они значат.

### `Origin`

Это header, который присылает браузер, чтобы сказать:
- “я пришел со страницы вот этого сайта”.

Например:

```text
Origin: https://app.example.com
```

или локально:

```text
Origin: http://localhost:3000
```

`Origin` — это по сути:
- протокол (`http` или `https`)
- хост
- порт

То есть:
- `https://app.example.com`
- `https://api.example.com`
- `http://localhost:3000`
- `http://localhost:8080`

это все разные origins.

Когда в тексте выше говорится:
- “любой origin, который прислал браузер, я считаю разрешенным”

это означает буквально:
- браузер сказал “я страница с сайта `X`”
- сервер ответил “ок, сайту `X` можно читать мой ответ”

Если сервер так говорит для любого `X`, policy получается слишком широкой.

### `Access-Control-Allow-Origin`

Это ответ сервера браузеру:
- “какому origin я разрешаю читать этот ответ”.

Примеры:

```text
Access-Control-Allow-Origin: https://app.example.com
```

или:

```text
Access-Control-Allow-Origin: *
```

Смысл:
- конкретный origin = разрешаем только этому сайту;
- `*` = разрешаем всем сайтам читать ответ.

Но:
- если нужны cookies/credentials, `*` обычно уже не подходит.

### `Access-Control-Allow-Credentials`

Это ответ сервера:
- “можно ли браузеру отправлять и использовать credentialed request”.

Под credentials обычно имеют в виду:
- cookies;
- browser-managed auth state;
- иногда другие credentialed browser flows.

Если сервер говорит:

```text
Access-Control-Allow-Credentials: true
```

то это уже более чувствительный сценарий.  
Поэтому вместе с этим обычно нужен не `*`, а конкретный allowlist origins.

### `Access-Control-Allow-Methods`

Это ответ сервера:
- “какие HTTP методы я разрешаю для cross-origin запросов”.

Например:

```text
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
```

Простыми словами:
- браузер спрашивает “можно ли мне с другого origin делать `POST` или `DELETE`?”
- сервер отвечает “да, вот эти методы разрешены”.

### `Access-Control-Allow-Headers`

Это ответ сервера:
- “какие request headers браузеру можно отправлять в таком cross-origin запросе”.

Например:

```text
Access-Control-Allow-Headers: Content-Type, Authorization, X-Request-Id
```

Простыми словами:
- frontend хочет отправить `Authorization` или кастомный `X-Request-Id`;
- браузер спрашивает, можно ли;
- сервер отвечает разрешенным списком.

### `Access-Control-Request-Headers`

Это уже не ответ сервера, а вопрос от браузера.

Браузер говорит:
- “я собираюсь отправить вот такие headers, это можно?”

Например:

```text
Access-Control-Request-Headers: authorization, x-request-id
```

Поэтому dangerous часть в исходном middleware такая:
- браузер сам перечислил, что хочет отправить;
- сервер без проверки просто отзеркалил это обратно как “разрешено”.

### `Access-Control-Request-Method`

Это тоже вопрос от браузера во время preflight:
- “я собираюсь сделать `POST` / `DELETE` / `PATCH`, это можно?”

То есть:
- это еще не реальный запрос с бизнес-данными;
- это предварительная проверка правил.

### `Access-Control-Max-Age`

Это ответ сервера:
- “как долго браузер может кэшировать результат preflight-проверки”.

Например:

```text
Access-Control-Max-Age: 600
```

означает:
- примерно 10 минут браузер может не спрашивать заново одно и то же preflight-решение.

Если сделать слишком большим:
- меньше preflight traffic;
- но сложнее быстро поменять policy во время разработки или incident.

### `Vary`

Это уже не чисто CORS-шный header, а подсказка кэшам.

Когда сервер пишет:

```text
Vary: Origin, Access-Control-Request-Method, Access-Control-Request-Headers
```

он говорит:
- “мой ответ зависит от этих входных headers”
- “нельзя кэшировать один CORS-ответ как будто он одинаков для всех origins”

Иначе кэш может сделать неправильную вещь:
- сохранить ответ для одного origin;
- потом отдать его другому origin.

### `OPTIONS`

Это HTTP method, который браузер часто использует для preflight.

Простыми словами:
- браузер не сразу шлет “настоящий” cross-origin запрос;
- сначала он шлет “пробный вопрос”:
  - можно ли с этого origin?
  - можно ли с этим методом?
  - можно ли с этими headers?

Именно поэтому middleware часто отдельно обрабатывает:

```go
if r.Method == http.MethodOptions { ... }
```

Потому что `OPTIONS` здесь — это не бизнес-действие, а проверка правил.

## Что в таком коде уже нормально

### 1. Возвращать конкретный `Origin`, а не `*`, если нужны credentials

Это правильно по смыслу.

Если используются:
- cookies;
- session auth;
- credentialed browser requests,

то `Access-Control-Allow-Origin: *` уже не подходит.

### 2. Отдельно обрабатывать `OPTIONS`

Это тоже правильно:
- preflight запросы не должны уходить в обычную бизнес-логику;
- браузеру нужен быстрый корректный ответ с `Access-Control-*` headers.

### 3. Добавлять `Vary`

Идея верная, потому что CORS-ответ зависит от:
- `Origin`
- `Access-Control-Request-Method`
- `Access-Control-Request-Headers`

И это важно для корректного caching behavior.

## Что в таком коде опасно или слишком доверчиво

### 1. Blind reflection `Origin`

Вот это главный риск:

```go
w.Header().Set("Access-Control-Allow-Origin", origin)
```

Если ты просто отражаешь любой пришедший `Origin`, то по сути говоришь:
- “любой origin, который прислал браузер, я считаю разрешенным”.

Если одновременно включено:

```go
Access-Control-Allow-Credentials: true
```

то это уже очень широкая и потенциально опасная политика.

Практически это значит:
- policy становится “разрешаем всем origins”, а не “разрешаем только наш frontend”.

Правильнее:
- иметь allowlist origins;
- возвращать `Access-Control-Allow-Origin` только если origin реально разрешен.

### 2. Blind reflection `Access-Control-Request-Headers`

Вот это тоже слишком щедро:

```go
if reqHdrs := r.Header.Get("Access-Control-Request-Headers"); reqHdrs != "" {
	w.Header().Set("Access-Control-Allow-Headers", reqHdrs)
}
```

Почему это спорно:
- браузер просит список заголовков;
- сервер просто автоматически говорит “да, все ок”.

Это удобно для совместимости, но security policy получается очень расплывчатой.

Лучше:
- заранее знать разумный allowlist custom headers;
- валидировать запрошенные headers против allowlist.

### 3. `Vary` через `Set`, а не через аккуратное добавление

Технически это может затереть уже существующий `Vary`.

Если раньше middleware или handler уже поставил:
- `Vary: Accept-Encoding`

а потом CORS сделал:

```go
w.Header().Set("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
```

то старое значение пропадет.

Идея правильнее такая:
- `Vary` надо расширять, а не бездумно перезаписывать.

### 4. Отвечать на `OPTIONS` слишком широко

Если middleware возвращает `204` на любой `OPTIONS`, даже без проверки:
- что origin разрешен;
- что method разрешен;
- что headers допустимы,

то preflight policy становится слишком permissive.

Лучше:
- сначала проверить origin;
- потом method;
- потом requested headers;
- и только после этого отвечать успешным preflight.

## Более безопасная practical версия

Ниже пример не “идеального фреймворка”, а просто более здравой версии.

```go
var allowedOrigins = map[string]struct{}{
	"https://app.example.com": {},
	"http://localhost:3000":   {},
}

var allowedHeaders = map[string]struct{}{
	"content-type":  {},
	"authorization": {},
	"x-request-id":  {},
}

var allowedMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodOptions: {},
}

func allowCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		if _, ok := allowedOrigins[origin]; !ok {
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		addVary(w.Header(), "Origin")
		addVary(w.Header(), "Access-Control-Request-Method")
		addVary(w.Header(), "Access-Control-Request-Headers")

		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-Id")
		w.Header().Set("Access-Control-Max-Age", "600")

		if r.Method == http.MethodOptions {
			reqMethod := r.Header.Get("Access-Control-Request-Method")
			if _, ok := allowedMethods[reqMethod]; !ok {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
```

## Почему эта версия лучше

### Origin allowlist

Теперь не любой `Origin` получает доступ, а только явно разрешенный.

Это уже настоящая policy, а не reflection.

### Явный набор методов и headers

Теперь сервер сам диктует, что он готов принимать.

Это лучше, чем blindly mirror browser request.

### Более разумный `Max-Age`

`86400` может работать, но на практике:
- браузеры все равно могут капать его по-своему;
- при active development слишком длинный cache иногда мешает.

`600` или `3600` часто pragmatic default.

### Аккуратная работа с `Vary`

Нужно не затирать его, а добавлять.

Например helper:

```go
func addVary(h http.Header, value string) {
	current := h.Values("Vary")
	for _, line := range current {
		for _, part := range strings.Split(line, ",") {
			if strings.TrimSpace(part) == value {
				return
			}
		}
	}
	h.Add("Vary", value)
}
```

## Когда такой middleware вообще нормален

Нормально держать CORS middleware в приложении, если:
- сервис один;
- browser clients понятны;
- policy несложная;
- разные route могут требовать разных правил.

Если же:
- много сервисов;
- одна и та же browser-facing policy повторяется;
- есть единый ingress/gateway,

то часто лучше вынести CORS на gateway/proxy уровень.

## Когда самописный middleware уже не лучший путь

Если появляются:
- сложные allowlists;
- разные правила по routes;
- много origins per environment;
- credentials + cookies + admin/front separation,

то лучше:
- либо использовать battle-tested middleware/library;
- либо централизовать policy в gateway/ingress.

Иначе есть риск, что самописная логика:
- будет inconsistent между сервисами;
- случайно откроет слишком широкую политику;
- сломает preflight в edge cases.

## Дополнительные CORS-related и browser security headers

Ниже параметры, которые не всегда нужны в простом API, но часто встречаются рядом с `CORS`.

Важно различать:
- часть headers — это именно CORS response headers;
- часть — browser security / fetch metadata headers, которые помогают принимать решения рядом с CORS.

### `Access-Control-Expose-Headers`

Кто отправляет:
- сервер.

Что означает:
- какие response headers браузер разрешит читать JavaScript-коду.

Пример:

```text
Access-Control-Expose-Headers: X-Request-Id, X-RateLimit-Remaining, X-Total-Count
```

Зачем это нужно:
- backend вернул header;
- но браузер не всегда даст frontend-коду его прочитать;
- если header кастомный и нужен JS-коду, его надо явно exposed.

Типичные значения:
- `X-Request-Id`
- `X-Correlation-Id`
- `X-RateLimit-Limit`
- `X-RateLimit-Remaining`
- `X-RateLimit-Reset`
- `X-Total-Count`
- `Content-Disposition`

Практический пример:
- API возвращает pagination через `X-Total-Count`;
- frontend должен показать “найдено 120 элементов”;
- без `Access-Control-Expose-Headers: X-Total-Count` JS может не увидеть этот header.

Когда добавлять:
- когда frontend реально должен читать custom response headers.

Когда не добавлять:
- если header нужен только инфраструктуре или backend-to-backend flow.

### `Access-Control-Allow-Private-Network`

Кто отправляет:
- сервер в ответ на private network preflight.

Что означает:
- сервер разрешает браузеру делать запрос из менее приватной сети в более приватную.

Пример:

```text
Access-Control-Allow-Private-Network: true
```

Где это всплывает:
- public сайт пытается обратиться к `http://192.168.0.1`;
- web page обращается к локальному или private network device;
- admin UI в браузере ходит к сервису внутри private network.

Зачем это нужно:
- браузеры постепенно ужесточают доступ из public origins к private network resources;
- это защита от сценариев, где вредный сайт пытается дергать роутер, принтер, local admin panel или private service пользователя.

Типичные значения:
- `true`

Когда добавлять:
- только если ты осознанно поддерживаешь browser access к private network resource.

Когда не добавлять:
- в обычном public API это чаще всего не нужно.

### `Timing-Allow-Origin`

Кто отправляет:
- сервер.

Что означает:
- каким origins браузер разрешит видеть detailed timing information через Resource Timing API.

Пример:

```text
Timing-Allow-Origin: https://app.example.com
```

или:

```text
Timing-Allow-Origin: *
```

Зачем это нужно:
- frontend observability;
- real user monitoring;
- измерение network timing до API/CDN;
- понимание DNS/TLS/TTFB timing в браузере.

Какие могут быть значения:
- конкретный origin: `https://app.example.com`
- список origins, если инфраструктура поддерживает такой формат;
- `*`, если timing details можно раскрывать всем.

Когда добавлять:
- когда frontend performance monitoring должен видеть подробные timing breakdowns.

Когда осторожно:
- если timing information может раскрывать sensitive side-channel детали.

### `Sec-Fetch-Site`

Кто отправляет:
- браузер.

Что означает:
- откуда относительно target site пришел запрос.

Примеры:

```text
Sec-Fetch-Site: same-origin
Sec-Fetch-Site: same-site
Sec-Fetch-Site: cross-site
Sec-Fetch-Site: none
```

Простыми словами:
- `same-origin` — тот же origin;
- `same-site` — тот же site, но origin может отличаться;
- `cross-site` — запрос пришел с другого сайта;
- `none` — navigation/user-initiated case, например ввод URL в адресной строке.

Зачем это нужно:
- можно строить defense-in-depth policy;
- например отклонять подозрительные `cross-site` state-changing запросы;
- полезно рядом с CSRF defense.

Важно:
- это не CORS response header;
- это request signal от браузера.

### `Sec-Fetch-Mode`

Кто отправляет:
- браузер.

Что означает:
- в каком режиме браузер делает запрос.

Примеры:

```text
Sec-Fetch-Mode: cors
Sec-Fetch-Mode: no-cors
Sec-Fetch-Mode: navigate
Sec-Fetch-Mode: same-origin
```

Простыми словами:
- `cors` — обычный cross-origin fetch с CORS policy;
- `no-cors` — ограниченный режим, где JS не получает нормальный readable response;
- `navigate` — переход страницы, а не API fetch;
- `same-origin` — запрос только в рамках same-origin.

Зачем это нужно:
- отличать browser navigation от API fetch;
- строить дополнительные правила для suspicious traffic;
- усиливать CSRF/browser boundary defense.

### `Sec-Fetch-Dest`

Кто отправляет:
- браузер.

Что означает:
- для чего браузер хочет использовать ответ.

Примеры:

```text
Sec-Fetch-Dest: document
Sec-Fetch-Dest: image
Sec-Fetch-Dest: script
Sec-Fetch-Dest: style
Sec-Fetch-Dest: empty
```

Простыми словами:
- `document` — страница;
- `image` — картинка;
- `script` — JS;
- `style` — CSS;
- `empty` — обычно `fetch`/XHR/API request.

Зачем это нужно:
- можно понять, похож ли запрос на нормальный use case;
- API endpoint чаще ожидает `empty`, а не `image` или `script`;
- это может помочь отфильтровать странные browser-driven запросы.

### `Sec-Fetch-User`

Кто отправляет:
- браузер, обычно для navigation requests.

Что означает:
- был ли запрос вызван пользовательским действием.

Пример:

```text
Sec-Fetch-User: ?1
```

Зачем это нужно:
- можно отличить некоторые user-initiated navigations от background запросов.

Для обычного API это редко главный сигнал, но в browser security policy может быть полезен.

### `Cross-Origin-Resource-Policy`

Кто отправляет:
- сервер.

Что означает:
- кто может встраивать ресурс в другие страницы.

Примеры:

```text
Cross-Origin-Resource-Policy: same-origin
Cross-Origin-Resource-Policy: same-site
Cross-Origin-Resource-Policy: cross-origin
```

Зачем это нужно:
- защищать ресурсы от нежелательного cross-origin embedding;
- например изображения, scripts, документы, приватные ресурсы.

Чем отличается от CORS:
- CORS управляет чтением response из JS;
- CORP управляет тем, можно ли resource быть загруженным/использованным cross-origin.

### `Cross-Origin-Opener-Policy`

Кто отправляет:
- сервер.

Что означает:
- как страница изолируется от других browsing contexts/windows.

Пример:

```text
Cross-Origin-Opener-Policy: same-origin
```

Зачем это нужно:
- усиление isolation;
- защита от некоторых cross-origin window interaction scenarios;
- часть набора browser isolation headers.

Для обычного JSON API это не первый header, но для web pages/admin UI может быть важным.

### `Cross-Origin-Embedder-Policy`

Кто отправляет:
- сервер.

Что означает:
- какие cross-origin resources страница может встраивать.

Пример:

```text
Cross-Origin-Embedder-Policy: require-corp
```

Зачем это нужно:
- stronger browser isolation;
- cross-origin isolation use cases;
- некоторые advanced browser APIs требуют COOP/COEP.

Для backend API встречается реже, но полезно знать, что это соседняя browser security тема.

## Какие headers обычно нужны обычному API

Для стандартного browser frontend + JSON API чаще всего достаточно:
- `Access-Control-Allow-Origin`
- `Access-Control-Allow-Credentials`, если есть cookies/session;
- `Access-Control-Allow-Methods`
- `Access-Control-Allow-Headers`
- `Access-Control-Expose-Headers`, если frontend читает custom response headers;
- `Access-Control-Max-Age`
- корректный `Vary`
- обработка `OPTIONS`

Остальное:
- advanced browser security;
- private network access;
- frontend observability;
- hardening для web pages.

## Practical Rule

Если коротко:
- отражать любой `Origin` — плохой default;
- credentials почти всегда требуют явный allowlist;
- preflight надо валидировать, а не просто “всех пускать”;
- `CORS` должен быть policy, а не механическое отражение browser headers.
