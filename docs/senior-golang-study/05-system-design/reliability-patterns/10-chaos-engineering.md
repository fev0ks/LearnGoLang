# Chaos Engineering

Chaos Engineering — практика **намеренно** ломать production-системы в контролируемых условиях, чтобы найти слабые места **до того** как они проявятся сами. Парадоксальная идея, рождённая в Netflix около 2010 года, ставшая стандартом в крупных компаниях.

Главная мысль: **надёжность системы неизвестна, пока её не пробовали сломать**. Тесты в CI проверяют логику. Chaos engineering проверяет **архитектуру** — как система ведёт себя когда что-то отказывает.

## Содержание

- [Простая аналогия](#простая-аналогия)
- [Зачем намеренно ломать production](#зачем-намеренно-ломать-production)
- [Принципы chaos engineering](#принципы-chaos-engineering)
- [Что можно ломать: типы экспериментов](#что-можно-ломать-типы-экспериментов)
- [Hypothesis-driven подход](#hypothesis-driven-подход)
- [Blast radius — контроль ущерба](#blast-radius-контроль-ущерба)
- [Game days](#game-days)
- [Инструменты](#инструменты)
- [Chaos в Go-сервисах](#chaos-в-go-сервисах)
- [Внедрение в команде](#внедрение-в-команде)
- [Anti-patterns](#anti-patterns)
- [Известные истории](#известные-истории)

---

## Простая аналогия

Представь пожарную службу. Они не ждут пожара чтобы тренироваться. Они **устраивают учения** — поджигают тренировочный дом, спасают манекенов. Когда случится реальный пожар — реакция отрепетирована.

Backend-команды без chaos engineering — это пожарная служба, которая впервые увидит пожар когда он начнётся. Знания об оборудовании, процедурах, координации между людьми — будут учиться **в момент инцидента**. Плохой момент.

Chaos engineering — это пожарные учения для distributed systems. Ты **симулируешь** отказы (БД упала, сеть разорвана, диск переполнен, latency 10 секунд), смотришь как реагирует система, исправляешь слабые места.

---

## Зачем намеренно ломать production

### 1. Найти неизвестные зависимости

"Если упадёт Redis — ничего страшного, у нас БД есть". На бумаге — да. В реальности часто:
- Сервис A ходит в Redis за конфигом → конфиг не загружен → сервис не запускается
- Сервис B хранит rate limit state в Redis → без Redis нет rate limit → DDoS прорывается
- Auth middleware кэширует sessions в Redis → без Redis каждый запрос идёт в auth-сервис → перегрузка

Без эксперимента эти зависимости остаются невидимыми. **Chaos находит их до production-инцидента**.

### 2. Проверить fallback'и

Ты написал circuit breaker, retry с backoff, fallback на кэш. Работает ли всё это **реально** под нагрузкой? До chaos — это надежда. После — знание.

### 3. Найти cascading failures

Один сервис тормозит → его клиенты копят соединения → их БД connection pool исчерпывается → они начинают тормозить → их клиенты копят → cascade. Без эксперимента таких связей не увидеть.

### 4. Тренировать команду

Реальный инцидент в 3 часа ночи — высокий стресс. **Game day** с симуляцией готовит людей: знают где смотреть метрики, как откатить деплой, кому звонить.

### 5. Доказать reliability claims

Бизнес: "у нас 99.99% SLO". Без chaos — это вера. С chaos — измеряемая характеристика. SLO становится **верифицируемой**.

---

## Принципы chaos engineering

Из [Principles of Chaos Engineering](https://principlesofchaos.org/):

### 1. Define "steady state"

Что значит "система работает нормально"? Это должна быть **измеримая метрика**, отражающая бизнес-ценность:
- Successful checkouts per minute
- Login success rate
- API p99 latency
- Stream playback bitrate

**Плохо:** "CPU usage ниже 80%" (системная метрика, не пользовательская).

**Хорошо:** "доля успешных запросов > 99.5%".

### 2. Hypothesise

Сформулируй гипотезу что произойдёт: "Если убью один экземпляр сервиса A, steady state не изменится (load balancer переключит трафик на остальные)".

### 3. Vary real-world events

Симулируй реальные сбои:
- Crash экземпляра сервиса
- Сетевые проблемы (latency, packet loss)
- Disk full
- CPU spike
- Memory leak
- DNS failure

Не выдумывай экзотику — то что **реально** может случиться.

### 4. Run experiments in production

Контр-интуитивно, но критично. В staging — другие данные, другая нагрузка, другая инфра. Реальные слабости видны только на проде.

Это требует **контроля blast radius** — см. ниже.

### 5. Automate

Ручные эксперименты — раз в месяц. Автоматические — каждый день, в фоне. Регрессии надёжности находятся быстро.

### 6. Minimize blast radius

Начинай с минимального ущерба:
- 1% пользователей → 10% → 50% → 100%
- Один pod → одна AZ → один регион
- Test environment → staging → production canary → full production

Всегда имей **kill switch** — мгновенно остановить эксперимент.

---

## Что можно ломать: типы экспериментов

### Infrastructure failures

**Instance termination:**
```bash
# Случайно убить EC2 instance в production
aws ec2 terminate-instances --instance-ids i-abc123
```
Проверяет: автомасштабирование сработает? Load balancer удалит мёртвый node? Текущие соединения корректно завершатся?

**Network partition:**
Изолировать сервис от части инфраструктуры. Проверяет partition tolerance (CAP).

**Datacenter / AZ failure:**
Заблокировать весь трафик из/в одной AZ. Проверяет multi-AZ deployment.

### Network failures

**Latency injection:**
Добавить 1-10 секунд задержки на запросы к Redis/БД. Проверяет timeout'ы и user-facing UX.

**Packet loss:**
20% пакетов теряются. Проверяет retry-логику.

**Connection reset:**
Закрывать TCP connections принудительно. Проверяет reconnect-логику.

### Resource failures

**CPU spike:**
Запустить процесс жгущий CPU. Проверяет: throttling, autoscaling, deadlines.

**Memory pressure:**
Заполнить heap до лимита. Проверяет OOM handling, graceful degradation.

**Disk full:**
Заполнить диск. Проверяет: что делает сервис, когда нет места записать лог или временный файл?

**Disk slow:**
Замедлить fsync. Проверяет: что если БД пишет медленно?

### Application failures

**HTTP errors:**
Заставить downstream сервис возвращать 500. Проверяет error handling.

**Slow responses:**
Downstream отвечает за 30 секунд. Проверяет timeout'ы и circuit breaker.

**Corrupt responses:**
Возвращать невалидный JSON. Проверяет input validation.

### People & process failures

- On-call недоступен — есть ли backup?
- Runbook отсутствует — кто-то знает что делать?
- Deploy сломан — есть ли rollback?
- Логи недоступны — есть ли альтернативный observability?

---

## Hypothesis-driven подход

Хороший chaos experiment — это **научный эксперимент** с гипотезой и измерениями.

### Шаблон эксперимента

```
ЭКСПЕРИМЕНТ: что мы проверяем

STEADY STATE: какая метрика отражает "нормальную работу"
   Например: успешность checkout > 99.5%, p99 latency < 500ms

HYPOTHESIS: что ожидаем
   "Если убьём один экземпляр payment-service, steady state не изменится
    (load balancer переключит на оставшиеся 4 экземпляра в течение 30 сек)"

VARIABLE (что меняем):
   Завершить случайный pod payment-service через kubectl delete

BLAST RADIUS:
   1 из 5 pods (20% capacity removed)
   Длительность: 10 минут
   Время: 14:00 UTC (бизнес-часы, есть кому реагировать)

ABORT CONDITIONS (что прерывает эксперимент):
   - Успешность checkout падает ниже 99%
   - p99 latency превышает 2 секунды

ROLLBACK:
   Pod автоматически восстановится через deployment.
   Если что-то пошло не так — отменяем эксперимент через chaos tool.

RESULT:
   ✅ Successful: pod восстановился за 15 сек, успешность не упала
   или
   ❌ Failed: p99 подскочил до 3 секунд → корень проблемы → действие
```

### Результаты — это lessons

Эксперимент "провалился" — отличный результат. Нашли проблему до production-инцидента. Действие: создать ticket, фиксить root cause, повторить эксперимент после фикса.

Эксперимент "успешный" — тоже хорошо. Подтвердил надёжность. Можно расширять blast radius в следующий раз.

---

## Blast radius — контроль ущерба

Главное правило chaos в production — **начинать минимально**. Эскалация:

```
1. Staging only          ← начало для нового эксперимента
2. Production, 0.1% traffic
3. Production, 1% traffic
4. Production, 10% traffic
5. Production, single AZ
6. Production, full
```

Каждый шаг — после успешного предыдущего и review результатов.

### Always have a kill switch

```go
// В коде сервиса
if chaosController.IsExperimentActive("redis-latency") {
    // Аборт возможен в любой момент через flag
}

// HTTP endpoint для аварийной остановки
http.HandleFunc("/admin/chaos/stop", func(w http.ResponseWriter, r *http.Request) {
    chaosController.StopAll()
})
```

Эксперимент должен останавливаться **за секунды**, не минуты.

### Automated abort

Лучше — автомат:

```yaml
# Litmus / Chaos Mesh
abortConditions:
  - metric: "http_5xx_rate"
    threshold: "> 5%"
    duration: "30s"
    action: "stop"
```

Метрика выходит за норму → эксперимент останавливается без участия человека.

---

## Game days

**Game day** — запланированное упражнение с участием команды:
- Заранее знают что **что-то** случится
- Не знают **что именно** или **когда**
- Тренируются реагировать как на реальный инцидент

### Формат

**До дня:**
- Объявить: "среда, 10:00-13:00 — game day"
- Все участники свободны от обычных задач
- Подготовить сценарий (организаторы знают, остальные нет)

**В день:**
- 10:00 — пейджер срабатывает: "p99 latency растёт"
- Команда диагностирует — что случилось? как починить?
- Организаторы наблюдают: всё ли понятно? есть ли runbook? кто-то знает где смотреть метрики?
- 12:00 — раскрываем что было

**После:**
- Retro: что работало хорошо, что плохо
- Action items: добавить metric, написать runbook, улучшить алерт

### Что тренировать

- **Page handling** — кто отвечает на пейджер первым?
- **Diagnosis** — за сколько находим root cause?
- **Communication** — как обновляем status page и stakeholders?
- **Coordination** — кто-то IC (incident commander), кто-то delegates?
- **Rollback** — за сколько откатываем плохой деплой?
- **Post-mortem writing** — пишут ли blameless?

См. также: [09-postmortem.md](./09-postmortem.md).

---

## Инструменты

### Chaos Monkey (Netflix)

Дедушка всех chaos tools. Случайно убивает EC2 instances в production в рабочие часы. С 2010 года в Netflix.

```yaml
# Chaos Monkey спросит у Spinnaker:
# "какие сервисы можно ломать?"
# "когда не ломать?" (на праздниках, во время больших релизов)
```

Сейчас часть [Simian Army](https://netflix.github.io/) — набор chaos tools от Netflix.

### Chaos Mesh

Kubernetes-native chaos engineering. Через CRD задаёшь эксперименты:

```yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: PodChaos
metadata:
  name: kill-payment-pod
spec:
  action: pod-kill
  mode: one
  selector:
    namespaces: [production]
    labelSelectors:
      app: payment-service
  duration: "30s"
```

Поддерживает: pod-kill, network chaos (latency, loss), IO chaos (slow disk), stress (CPU/memory), time chaos (clock skew).

### Litmus

Аналог Chaos Mesh, тоже Kubernetes-native. Опен-сорс от CNCF.

```yaml
apiVersion: litmuschaos.io/v1alpha1
kind: ChaosEngine
metadata:
  name: payment-chaos
spec:
  experiments:
    - name: pod-network-latency
      spec:
        components:
          env:
            - name: NETWORK_LATENCY
              value: "2000"  # 2 секунды
            - name: TOTAL_CHAOS_DURATION
              value: "60"
```

### AWS Fault Injection Simulator

Managed chaos tool от AWS. Templates для EC2, RDS, EKS:

```bash
aws fis start-experiment --template-id template-abc
```

### Gremlin

Коммерческое решение с UI. Best-in-class UX, но дорого. Используется в Fortune 500.

### tc (Linux Traffic Control)

Низкоуровневый инструмент. Можешь руками:

```bash
# Добавить 100ms задержки на eth0
tc qdisc add dev eth0 root netem delay 100ms

# Packet loss 10%
tc qdisc add dev eth0 root netem loss 10%

# Убрать
tc qdisc del dev eth0 root
```

Используется внутри других tools.

### Toxiproxy (Shopify)

TCP-прокси с возможностью добавлять "toxics": latency, slow_close, timeout, bandwidth limit.

```go
// В тестах
proxy, _ := toxiproxy.CreateProxy("redis", "localhost:8474", "localhost:6379")
proxy.AddToxic("latency", "latency", "downstream", 1.0, toxiproxy.Attributes{
    "latency": 2000,  // 2 секунды
})

// Теперь Go-сервис, ходящий в Redis через proxy, испытывает 2s задержку
// Проверяем: timeout? circuit breaker? fallback?
```

Очень удобно для chaos тестов в Go.

---

## Chaos в Go-сервисах

### 1. Fault injection через библиотеку

```go
package chaos

import (
    "math/rand"
    "time"
)

type Injector struct {
    enabled       bool
    failureRate   float64  // 0.0 - 1.0
    latencyMin    time.Duration
    latencyMax    time.Duration
}

func (i *Injector) Inject(ctx context.Context) error {
    if !i.enabled {
        return nil
    }

    // Random latency
    if i.latencyMax > 0 {
        delay := i.latencyMin + time.Duration(rand.Int63n(int64(i.latencyMax-i.latencyMin)))
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(delay):
        }
    }

    // Random failure
    if rand.Float64() < i.failureRate {
        return errors.New("chaos: injected failure")
    }

    return nil
}

// Использование в HTTP handler
func handleCheckout(w http.ResponseWriter, r *http.Request) {
    if err := chaosInjector.Inject(r.Context()); err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    // ... нормальная логика
}
```

Управление через feature flag — включается на staging или мелкой доле трафика.

### 2. Slow downstream через middleware

```go
// HTTP client с искусственной задержкой
type chaosTransport struct {
    base   http.RoundTripper
    delay  time.Duration
    failureRate float64
}

func (t *chaosTransport) RoundTrip(r *http.Request) (*http.Response, error) {
    time.Sleep(t.delay)

    if rand.Float64() < t.failureRate {
        return nil, errors.New("chaos: simulated network error")
    }

    return t.base.RoundTrip(r)
}

client := &http.Client{
    Transport: &chaosTransport{
        base:        http.DefaultTransport,
        delay:       2 * time.Second,
        failureRate: 0.1,
    },
}
```

### 3. Toxiproxy в integration tests

```go
func TestRedisLatency(t *testing.T) {
    proxy := toxiproxy.NewClient("localhost:8474")
    redisProxy, _ := proxy.CreateProxy("redis-test", "localhost:0", "localhost:6379")
    defer redisProxy.Delete()

    // Get the proxy address
    redisAddr := redisProxy.Listen

    // Connect Go service through proxy
    cache := NewCache(redisAddr)

    // Inject 5 second latency
    redisProxy.AddToxic("latency", "latency", "downstream", 1.0, toxiproxy.Attributes{
        "latency": 5000,
    })

    // Test: service should NOT block 5 seconds — should timeout and fallback
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()

    start := time.Now()
    _, err := cache.Get(ctx, "key")
    elapsed := time.Since(start)

    // Assert timeout fired, not waited for Redis
    assert.Less(t, elapsed, 2*time.Second)
    assert.Error(t, err)
}
```

### 4. Failpoints для unit tests

[gofail](https://github.com/etcd-io/gofail) — failpoints (как в etcd):

```go
// В коде сервиса
import "github.com/etcd-io/gofail/runtime"

func processOrder(o Order) error {
    // gofail: var BeforePayment string
    return chargePayment(o)
}

// В тесте включаем failpoint
gofail.Enable("BeforePayment", "panic")  // паника в этом месте
// Тестируем: что делает система при panic в processOrder?
```

---

## Внедрение в команде

Chaos engineering часто встречает сопротивление: "зачем намеренно ломать?". Подход к внедрению:

### Шаг 1: Начни с staging

Не стартуй сразу в production. На staging тренируйся:
- Какие эксперименты делать
- Как настраивать tools
- Что измерять

Раннее обучение без риска для пользователей.

### Шаг 2: Game days

Перед "automated chaos in prod" сделай несколько game days. Покажи команде ценность:
- Найдём проблему, которая бы аукнулась в 3 ночи
- Натренируемся как реагировать
- Постмортем — конкретные action items

### Шаг 3: Маленькие эксперименты в prod

Начни с очевидно безопасных:
- Убить один pod, который должен переподняться через 30 секунд
- Добавить 100ms latency к одному endpoint
- Включить chaos на 0.1% трафика

### Шаг 4: Автоматизация

Когда команда привыкла — автоматизируй:
- Chaos Monkey-like daemon
- Раз в день — random pod kill
- Раз в неделю — network chaos
- Метрики SLO мониторят влияние

### Шаг 5: Chaos as part of SDLC

- В CI: каждое merge — chaos test против fresh deployment
- В deployment: после canary roll-out — chaos на canary
- В постмортемах: action item — chaos test для предотвращения повторения

---

## Anti-patterns

**1. Chaos без observability.**
Запускаешь эксперимент, что-то ломается, не понимаешь почему. Без метрик/логов/traces chaos бесполезен. **Сначала observability, потом chaos**.

**2. Chaos без steady state metric.**
"Сломаем что-нибудь и посмотрим". Что значит "хуже"? Без чёткой метрики — субъективная оценка.

**3. Chaos в среде без resilience.**
Если сервис не имеет circuit breakers, retries, timeouts — chaos просто покажет что всё валится. Это не информация. **Сначала строй надёжность**, потом проверяй её через chaos.

**4. Слишком большой blast radius с самого начала.**
"Убьём всю production в test environment" — паника, ничего не учим. Постепенная эскалация.

**5. Без kill switch.**
Эксперимент идёт неправильно, а остановить нельзя. Catastrophic failure возможен.

**6. Эксперимент без гипотезы.**
"Просто сломаем". Без hypothesis нет четкого критерия "удачно/неудачно". Все эксперименты выглядят "interesting" и не приводят к фиксам.

**7. Игнорировать negative results.**
Эксперимент показал проблему → дальше не фиксят → проблема остаётся. Chaos ценен только если на основе результатов меняется код/архитектура.

**8. Только infra-level chaos.**
"Убей pod" — недостаточно. Реальные инциденты часто из application logic (memory leak, slow query, queue backup). Имитируй разное.

**9. Chaos в нерабочее время.**
"Безопаснее ночью". Нет! В 3 ночи никто не реагирует. Эксперимент в **рабочие часы** (с готовностью abort) — нужная команда на месте.

**10. Chaos без post-mortem.**
Прошёл эксперимент → разошлись. Без формального обсуждения — выводы не закрепляются. Делай retro даже на успешные эксперименты ("что мы научились").

---

## Известные истории

### Netflix Chaos Monkey (2010-)

Родина chaos engineering. Netflix перешёл из data center в AWS и обнаружил что нужна другая модель надёжности. **Решение:** постоянно убивать random instances в production. К 2025 — десятки tools в Simian Army, индустриальный стандарт.

### AWS DynamoDB outage (2015)

Региональный отказ DynamoDB вызвал каскадный фейл многих AWS сервисов и клиентов. После этого AWS начал агрессивно тестировать region isolation, появились FIS (Fault Injection Simulator).

### Google's DiRT (Disaster Recovery Testing)

Google проводит компанию-wide DiRT exercises: симулирует отключение целых датацентров, проверяет процедуры. По их данным — DiRT находит больше реальных проблем чем "органические" инциденты.

### Slack's Disasterpiece Theater

Slack делает регулярные chaos exercises с публичным написанием постмортемов. Прекрасные примеры reporting.

### Stripe's chaos engineering culture

Stripe (платежи!) активно делает chaos на production. Аргумент: "если уж мы делаем chaos на платёжном процессоре — никто не имеет оправдания не делать".

### GitHub's MySQL incident (2018)

Не chaos engineering, а реальный инцидент. Но: 24-часовой outage из-за split-brain в MySQL replication. После — GitHub серьёзно инвестировал в chaos engineering и game days.

---

## Полезные ссылки

- [Principles of Chaos Engineering](https://principlesofchaos.org/) — манифест
- [Chaos Engineering by Casey Rosenthal, Nora Jones (book)](https://www.oreilly.com/library/view/chaos-engineering/9781492043867/)
- [Awesome Chaos Engineering](https://github.com/dastergon/awesome-chaos-engineering) — список tools
- [Chaos Mesh docs](https://chaos-mesh.org/docs/)
- [Litmus docs](https://litmuschaos.io/)
- [Toxiproxy](https://github.com/Shopify/toxiproxy) — для Go integration tests
- [AWS Fault Injection Simulator](https://aws.amazon.com/fis/)
- [gofail](https://github.com/etcd-io/gofail) — failpoints для Go
