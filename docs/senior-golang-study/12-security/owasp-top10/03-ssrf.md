# SSRF (Server-Side Request Forgery)

SSRF — атака, при которой атакующий заставляет твой сервер делать HTTP-запросы туда, куда сам атакующий с интернета достучаться не может: внутренние сервисы, метаданные облака, локальные файлы, БД администраторов.

OWASP Top 10 #10 в 2021 году. Один из самых опасных классов в облачных средах — именно через SSRF происходили крупнейшие утечки облачных provider'ов.

## Содержание

- [Простая аналогия](#простая-аналогия)
- [Как выглядит уязвимость](#как-выглядит-уязвимость)
- [Куда атакующий хочет попасть](#куда-атакующий-хочет-попасть)
- [Capital One — самый известный SSRF инцидент](#capital-one-самый-известный-ssrf-инцидент)
- [Защита: allowlist URL](#защита-allowlist-url)
- [Защита: блокировка приватных диапазонов IP](#защита-блокировка-приватных-диапазонов-ip)
- [Защита: IMDSv2 в облаке](#защита-imdsv2-в-облаке)
- [Hidden vectors](#hidden-vectors)
- [Полный пример безопасного клиента в Go](#полный-пример-безопасного-клиента-в-go)
- [Что атакующий может сделать через SSRF](#что-атакующий-может-сделать-через-ssrf)
- [Известные инциденты](#известные-инциденты)
- [Чек-лист защиты](#чек-лист-защиты)

---

## Простая аналогия

Представь курьера в офисном здании. Снаружи в здание не пускают (firewall). Но курьер ходит куда хочет — у него пропуск. Если внешний человек напишет курьеру записку "пожалуйста, отнеси этот пакет в комнату 401 директору" — курьер отнесёт. И принесёт обратно ответ. **Снаружи тебя не пустят к директору, но через курьера — можно.**

SSRF — это твой сервер в роли курьера. Атакующий снаружи не может достучаться до внутренних API, но может **попросить твой сервер** сделать запрос за него. И твой сервер с радостью делает — потому что он же внутри сети.

---

## Как выглядит уязвимость

Сценарий: фича "загрузить аватар по URL" или "сделать превью ссылки".

### Уязвимый код

```go
func handleFetchAvatar(w http.ResponseWriter, r *http.Request) {
    avatarURL := r.URL.Query().Get("url")  // user input!

    // Сервер делает запрос на любой URL
    resp, err := http.Get(avatarURL)
    if err != nil {
        http.Error(w, "fetch failed", 500)
        return
    }
    defer resp.Body.Close()

    // Возвращает результат пользователю
    io.Copy(w, resp.Body)
}
```

Атакующий вызывает:
```
GET /fetch-avatar?url=http://169.254.169.254/latest/meta-data/iam/security-credentials/
```

Сервер на AWS делает запрос к этому URL — это metadata service AWS — и возвращает атакующему AWS credentials. Через них атакующий получает доступ ко всему облачному аккаунту.

То же самое работает с:
- `http://localhost:6379/` — Redis на хосте
- `http://10.0.0.5:8080/admin` — внутренний админ-API
- `http://internal-db.local/` — внутренний DNS
- `file:///etc/passwd` — если HTTP-клиент поддерживает file://

---

## Куда атакующий хочет попасть

### 1. Cloud metadata services

Каждый VM в облаке имеет специальный endpoint, через который софт на VM может узнать свои credentials. Без правильной защиты — это золотая жила для SSRF.

**AWS:** `http://169.254.169.254/latest/meta-data/`
- IAM credentials с правами на S3, RDS, EC2
- VPC info, security groups
- User-data (часто содержит секреты bootstrap'а)

**GCP:** `http://metadata.google.internal/computeMetadata/v1/`
- Service account tokens
- Project metadata

**Azure:** `http://169.254.169.254/metadata/instance`

Метадата доступна с `127.0.0.1` и из VM — но **снаружи интернета её нет**. SSRF создаёт мост.

### 2. Внутренние сервисы

В Kubernetes и микросервисных архитектурах кучу сервисов доступны только изнутри:

- `http://kubernetes.default.svc.cluster.local/api/` — Kubernetes API
- `http://prometheus:9090/` — внутренний Prometheus с метриками всего кластера
- `http://elasticsearch:9200/_cluster/health` — логи всего production
- `http://internal-admin-panel/` — админка без external auth (предполагается что снаружи не пускают)

### 3. Localhost сервисы

На самой машине часто работают сервисы с `bind 127.0.0.1` — без auth, потому что "только локально":
- `http://localhost:6379/` — Redis
- `http://localhost:9200/` — Elasticsearch
- `http://localhost:8500/` — Consul
- `http://localhost:5984/` — CouchDB

Через SSRF атакующий стучится с этой же машины к этим сервисам.

### 4. Файлы через `file://`

Если HTTP-клиент поддерживает schema `file://` — `file:///etc/passwd`, `file:///root/.aws/credentials`. Go-овский `http.Get` по умолчанию НЕ поддерживает `file://`, но кастомные клиенты или сторонние библиотеки могут.

### 5. Сканирование внутренней сети

Атакующий перебирает IP `10.0.0.1`, `10.0.0.2`, ... — твой сервер делает запросы, по timing и кодам ответа атакующий составляет карту внутренней сети.

---

## Capital One — самый известный SSRF инцидент

В 2019 хакер Paige Thompson через SSRF в Web Application Firewall Capital One:

1. Нашла misconfigured WAF, который позволял SSRF
2. Через SSRF запросила IAM credentials с metadata endpoint AWS
3. Получила credentials с правом на S3 чтение
4. Скачала бакеты Capital One — **106 миллионов** клиентских записей: SSN, банковские счета, заявки на кредиты

Capital One выплатил **$80M штрафа** регулятору + **$190M settlement** клиентам. Хакеру дали 5 лет тюрьмы.

Уязвимость — одна-единственная "невинная" фича в WAF, через которую можно было заставить его сделать запрос на metadata endpoint. После этого инцидента AWS ускорил релиз **IMDSv2** — новой версии metadata service, защищённой от SSRF.

---

## Защита: allowlist URL

**Лучшая защита** — не давать пользователю произвольно указывать URL. Если фича "превью ссылок" — список доверенных доменов:

```go
var allowedDomains = map[string]bool{
    "youtube.com":       true,
    "vimeo.com":         true,
    "github.com":        true,
    "twitter.com":       true,
}

func isAllowedURL(rawURL string) error {
    u, err := url.Parse(rawURL)
    if err != nil {
        return fmt.Errorf("invalid url: %w", err)
    }

    if u.Scheme != "https" {
        return errors.New("only https allowed")
    }

    // Только зарегистрированный домен (eTLD+1) — не поддомены
    domain, err := publicsuffix.EffectiveTLDPlusOne(u.Hostname())
    if err != nil {
        return err
    }

    if !allowedDomains[domain] {
        return fmt.Errorf("domain %q not allowed", domain)
    }

    return nil
}
```

Allowlist — самый безопасный подход. Если фича не подходит под allowlist — может, она вообще не нужна?

---

## Защита: блокировка приватных диапазонов IP

Если allowlist невозможен (например, "загрузка картинки по любому URL") — нужно блокировать запросы на private/internal IP.

**Опасные диапазоны:**

| Диапазон | Что |
|---|---|
| `127.0.0.0/8` | Localhost |
| `10.0.0.0/8` | Private (RFC1918) |
| `172.16.0.0/12` | Private (RFC1918) |
| `192.168.0.0/16` | Private (RFC1918) |
| `169.254.0.0/16` | Link-local (cloud metadata!) |
| `100.64.0.0/10` | Carrier-grade NAT |
| `0.0.0.0/8` | "this network" |
| `224.0.0.0/4` | Multicast |
| `::1/128` | IPv6 localhost |
| `fc00::/7` | IPv6 unique local |
| `fe80::/10` | IPv6 link-local |

```go
import "net"

var privateBlocks []*net.IPNet

func init() {
    for _, cidr := range []string{
        "127.0.0.0/8",
        "10.0.0.0/8",
        "172.16.0.0/12",
        "192.168.0.0/16",
        "169.254.0.0/16",   // cloud metadata!
        "100.64.0.0/10",
        "::1/128",
        "fc00::/7",
        "fe80::/10",
    } {
        _, block, _ := net.ParseCIDR(cidr)
        privateBlocks = append(privateBlocks, block)
    }
}

func isPrivateIP(ip net.IP) bool {
    for _, block := range privateBlocks {
        if block.Contains(ip) {
            return true
        }
    }
    return false
}
```

### Подвох — DNS rebinding

Простая проверка `isPrivateIP(net.ParseIP(hostname))` НЕ работает для доменов. Атакующий регистрирует домен `evil.com` с DNS-записью, указывающей на `127.0.0.1`. Хост `evil.com` — публичный, IP — приватный.

Хуже того: **DNS rebinding атака**:
1. Атакующий регистрирует `attacker.com` с TTL=0
2. На запрос #1 DNS возвращает `8.8.8.8` (публичный)
3. Сервер проверяет — публичный, разрешает
4. Сервер делает запрос — но в этот момент DNS-запись уже **`127.0.0.1`**
5. Сервер делает запрос на свой localhost

**Решение** — резолвить DNS один раз, проверять, и использовать тот же IP в HTTP-клиенте:

```go
func safeDial(ctx context.Context, network, addr string) (net.Conn, error) {
    host, port, err := net.SplitHostPort(addr)
    if err != nil {
        return nil, err
    }

    // Резолвим
    ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
    if err != nil {
        return nil, err
    }

    // Проверяем все полученные IP
    for _, ip := range ips {
        if isPrivateIP(ip.IP) {
            return nil, fmt.Errorf("blocked private ip: %s", ip.IP)
        }
    }

    // Подключаемся по первому проверенному IP (НЕ по hostname — иначе DNS resolved заново и может вернуть другое)
    return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(
        ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

client := &http.Client{
    Transport: &http.Transport{
        DialContext: safeDial,
    },
    Timeout: 10 * time.Second,
}
```

### Защита через сетевую изоляцию

Самый надёжный способ — на уровне инфраструктуры:
- сервис, делающий внешние запросы, **не имеет доступа к internal сети**
- запускается в отдельном subnet с egress-only правилами
- VPC NACL/security groups блокируют исходящие на `169.254.0.0/16`, `10.0.0.0/8` и т.д.

Тогда даже если код уязвим — пакет не уйдёт из subnet'а. Defense-in-depth.

---

## Защита: IMDSv2 в облаке

AWS после Capital One выпустил IMDSv2 — версию metadata service, защищённую от SSRF.

**IMDSv1 (уязвимая):**
```bash
curl http://169.254.169.254/latest/meta-data/
# → данные
```
Простой GET. Любой SSRF получает credentials.

**IMDSv2 (защищённая):**
```bash
# 1. Получить session token (требует PUT с заголовком)
TOKEN=$(curl -X PUT "http://169.254.169.254/latest/api/token" \
    -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")

# 2. Использовать token
curl http://169.254.169.254/latest/meta-data/ \
    -H "X-aws-ec2-metadata-token: $TOKEN"
```

Зачем PUT? Большинство SSRF-векторов — это GET-запросы (через `?url=...`). PUT с custom header — гораздо реже встречается. Это поднимает планку для атакующего.

**Включить enforced IMDSv2:**

```bash
# Через AWS CLI
aws ec2 modify-instance-metadata-options \
    --instance-id i-xxx \
    --http-tokens required \
    --http-put-response-hop-limit 1
```

`hop-limit 1` — metadata доступен только из самой VM, не из контейнеров на ней (по умолчанию 2 — Docker контейнер мог бы достучаться). Для Kubernetes — должен быть 1, чтобы контейнеры не получали credentials ноды.

GCP и Azure имеют похожие защиты — header `Metadata-Flavor: Google` обязателен для GCP.

---

## Hidden vectors

SSRF прячется не только в очевидном `?url=`. Везде где приложение делает HTTP-запрос на основе пользовательского ввода:

### Webhook URL

```go
// Пользователь регистрирует webhook для своего бота
func registerWebhook(url string) error {
    return db.Save("webhook", url)
}

// Когда событие — отправить запрос
func sendEvent(url string, event Event) {
    http.Post(url, "application/json", payload)  // SSRF возможен
}
```

Атакующий регистрирует webhook на `http://169.254.169.254/...` — твой сервис будет POSTить ему metadata.

### Image processing

```go
// Загрузка аватара по URL
img, _ := http.Get(userProvidedURL)
process(img.Body)
```

### Открытые редиректы как вектор

```go
// Безобидная фича: после OAuth редирект на ?next=...
http.Redirect(w, r, r.URL.Query().Get("next"), 302)
```

Сама по себе — open redirect (фишинг). Но в комбинации с SSRF — атакующий передаёт `?next=https://evil.com/redirect-to-localhost` — твой следующий сервис, который читает Location header, попадает в SSRF.

### XML и SVG

Парсеры XML с включенным XXE (External Entity) делают HTTP-запросы при парсинге — SSRF через XML-payload:

```xml
<?xml version="1.0"?>
<!DOCTYPE foo [<!ENTITY xxe SYSTEM "http://169.254.169.254/...">]>
<root>&xxe;</root>
```

В Go — отключай DTD parsing. SVG-картинки тоже могут содержать XXE.

### PDF и Office processing

Если рендеришь PDF из HTML или конвертируешь Office docs — рендереры делают запросы на embedded ресурсы. SSRF через `<img src="http://169.254.169.254/...">` в HTML.

### Server-side request на PDF preview

```go
// Сервис делает screenshot URL через headless Chrome
chromedp.Run(ctx, chromedp.Navigate(userURL))  // SSRF
```

Headless browsers — отдельный вектор. Помимо metadata, через них можно зайти на localhost и эксплуатировать.

---

## Полный пример безопасного клиента в Go

```go
package safehttp

import (
    "context"
    "errors"
    "fmt"
    "net"
    "net/http"
    "net/url"
    "time"
)

var privateBlocks []*net.IPNet

func init() {
    cidrs := []string{
        "127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12",
        "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10",
        "0.0.0.0/8", "224.0.0.0/4",
        "::1/128", "fc00::/7", "fe80::/10",
    }
    for _, cidr := range cidrs {
        _, block, _ := net.ParseCIDR(cidr)
        privateBlocks = append(privateBlocks, block)
    }
}

func isPrivateIP(ip net.IP) bool {
    for _, block := range privateBlocks {
        if block.Contains(ip) {
            return true
        }
    }
    return false
}

// safeControl вызывается перед dial — последняя проверка
func safeControl(network, addr string, c syscall.RawConn) error {
    host, _, err := net.SplitHostPort(addr)
    if err != nil {
        return err
    }
    ip := net.ParseIP(host)
    if ip == nil {
        return errors.New("not an ip")
    }
    if isPrivateIP(ip) {
        return fmt.Errorf("blocked: %s", ip)
    }
    return nil
}

func NewSafeClient() *http.Client {
    dialer := &net.Dialer{
        Timeout:   5 * time.Second,
        KeepAlive: 30 * time.Second,
        Control:   safeControl,  // блокировка после resolve
    }

    transport := &http.Transport{
        DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
            host, port, err := net.SplitHostPort(addr)
            if err != nil {
                return nil, err
            }

            // Резолвим один раз, проверяем все IP
            ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
            if err != nil {
                return nil, err
            }
            if len(ips) == 0 {
                return nil, fmt.Errorf("no ips for %s", host)
            }
            for _, ip := range ips {
                if isPrivateIP(ip.IP) {
                    return nil, fmt.Errorf("blocked private ip: %s", ip.IP)
                }
            }

            // Подключаемся по resolved IP — DNS rebinding не сработает
            return dialer.DialContext(ctx, network,
                net.JoinHostPort(ips[0].IP.String(), port))
        },
        TLSHandshakeTimeout:   5 * time.Second,
        ResponseHeaderTimeout: 10 * time.Second,
        DisableKeepAlives:     false,
        MaxIdleConnsPerHost:   10,
    }

    return &http.Client{
        Transport: transport,
        Timeout:   30 * time.Second,
        // Запретить редиректы или валидировать каждый
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) > 5 {
                return errors.New("too many redirects")
            }
            // Проверить URL редиректа на private IP тоже
            return validateURL(req.URL)
        },
    }
}

func validateURL(u *url.URL) error {
    if u.Scheme != "http" && u.Scheme != "https" {
        return fmt.Errorf("unsupported scheme: %s", u.Scheme)
    }
    return nil
}
```

Использование:

```go
client := safehttp.NewSafeClient()
resp, err := client.Get(userProvidedURL)
```

Этот клиент:
1. Проверяет схему (только http/https)
2. Резолвит DNS, блокирует приватные IP
3. Использует `Control` для проверки на этапе dial (защита от race)
4. Подключается по IP, не по hostname (DNS rebinding mitigation)
5. Имеет таймауты на всех этапах
6. Ограничивает редиректы и проверяет каждый

---

## Что атакующий может сделать через SSRF

В порядке убывания тяжести:

**1. Утечка cloud credentials → захват аккаунта.** Через metadata service атакующий получает IAM credentials. Через них — доступ ко всему: S3 (данные клиентов), RDS (БД), EC2 (запуск своих машин для майнинга), IAM (создание новых пользователей с правами). Capital One потеряли 106M записей именно так.

**2. Доступ к внутренним сервисам.** Внутренний admin panel без auth ("ведь снаружи не пускают"), Prometheus с метриками всех сервисов, Kubernetes API с правами SA сервиса.

**3. Эксплуатация уязвимых внутренних сервисов.** Старый Redis, старый Elasticsearch, JMX endpoint Java-приложения — все с RCE-уязвимостями, которые "не страшны потому что только локально". Через SSRF — таки страшны.

**4. Сканирование внутренней сети.** Атакующий мапит твою инфраструктуру: какие IP, какие порты, какие сервисы. По таймингу и кодам ответа собирается полная карта.

**5. Эксфильтрация через DNS / out-of-band.** Если атакующий не получает прямой ответ — заставляет твой сервер сделать DNS-запрос вида `data-{base64-of-secret}.attacker.com`. На DNS-сервере attacker.com видит запрос и декодирует данные.

**6. Internal service-to-service abuse.** В микросервисной архитектуре часто внутренние сервисы доверяют друг другу без auth. SSRF превращает один скомпрометированный сервис во входную точку ко всему.

---

## Известные инциденты

**Capital One (2019)** — описан выше. 106M записей, $270M в штрафах и settlement.

**Shopify (2019, bug bounty).** SSRF в фиче "import store from URL" — позволял достать AWS credentials. Bug bounty $25k.

**GitLab (2018, bug bounty).** SSRF через "import project from URL" — доступ к internal Docker registry. Bug bounty $1k.

**Slack (2016, bug bounty).** SSRF в Slack's link unfurling (превью ссылок) — давал доступ к internal services. После этого Slack публично описал свою архитектуру защиты от SSRF.

**MyBB (2018).** SSRF в популярном форумном движке через avatar fetching → доступ к metadata. Десятки тысяч форумов уязвимы.

**Bug bounty статистика:** SSRF — стабильно в топе самых дорогих уязвимостей. Средняя выплата $5k-$50k, верхние — до $30k+ за SSRF в cloud-приложениях.

---

## Чек-лист защиты

**Обязательно:**
- ✅ Allowlist разрешённых доменов везде где возможно
- ✅ Если allowlist невозможен — блокировка приватных IP с проверкой ПОСЛЕ DNS resolve
- ✅ Подключение по resolved IP, не по hostname (защита от DNS rebinding)
- ✅ Только `http`/`https` схемы — никаких `file://`, `gopher://`, `dict://`, `ftp://`
- ✅ IMDSv2 на AWS (или эквивалент на GCP/Azure) с hop-limit 1
- ✅ Таймауты на все HTTP-запросы (Connect, TLS, Read)

**Сильно желательно:**
- ✅ Ограничение редиректов (max 5) с валидацией каждого
- ✅ Сетевая изоляция: сервис, делающий внешние запросы, в отдельном subnet с egress-only правилами
- ✅ Egress firewall: блокировка `169.254.0.0/16`, `10.0.0.0/8` на VPC-уровне
- ✅ Логирование исходящих запросов на необычные адреса
- ✅ Лимит на размер ответа (защита от amplification и memory exhaustion)
- ✅ Заголовок `Host` устанавливать явно — не из user input

**XML/SVG/PDF processing:**
- ✅ Отключить DTD/external entity processing в XML парсерах
- ✅ Отключить network access в SVG/HTML рендерерах
- ✅ Headless Chrome — `--no-sandbox` НЕ использовать, `--proxy-server` для контроля сети

**Тестирование:**
- ✅ Burp Collaborator или ngrok для проверки SSRF в bug bounty/pentest
- ✅ Static analysis на паттерны `http.Get(userInput)`, `http.Post(userInput, ...)`
- ✅ Включить выделенный test endpoint вроде `http://localhost:9999/secret` и проверить что внешние сервисы не могут на него попасть
