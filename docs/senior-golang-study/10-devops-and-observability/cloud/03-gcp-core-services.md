# Google Cloud: практический обзор основных сервисов

## Содержание

- [Ментальная модель GCP](#ментальная-модель-gcp)
- [С чего начинается работа](#с-чего-начинается-работа)
- [Карта выбора сервисов](#карта-выбора-сервисов)
- [Где запускать Go-приложение](#где-запускать-go-приложение)
- [Где хранить данные](#где-хранить-данные)
- [Асинхронная работа и интеграции](#асинхронная-работа-и-интеграции)
- [Сеть и внешний трафик](#сеть-и-внешний-трафик)
- [IAM, аутентификация и секреты](#iam-аутентификация-и-секреты)
- [Сборка и доставка](#сборка-и-доставка)
- [Наблюдаемость](#наблюдаемость)
- [Три типовые архитектуры](#три-типовые-архитектуры)
- [Соответствие сервисов AWS и GCP](#соответствие-сервисов-aws-и-gcp)
- [Стоимость и контроль расходов](#стоимость-и-контроль-расходов)
- [Типичные ошибки](#типичные-ошибки)
- [Практический чек-лист](#практический-чек-лист)
- [Interview-ready answer](#interview-ready-answer)
- [Официальная документация](#официальная-документация)

Google Cloud Platform (GCP), официально Google Cloud, — набор управляемых
сервисов вычислений, сети, хранения данных, баз данных и наблюдаемости. Для
backend-разработчика важнее не помнить весь каталог, а уметь собрать рабочий
путь: где запустить приложение, где хранить данные, как выдать ему права, как
принять внешний трафик и где искать причину сбоя.

Статья рассматривает GCP со стороны пользователя платформы. Внутреннее устройство
Spanner, глобального балансировщика или сети Google здесь не разбирается: оно
нужно значительно реже, чем правильный выбор продукта, региона, модели доступа и
границ ответственности.

---

## Ментальная модель GCP

Почти любой сценарий в GCP складывается из пяти решений:

1. В каком `project` живут ресурсы и кто за них платит.
2. В каком регионе находятся приложение и данные.
3. Какой managed-сервис выполняет работу.
4. Под каким service account работает приложение и что ему разрешено через IAM.
5. Как приложение наблюдается, обновляется и восстанавливается после сбоя.

Ресурсы организованы иерархически:

```text
Organization: example.com
└── Folder: production
    ├── Project: orders-prod
    │   ├── Cloud Run service
    │   ├── Cloud SQL instance
    │   └── Pub/Sub topics
    └── Project: analytics-prod
        └── BigQuery datasets
```

- **Organization** — корень компании и место для общих политик.
- **Folder** — необязательная группировка проектов, например по окружению,
  подразделению или требованиям безопасности.
- **Project** — основная граница ресурсов, включённых API, квот, IAM и учёта
  расходов. Для использования большинства сервисов нужен project.
- **Resource** — конкретный bucket, база, сервис Cloud Run, виртуальная машина или
  topic.

IAM-политики наследуются сверху вниз. Роль, выданная на organization или folder,
может открыть доступ ко всем дочерним проектам, поэтому высокоуровневые назначения
требуют особенно осторожного review.

### Region, zone и location

- **Region** — географическая область вроде `europe-west1`.
- **Zone** — отдельная зона размещения внутри региона, например
  `europe-west1-b`.
- **Location** — более общее название размещения. В зависимости от продукта это
  zone, region, dual-region или multi-region.

Вычисления и данные по умолчанию размещают рядом. Cloud Run в Европе и Cloud SQL
в США добавят latency и сетевые расходы к каждому запросу. Если данные нельзя
выносить из определённой юрисдикции, location становится не только вопросом
производительности, но и требованием compliance.

---

## С чего начинается работа

### Project, billing и API

Создание project ещё не делает все продукты доступными. Обычно нужно привязать
billing account и явно включить API нужных сервисов.

```bash
# Вход пользователя для команд gcloud
gcloud auth login

# Выбрать существующий project
gcloud config set project orders-dev
gcloud config set run/region europe-west1

# Включить API, необходимые для деплоя из исходного кода в Cloud Run
gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com

# Отдельные локальные credentials для клиентских библиотек и Terraform
gcloud auth application-default login
```

`gcloud auth login` аутентифицирует команды CLI. Команда
`gcloud auth application-default login` создаёт локальные Application Default
Credentials (ADC), которые находят Go-клиенты и Terraform. Это два разных
контекста аутентификации; успешный вызов `gcloud projects list` ещё не означает,
что локально запущенное Go-приложение получило credentials.

В production пользовательский вход не применяют. К Cloud Run, GKE или Compute
Engine прикрепляют service account, а библиотека получает его краткоживущие
credentials автоматически.

### Console, gcloud, client libraries и Terraform

С одним ресурсом можно работать четырьмя способами:

| Инструмент | Для чего подходит | Ограничение |
| --- | --- | --- |
| Google Cloud Console | изучение продукта, единичная диагностика | плохо воспроизводит изменения |
| `gcloud` | быстрые операции, скрипты, диагностика | длинная конфигурация становится неудобной |
| Cloud Client Libraries | вызовы API из приложения | не заменяют управление инфраструктурой |
| Terraform | воспроизводимая инфраструктура и review изменений | runtime-операции приложения сюда не относятся |

Практический подход: первый учебный ресурс можно создать через Console или
`gcloud`, а постоянную инфраструктуру описывать Terraform. Готовые GCP-паттерны
для Terraform находятся в [08-gcp-patterns.md](../terraform/08-gcp-patterns.md).

---

## Карта выбора сервисов

| Задача | Первый кандидат | Когда нужен другой вариант |
| --- | --- | --- |
| Запустить stateless HTTP/gRPC API в контейнере | Cloud Run service | GKE для Kubernetes-контроля; Compute Engine для контроля ОС |
| Выполнить контейнер и завершиться | Cloud Run job | Batch для больших VM- и GPU-задач |
| Обработать одно событие небольшой функцией | Cloud Run functions | Cloud Run service, если удобнее обычное контейнерное приложение |
| Запустить приложения в Kubernetes | GKE Autopilot | GKE Standard, если нужен контроль node pools и привилегированные возможности |
| Запустить legacy-приложение или произвольный daemon | Compute Engine | GKE, если приложение уже контейнеризовано и нужна оркестрация |
| Хранить файлы и большие неизменяемые объекты | Cloud Storage | Persistent Disk/Filestore, если нужны файловые или блочные операции |
| Обычная PostgreSQL/MySQL/SQL Server база | Cloud SQL | Spanner для горизонтального масштаба SQL; self-hosted DB для полного контроля |
| Требовательная PostgreSQL-compatible нагрузка | AlloyDB | Cloud SQL для более простой и дешёвой отправной точки |
| Документы с известными запросами | Firestore | Cloud SQL для joins и сложной реляционной модели |
| Огромный объём single-keyed данных с высокой пропускной способностью | Bigtable | Firestore для документов; BigQuery для аналитических сканов |
| Глобальная транзакционная SQL-база большого масштаба | Spanner | Cloud SQL, если одна managed SQL instance закрывает нагрузку |
| Аналитика по большим наборам данных | BigQuery | Cloud SQL/Spanner для коротких OLTP-транзакций |
| Cache, sessions, rate-limit state | Memorystore | Основная БД, если потеря данных недопустима |
| Рассылка событий нескольким независимым системам | Pub/Sub | Cloud Tasks для адресной фоновой команды одному HTTP worker |
| Отложенный вызов worker с retry и rate limit | Cloud Tasks | Pub/Sub для событий и fan-out |
| Запуск по cron | Cloud Scheduler | Workflows, если запуск состоит из нескольких шагов |
| Оркестрация API и долгих шагов | Workflows | Код приложения, если процесс короткий и локальный |
| Хранить container images и пакеты | Artifact Registry | внешний registry, если этого требует общий multi-cloud pipeline |
| Собирать приложение в GCP | Cloud Build | существующая CI-система через Workload Identity Federation |
| Метрики, dashboards и alerts | Cloud Monitoring | Managed Service for Prometheus для Prometheus-совместимого стека |
| Централизованные логи | Cloud Logging | внешний SIEM или хранилище логов при специальных требованиях |

Таблица даёт стартовый вариант, а не автоматический ответ. Выбор меняют access
patterns, требования к отказоустойчивости, опыт команды, цена миграции и
операционная сложность.

---

## Где запускать Go-приложение

### Cloud Run service

Cloud Run service запускает контейнер и выдаёт ему стабильный HTTPS endpoint.
Платформа управляет экземплярами и масштабированием, а приложение отвечает за
HTTP/gRPC-сервер, обработку сигналов завершения и сохранение состояния во внешних
системах.

Это основной кандидат для нового stateless Go API, если не требуется Kubernetes:

- можно деплоить исходный код или готовый container image;
- сервис создаёт revisions, между которыми можно делить трафик;
- число экземпляров автоматически меняется с нагрузкой и может уменьшаться до
  нуля;
- доступ бывает публичным или только для аутентифицированных principals;
- каждому сервису прикрепляется свой service account;
- файлы внутри контейнера не являются постоянным хранилищем.

Минимальный сервер читает выданный платформой порт:

```go
func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    mux := http.NewServeMux()
    mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    server := &http.Server{
        Addr:              ":" + port,
        Handler:           mux,
        ReadHeaderTimeout: 5 * time.Second,
    }
    if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatal(err)
    }
}
```

Из каталога приложения его можно собрать и развернуть одной командой:

```bash
gcloud run deploy orders-api \
  --source . \
  --region europe-west1 \
  --service-account orders-api@orders-dev.iam.gserviceaccount.com \
  --allow-unauthenticated
```

Флаг `--allow-unauthenticated` открывает вызов любому клиенту. Для внутреннего
сервиса его не добавляют и выдают `roles/run.invoker` только вызывающему service
account. Публичный endpoint и сетевой ingress — связанные, но разные границы:
настройка ingress определяет, откуда трафик вообще принимается, а IAM — кто имеет
право вызвать сервис.

Cloud Run не выбирают как постоянное локальное хранилище, обычный Kubernetes
cluster или VM с полным контролем ОС. Масштабирование создаёт несколько
экземпляров, поэтому in-memory session, локальный cron и запись важных файлов на
диск делают поведение зависимым от случайного экземпляра.

### Cloud Run job и Cloud Run functions

**Cloud Run job** запускает одну или несколько задач, которые выполняют работу и
завершаются. Он подходит для миграций, периодических отчётов, импорта или
параллельной обработки набора объектов. Расписание обычно создаёт Cloud Scheduler,
а сложную последовательность шагов — Workflows.

**Cloud Run functions** запускает функцию по HTTP или событию. Этот вариант
удобен для небольшого обработчика изменения объекта, Pub/Sub message или webhook.
Если приложение уже имеет нормальный HTTP server, несколько маршрутов и общую
инициализацию, обычный Cloud Run service часто яснее.

### Compute Engine

Compute Engine предоставляет виртуальные машины. Команда выбирает образ ОС,
machine type, диски, сеть, обновления и способ масштабирования.

Compute Engine нужен, когда нагрузка требует:

- конкретную ОС, kernel-настройки, системные пакеты или privileged access;
- произвольный сетевой протокол или долгоживущий daemon;
- legacy software, которое трудно упаковать под serverless-контракт;
- специальное оборудование либо точный выбор CPU, памяти и GPU;
- self-hosted базу данных или broker, если команда готова ими управлять.

Одиночная VM остаётся одиночной точкой отказа. Для одинаковых stateless VM
используют Managed Instance Group (MIG): instance template, health check,
autohealing, autoscaling и при необходимости размещение по нескольким зонам.
Spot VM дешевле стандартной, но платформа может забрать её в любой момент, поэтому
она подходит только прерываемой работе с checkpoint или повторным запуском.

### Google Kubernetes Engine

Google Kubernetes Engine (GKE) — managed Kubernetes. Google управляет control
plane, но степень ответственности за worker nodes зависит от режима.

| Режим | Что контролирует команда | Когда выбирать |
| --- | --- | --- |
| Autopilot | приложения и Kubernetes-ресурсы; nodes в основном управляет GKE | Kubernetes действительно нужен, но отдельную node platform поддерживать не хочется |
| Standard | node pools, machine types, scaling и больше низкоуровневых настроек | нужны приложения с привилегиями, специальные nodes, точный контроль цены или сети |

GKE оправдан, если уже нужны Kubernetes API, operators, sidecars, DaemonSets,
network policies, общий platform layer или переносимость существующих manifests.
Для одного stateless API он обычно добавляет cluster upgrades, capacity planning,
RBAC, networking и observability без полезной бизнес-функции.

### Как выбрать compute

```text
Обычный HTTP/gRPC контейнер?
├── Да → Cloud Run
│   └── Нужен именно Kubernetes API или operators? → GKE
└── Нет
    ├── Задача выполняется и завершается? → Cloud Run Jobs или Batch
    └── Нужен контроль ОС/протокола/daemon? → Compute Engine
```

App Engine остаётся полноценной платформой приложений и встречается в существующих
системах. Для нового контейнеризированного Go backend сначала сравнивают Cloud Run
и GKE: у них яснее путь от container image к runtime и меньше специфичного для
App Engine контракта.

---

## Где хранить данные

### Cloud Storage

Cloud Storage — объектное хранилище (`object storage`): bucket содержит objects,
доступные по имени.
Сервис подходит для изображений, видео, архивов, backups, выгрузок и статических
файлов. Это не обычная файловая система и не база для частых частичных изменений
одного файла.

При создании bucket выбирают:

- location: region, dual-region или multi-region;
- класс хранения (`storage class`) по частоте чтения;
- IAM и запрет публичного доступа;
- lifecycle rules для перехода в более дешёвый класс или удаления;
- versioning, soft delete и retention policy по требованиям восстановления.

Рабочий путь для загрузки пользовательского файла обычно выглядит так:

```text
1. Клиент просит backend начать upload.
2. Backend проверяет пользователя и создаёт уникальное object name.
3. Backend возвращает ограниченный по времени signed URL.
4. Клиент загружает байты напрямую в Cloud Storage.
5. Backend сохраняет object name и metadata в базе.
```

Так большие байты не проходят через Go API. В базе хранится идентификатор объекта,
а не постоянный signed URL: срок действия URL ограничен, и новый URL можно выдать
после повторной проверки доступа.

Для служебной загрузки приложение использует attached service account и ADC:

```go
func putObject(
    ctx context.Context,
    client *storage.Client,
    bucket string,
    object string,
    data []byte,
) error {
    writeCtx, cancel := context.WithCancel(ctx)
    defer cancel()

    writer := client.Bucket(bucket).Object(object).NewWriter(writeCtx)
    writer.ContentType = "application/octet-stream"

    if _, err := writer.Write(data); err != nil {
        return fmt.Errorf("write object: %w", err)
    }
    if err := writer.Close(); err != nil {
        return fmt.Errorf("commit object: %w", err)
    }
    return nil
}
```

`storage.NewClient(ctx)` сам находит ADC. Передавать путь к JSON key в коде не
нужно.

### Cloud SQL

Cloud SQL — managed PostgreSQL, MySQL и SQL Server. Он подходит для большинства
backend-систем с транзакциями, joins, constraints и привычными SQL-инструментами.
Google управляет VM и обслуживанием движка базы данных, но schema, indexes, запросы,
connection pools и план восстановления остаются ответственностью команды.

Для production обычно явно решают:

- нужен ли regional HA для переживания zonal failure;
- какие backups и point-in-time recovery требуются по RPO/RTO;
- нужен ли private IP;
- как Cloud Run, GKE или VM устанавливает защищённое соединение;
- сколько соединений разрешено приложению и его репликам;
- нужны ли read replicas для read scaling или disaster recovery.

HA standby и read replica решают разные задачи. Standby принимает failover и не
является обычным endpoint для чтения. Read replica разгружает чтения, но её lag
нужно учитывать, а cross-region replica сама по себе не делает переключение
приложения полностью автоматическим.

Для Cloud Run удобны Cloud SQL Language Connector или интеграция через Unix
socket. Private IP даёт более явный сетевой путь через VPC. В обоих случаях
приложению всё равно нужны пользователь/пароль базы данных или IAM database
authentication, а размер connection pool надо умножать на максимальное число
экземпляров приложения.

### AlloyDB for PostgreSQL

AlloyDB — управляемая PostgreSQL-compatible база данных для более требовательных
transactional и смешанных transactional/analytical нагрузок. Приложение
подключается стандартным PostgreSQL driver, а чтения можно вынести в отдельные
read pool instances.

Его рассматривают, когда PostgreSQL compatibility обязательна, но Cloud SQL уже
не закрывает требования по throughput, read scaling или availability. Выбор
должен опираться на benchmark конкретных запросов и расчёт стоимости: перенос
обычного небольшого CRUD backend в более мощный продукт сам по себе не исправит
плохую schema, отсутствующие indexes или неограниченный connection pool.

### Firestore

Firestore — документная база данных (`document database`). Данные лежат в
documents, объединённых в
collections. Сервис полезен, когда приложение читает небольшие документы по ключу
или заранее известным индексируемым запросам, хочет serverless scaling и не
нуждается в реляционных joins.

Модель проектируют от запросов:

- document должен иметь ограниченный и предсказуемый размер;
- нужные compound indexes создают заранее;
- fan-out reads и большое число мелких операций влияют и на latency, и на счёт;
- транзакции существуют, но Firestore не становится от этого заменой PostgreSQL
  для сложной реляционной модели.

### Bigtable

Bigtable — wide-column база данных для огромных объёмов single-keyed данных с
высокой пропускной способностью и низкой latency. Она подходит для временных рядов,
телеметрии, истории цен и других нагрузок, где чтения начинаются с хорошо
спроектированного row key.

Bigtable поддерживает GoogleSQL для `SELECT`, но не реляционные joins, обычные
SQL `INSERT/UPDATE/DELETE` и транзакции между несколькими rows. Плохой row key
создаёт hotspot или заставляет сканировать слишком широкий диапазон. Если нужен
документ с индексами — сначала смотрят на Firestore; если аналитика по колонкам —
на BigQuery; если multi-row транзакции и relations — на SQL базу данных.

### Spanner

Spanner — управляемая реляционная база данных с SQL, ACID-транзакциями, сильной
согласованностью и горизонтальным масштабированием. Его рассматривают, когда одна
обычная SQL instance перестаёт удовлетворять требованиям по масштабу или
географической доступности, а отказаться от транзакционной SQL-модели нельзя.

Spanner не является автоматическим «Cloud SQL, только лучше». У него другая модель
capacity и стоимости, schema требует учитывать распределение ключей и hotspots,
а локальной команде нужны причины оправдать дополнительную сложность. Для
большинства CRUD backend разумная отправная точка — Cloud SQL.

### BigQuery

BigQuery — serverless data warehouse для аналитических запросов по большим
наборам данных. Он хорошо выполняет сканирование, агрегации, отчёты и ad-hoc SQL,
но не заменяет OLTP-базу для коротких пользовательских транзакций.

Практический поток:

```text
OLTP database / events / object files
                 │
                 ▼
        ingestion or scheduled load
                 │
                 ▼
       BigQuery tables → SQL analytics → dashboard/report
```

Стоимость on-demand запросов зависит от объёма прочитанных данных. Partitioning,
clustering, выбор только нужных колонок и ограничения на ad-hoc запросы — часть
cost design, а не поздняя оптимизация.

### Memorystore

Memorystore предоставляет managed Valkey и Redis-варианты для cache, sessions,
rate limiting и временного быстрого состояния. Инстансы обычно доступны по private
IP из разрешённой VPC, поэтому подключение — одновременно задача работы с базой
данных и сетью.

Cache не делают единственным источником критичных данных только потому, что сервис
managed. Нужно заранее определить поведение при cache miss, eviction, failover и
полной недоступности.

### Как выбрать хранилище

| Access pattern | Кандидат |
| --- | --- |
| `PUT/GET` большого объекта по имени | Cloud Storage |
| транзакции и joins в обычном масштабе | Cloud SQL |
| PostgreSQL compatibility для требовательной нагрузки | AlloyDB |
| небольшие JSON-подобные документы по ключу и индексам | Firestore |
| огромная single-keyed wide-column нагрузка | Bigtable |
| огромная горизонтальная transactional SQL нагрузка | Spanner |
| аналитические сканы и агрегации | BigQuery |
| временное состояние с очень низкой latency | Memorystore |

Выбор по формату данных недостаточен. JSON можно положить почти в любой продукт;
важны операции чтения и записи, транзакционные границы, объём, latency, рост и
способ восстановления.

---

## Асинхронная работа и интеграции

### Pub/Sub

Pub/Sub — managed publish/subscribe service. Publisher отправляет message в topic,
а каждая subscription получает свою копию. Это подходит для domain events,
integration events, ingest pipeline и fan-out нескольким независимым системам.

По умолчанию доставка at-least-once и без гарантии порядка. Обработчик должен быть
идемпотентным. Ordering включают по ordering key, а exactly-once имеет отдельные
условия и не превращает побочный эффект во внешней БД или API в exactly-once.

```text
orders-api → topic orders
                 ├── subscription billing → billing-worker
                 ├── subscription email   → notification-worker
                 └── subscription data    → analytics pipeline
```

Pull subscription подходит постоянному worker, который сам регулирует чтение.
Push subscription вызывает HTTPS endpoint и хорошо сочетается с Cloud Run.
Подробная механика, Go client, ordering, ack deadline и DLQ разобраны в
[06-cloud-pubsub.md](../../07-message-brokers-and-streaming/06-cloud-pubsub.md).

### Cloud Tasks

Cloud Tasks хранит адресную команду и вызывает один HTTP handler с настроенными
retry, rate limit и временем запуска. Сервис удобен для отправки письма, повторного
вызова партнёра, генерации документа или сглаживания нагрузки на downstream.

| Вопрос | Pub/Sub | Cloud Tasks |
| --- | --- | --- |
| Семантика | событие для одной или многих subscriptions | команда конкретному HTTP target |
| Получатели | независимый fan-out | один handler на задачу |
| Регулирование нагрузки | flow control подписчика | rate и concurrency queue |
| Отложенный запуск | не основной сценарий | задаётся для отдельной task |
| Результат | ack/nack сообщения | HTTP 2xx завершает задачу |

Cloud Tasks доставляет at-least-once, поэтому HTTP handler остаётся идемпотентным.
Ответ `2xx` означает успешное завершение, а timeout или error запускает retry по
политике очереди. Большой payload лучше сохранить в Cloud Storage или базе, а в
task передать идентификатор.

### Cloud Scheduler, Workflows и Eventarc

- **Cloud Scheduler** — managed cron, вызывающий HTTP endpoint или публикующий в
  Pub/Sub. Доставка at-least-once, следовательно scheduled handler тоже должен
  переживать повторный запуск.
- **Workflows** — оркестрация нескольких HTTP и Google Cloud API calls с явными
  шагами, состоянием, ожиданием и retry. Подходит для business/operational flow,
  который важно видеть целиком.
- **Eventarc** — маршрутизация событий Google Cloud и пользовательских приложений
  к Cloud Run и другим поддерживаемым targets. Он удобен как слой событийного
  связывания, но не заменяет явную business orchestration в Workflows.

Выбор между Workflows и кодом зависит не от числа строк YAML. Оркестратор полезен,
когда шаги долгие, вызывают разные системы, должны независимо повторяться и
нуждаются в видимом состоянии. Короткую атомарную операцию яснее оставить внутри
сервиса.

---

## Сеть и внешний трафик

### VPC и subnets

VPC в Google Cloud — глобальный ресурс, а subnet — региональный. Это важное
отличие от модели AWS: одна VPC может содержать subnets из разных регионов без
создания отдельной VPC на каждый регион.

Для production Google рекомендует custom mode VPC, где команда сама задаёт
регионы и CIDR ranges. Автоматически созданная default network удобна для первого
эксперимента, но её готовые subnets и firewall rules редко соответствуют
production design.

```text
Global VPC: prod
├── subnet europe-west1: 10.10.0.0/20
│   ├── GKE nodes
│   └── internal load balancer
└── subnet europe-west4: 10.20.0.0/20
    └── Compute Engine MIG
```

VPC firewall rules управляют ingress и egress для VM-based ресурсов. IAM отвечает
на вопрос «кто может изменить правило или вызвать API», а firewall — «какой
сетевой пакет может пройти». Одно не заменяет другое.

### Исходящий доступ и private connectivity

- **Cloud NAT** даёт исходящий интернет-доступ ресурсам без external IPv4 и не
  принимает входящие соединения из интернета.
- **Private Google Access** позволяет ресурсам без external IP обращаться к
  поддерживаемым Google APIs.
- **Private Service Connect** публикует или потребляет сервис через private
  endpoint без общего публичного маршрута.
- **Cloud VPN** соединяет VPC с on-premises или другой сетью через VPN.
- **Cloud Interconnect** даёт выделенное подключение для требований по полосе и
  предсказуемости.

Не каждый managed-сервис физически «находится внутри VPC». Часто VPC даёт путь к
его private endpoint или сервисной интеграции. Поэтому при отладке надо отдельно
проверять DNS, route, firewall, способ serverless VPC egress и IAM.

### Load Balancing, DNS, CDN и защита

| Сервис | Роль |
| --- | --- |
| Cloud Load Balancing | L7 Application Load Balancer для HTTP(S) или L4 Network Load Balancer для TCP/UDP-сценариев |
| Cloud DNS | managed public и private DNS zones |
| Cloud CDN | cache статического и cacheable HTTP content ближе к клиенту |
| Cloud Armor | DDoS/WAF policies перед поддерживаемыми внешними backends |
| Certificate Manager | управление TLS certificates для поддерживаемых endpoints |

Для одного публичного Cloud Run API встроенного `run.app` endpoint часто
достаточно. Global external Application Load Balancer добавляют, когда нужны
единый custom domain, несколько backends, path routing, Cloud CDN, Cloud Armor или
глобальная схема трафика.

Application Load Balancer работает на L7 и понимает HTTP(S). Network Load Balancer
нужен для L4 traffic, TLS proxy или протоколов, которым HTTP routing не подходит.
Выбор начинается с протокола, затем с external/internal доступа и только потом с
global/regional scope.

---

## IAM, аутентификация и секреты

### Principal, role и resource

IAM-назначение читается как предложение:

```text
principal orders-api@... получает role storage.objectViewer на bucket invoices
```

- **Principal** — пользователь, группа, service account или federated identity.
- **Role** — набор прав (`permissions`).
- **Resource** — объект, project, folder или organization, на котором выдана роль.

Basic roles `Owner`, `Editor` и `Viewer` слишком широки для runtime. Для приложения
выбирают predefined role минимального размера, а custom role создают только когда
предопределённые роли стабильно дают лишние критичные права.

### Service accounts

Service account — identity приложения, а не файл с JSON key. Рекомендуемый путь:

1. Создать отдельный service account для приложения.
2. Выдать ему минимальные роли на конкретных ресурсах.
3. Прикрепить account к Cloud Run service или VM; для GKE назначить identity через
   Workload Identity Federation for GKE.
4. Позволить библиотекам получить краткоживущие credentials через ADC.

Один общий service account на все микросервисы увеличивает blast radius. Если
orders API умеет читать invoices bucket, а notification worker не должен, им нужны
разные identities.

Для внешнего CI/CD используют Workload Identity Federation. GitHub Actions или
GitLab CI обменивает OIDC token на краткоживущие Google credentials без постоянного
JSON key. Для локальной проверки прав полезна service account impersonation.

### Application Default Credentials

Google Cloud client libraries ищут ADC в установленном порядке:

1. Путь из `GOOGLE_APPLICATION_CREDENTIALS`.
2. Локальный ADC-файл, созданный `gcloud auth application-default login`.
3. Attached service account через metadata service среды выполнения.

Это порядок поиска, а не рейтинг безопасности. В production предпочтителен
attached identity; путь к постоянному JSON key нужен только для ограниченных
legacy-сценариев, когда federation или прикреплённый account недоступны.

Код при этом одинаков локально и в Cloud Run:

```go
func newStorageClient(ctx context.Context) (*storage.Client, error) {
    client, err := storage.NewClient(ctx)
    if err != nil {
        return nil, fmt.Errorf("create storage client: %w", err)
    }
    return client, nil
}
```

В коде нет project secret, private key или ветвления `if production`.

### Secret Manager и Cloud KMS

Secret Manager хранит API keys, passwords, certificates и другие небольшие
секреты как версии. IAM определяет, какое приложение может прочитать конкретный
secret, а audit logs помогают расследовать доступ.

Секрет можно:

- подставить в Cloud Run как environment variable или mounted file;
- получить через client library при старте;
- читать по конкретной версии для контролируемого rollout;
- читать как `latest`, если приложение умеет безопасно переживать ротацию.

Cloud Key Management Service (Cloud KMS) хранит и применяет encryption keys. Он
нужен для customer-managed encryption keys, подписи или шифрования, но не заменяет
Secret Manager: KMS key — ключ криптографической операции, secret — значение,
которое приложение должно получить.

Подробные способы доставки секретов разобраны в
[01-secrets-delivery-options.md](../../11-security/secrets-management/01-secrets-delivery-options.md).

---

## Сборка и доставка

### Artifact Registry

Artifact Registry хранит container images и пакеты. Repository имеет формат и
location. Для production image тег удобен человеку, но деплой по digest точнее:
digest однозначно указывает байты и не меняется при переносе тега.

```bash
gcloud artifacts repositories create apps \
  --repository-format docker \
  --location europe-west1

gcloud builds submit \
  --tag europe-west1-docker.pkg.dev/orders-prod/apps/orders-api:COMMIT_SHA

gcloud run deploy orders-api \
  --image europe-west1-docker.pkg.dev/orders-prod/apps/orders-api:COMMIT_SHA \
  --region europe-west1
```

`COMMIT_SHA` здесь placeholder, который CI заменяет фактическим commit SHA. Один
и тот же проверенный image продвигают между окружениями, а не пересобирают из
исходников отдельно для staging и production.

### Cloud Build и Cloud Deploy

Cloud Build выполняет build steps в контейнерах: тестирует код, собирает image и
публикует artifact. Триггер может запускаться от изменения репозитория, но сам
build pipeline не должен автоматически получать широкие production-права.

Cloud Deploy управляет продвижением releases в GKE и Cloud Run targets, rollout и
approval stages. Он нужен, когда отдельная delivery pipeline даёт полезный
контроль. Для небольшой команды достаточно существующего CI, Artifact Registry и
`gcloud run deploy` с Workload Identity Federation.

Разделение ответственности выглядит так:

```text
Terraform       → projects, IAM, networks, базы данных, queues
CI / Cloud Build → tests, binary, container image
CD / Cloud Deploy → revision rollout and promotion
Приложение       → business data and messages
```

Terraform не должен управлять каждым новым image tag, если deployment выполняет
CI/CD: иначе два инструмента начинают откатывать изменения друг друга.

---

## Наблюдаемость

Google Cloud Observability объединяет несколько продуктов:

| Сигнал | Основной сервис | Практическое использование |
| --- | --- | --- |
| Logs | Cloud Logging | поиск ошибок, structured fields, log-based metrics, sinks |
| Metrics | Cloud Monitoring | dashboards, SLI, alerts, uptime checks |
| Traces | Cloud Trace / OpenTelemetry | путь запроса и latency между сервисами |
| Profiles | Cloud Profiler | CPU- и memory-профили поддерживаемых сред выполнения |
| Audit | Cloud Audit Logs | кто и какой control-plane/data access вызов сделал |

Cloud Run автоматически отправляет `stdout` и `stderr` в Cloud Logging. Для
поиска лучше писать structured JSON с постоянными полями `severity`, `service`,
`request_id`, `trace_id`, `operation` и `error`, а не собирать смысл из строки
регулярным выражением.

Наблюдаемость не заканчивается фактом появления логов. До production нужны:

- dashboard по request rate, error rate, latency и saturation;
- alerts с понятным условием и каналом уведомления;
- метрики backlog для Pub/Sub/Cloud Tasks workers;
- контроль connection pool и насыщения базы данных;
- retention и sinks для логов с учётом требований и стоимости;
- корреляция logs и traces через trace context.

Cloud Monitoring автоматически собирает метрики многих GCP-сервисов. Для
метрик приложения можно использовать OpenTelemetry или Managed Service for
Prometheus. Подробный путь логов в Google Cloud разобран в
[11-cloud-log-delivery-aws-and-google-cloud.md](../logging-and-log-shipping/11-cloud-log-delivery-aws-and-google-cloud.md).

---

## Три типовые архитектуры

### Небольшой stateless API

```text
Client
  │ HTTPS
  ▼
Cloud Run service ─────→ Cloud SQL (business data)
  │        │
  │        ├───────────→ Cloud Storage (files)
  │        └───────────→ Cloud Tasks (background commands)
  │
  ├── identity: service account + IAM
  ├── secrets: Secret Manager
  └── signals: Cloud Logging + Cloud Monitoring
```

Это хороший старт для небольшой команды: нет cluster и VM fleet, но остаются
явные решения по HA базы данных, connection pool, idempotency, IAM и alerts.

### Event-driven backend

```text
Producer → Pub/Sub topic
              ├── billing subscription → Cloud Run / GKE worker → Cloud SQL
              ├── analytics subscription → Dataflow or worker → BigQuery
              └── audit subscription → Cloud Storage
```

Каждый consumer имеет собственную subscription и service account. Медленный
analytics consumer не удерживает billing consumer, но backlog, retry и DLQ каждой
подписки наблюдаются отдельно.

### Kubernetes platform

```text
Global Application Load Balancer + Cloud Armor
                       │
                       ▼
                 GKE Autopilot/Standard
                  ├── stateless APIs
                  ├── queue consumers
                  └── internal services
                       │
          ┌────────────┼────────────┐
          ▼            ▼            ▼
      Cloud SQL    Memorystore    Pub/Sub
```

GKE не требует переносить в cluster все зависимости. Управляемая база данных, cache
и broker обычно уменьшают операционную нагрузку. Stateful-приложение в Kubernetes
выбирают осознанно, когда контроль или переносимость важнее managed-варианта.

---

## Соответствие сервисов AWS и GCP

Соответствие приблизительное: продукты похожи по роли, но отличаются API,
гарантиями, placement model и pricing.

| AWS | Google Cloud | Важное отличие |
| --- | --- | --- |
| AWS Account | Project | GCP project — граница ресурсов и квот; над ним могут быть folder и organization |
| EC2 | Compute Engine | обе платформы дают VM и managed groups |
| ECS/Fargate | Cloud Run или GKE Autopilot | Cloud Run ближе к serverless container; Autopilot сохраняет Kubernetes API |
| EKS | GKE | в GKE есть Autopilot и Standard modes |
| Lambda | Cloud Run functions | для целого контейнерного API в GCP часто выбирают Cloud Run service |
| S3 | Cloud Storage | bucket/object model похожа, детали IAM, signing и locations отличаются |
| RDS | Cloud SQL | managed engines и HA похожи по назначению, конфигурация failover отличается |
| Aurora | AlloyDB или Spanner по задаче | AlloyDB PostgreSQL-compatible; Spanner — отдельная distributed SQL model |
| DynamoDB | Firestore, Bigtable или Spanner | выбор зависит от document, wide-column и transactional access patterns |
| ElastiCache | Memorystore | доступны managed Valkey/Redis-варианты |
| SNS | Pub/Sub topic | Pub/Sub subscription сама хранит позицию и доставку consumer |
| SQS | Pub/Sub subscription или Cloud Tasks | Pub/Sub — messaging/fan-out; Cloud Tasks — адресные HTTP-команды |
| EventBridge | Eventarc и Pub/Sub | Eventarc маршрутизирует события, Pub/Sub хранит и доставляет messages |
| Step Functions | Workflows | declarative orchestration внешних шагов |
| VPC | VPC | GCP VPC глобальна, subnets региональны |
| Security Groups | VPC firewall rules | модель назначения и приоритетов не идентична |
| NAT Gateway | Cloud NAT | оба дают managed outbound NAT без inbound path |
| ALB/NLB | Application/Network Load Balancer | в GCP выбор также делается по external/internal и global/regional scope |
| Route 53 | Cloud DNS | managed authoritative DNS |
| CloudFront | Cloud CDN | в GCP CDN подключается к поддерживаемому load balancer backend |
| AWS WAF/Shield | Cloud Armor | edge security и WAF policies |
| IAM Role для приложения | Service account + IAM roles | GCP runtime обычно работает как прикреплённый service account |
| Secrets Manager | Secret Manager | versioned secrets с IAM и audit |
| KMS | Cloud KMS | управление cryptographic keys |
| ECR | Artifact Registry | Artifact Registry хранит images и другие package formats |
| CodeBuild | Cloud Build | managed build execution |
| CloudWatch | Cloud Logging + Cloud Monitoring | logs и metrics представлены отдельными сервисами одного observability stack |

Таблица полезна для ориентации, но миграцию нельзя проектировать механической
заменой названий. Например, SNS+SQS часто собираются в пару вручную, а в Pub/Sub
topic и независимые subscriptions уже являются основной моделью.

---

## Стоимость и контроль расходов

В GCP нет единой модели оплаты: Cloud Run считает потреблённые runtime resources,
Cloud SQL и Memorystore держат выделенную capacity, Cloud Storage учитывает объём,
operations и transfer, а BigQuery может тарифицировать обработанные query data или
зарезервированную capacity.

Главные источники неожиданного счёта:

- network egress в интернет и между locations;
- постоянно работающие Cloud SQL, Memorystore, GKE nodes и VM;
- завышенные CPU/memory limits и minimum instances;
- debug logs с большим объёмом ingestion и retention;
- BigQuery queries, сканирующие таблицу без partition filter;
- забытые disks, snapshots, static IP и тестовые окружения;
- Pub/Sub fan-out, где одна публикация превращается в несколько доставок.

Billing budget отправляет alerts, но сам по себе не является жёстким лимитом и не
останавливает ресурсы. Автоматическое отключение billing опасно для production и
требует отдельного контролируемого процесса.

Практический минимум:

1. Создать budget и несколько порогов уведомления до запуска приложения.
2. Разделить хотя бы production и non-production по projects.
3. Назначить labels для service, environment, team и cost center.
4. Сопоставить регионы compute и data.
5. Поставить ограничения autoscaling там, где это допустимо.
6. Проверять billing export и cost breakdown регулярно, а не после инцидента.
7. Перед архитектурным решением считать steady state, peak и failure mode.

Цены меняются и зависят от региона, поэтому конкретный расчёт делают через
[Google Cloud Pricing Calculator](https://cloud.google.com/products/calculator) и
официальные pricing pages выбранных продуктов.

---

## Типичные ошибки

### Один project и один service account для всего

Так проще начать, но blast radius растёт вместе с системой. Ошибка в IAM одного
worker открывает данные других сервисов, а quotas и расходы трудно отнести к
окружению. Production, staging и shared infrastructure разделяют осознанными
границами проектов и identities.

### Постоянные JSON keys

Service account key легко утечь в repository, CI logs или backup. В GCP runtime
используют attached service account, во внешнем CI — Workload Identity Federation,
локально — ADC или impersonation.

### `Owner` или `Editor` вместо минимальных ролей

Работающий deploy не доказывает корректность IAM. Runtime получает только операции,
которые нужны ему после запуска. Права deployer и права приложения — разные наборы.

### Несогласованные регионы

Cloud Run, базу данных, bucket, queue и analytics dataset создают независимо, поэтому
случайно разнести их легко. Это проявляется как лишняя latency, egress cost или
нарушение data residency уже после запуска.

### Локальное состояние в Cloud Run

Файл или in-memory map принадлежит одному disposable instance. После scale-out
другой запрос попадёт на другой instance, а после restart данные исчезнут. Важное
состояние выносят в базу данных, объектное хранилище или cache с подходящими
гарантиями.

### Cloud SQL без расчёта соединений

Если каждый из 100 Cloud Run instances открывает pool по 20 connections,
теоретический максимум равен `100 × 20 = 2000` connections. Autoscaling приложения
без pool limit может исчерпать лимит базы данных раньше CPU.

### Pub/Sub consumer без идемпотентности

At-least-once delivery допускает повтор. Сначала проектируют idempotency key,
условную запись или inbox/outbox pattern, затем подтверждают message после
успешного эффекта.

### BigQuery как основная база данных

BigQuery оптимизирован под аналитику, а не под короткую OLTP-транзакцию на каждый
HTTP request. Online state остаётся в Cloud SQL, Spanner или другом operational
хранилище, а в BigQuery попадает аналитическая копия.

### Budget как hard limit

Budget notification сообщает о расходе, но не гарантирует остановку. Защитные
меры строят из quotas, autoscaling bounds, IAM, anomaly detection и процесса
реагирования.

---

## Практический чек-лист

### До создания ресурсов

- [ ] Определены project boundaries для production и non-production.
- [ ] Выбран основной region с учётом пользователей, данных и compliance.
- [ ] Посчитаны steady-state и peak capacity, а также network egress.
- [ ] Созданы budget alerts и owners расходов.

### Для приложения

- [ ] Выбран минимально сложный compute: Cloud Run до появления причины для GKE
  или VM.
- [ ] У приложения отдельный service account с минимальными ролями.
- [ ] В коде используются ADC, а постоянные JSON keys отсутствуют.
- [ ] Секреты находятся в Secret Manager и не печатаются в logs.
- [ ] Долговременное состояние не хранится на локальном диске serverless runtime.

### Для данных и messaging

- [ ] Хранилище выбрано по access pattern, а не по названию формата.
- [ ] Настроены backups, PITR/retention и проверен restore path.
- [ ] Connection pool согласован с максимальным числом instances.
- [ ] Consumers и scheduled handlers идемпотентны.
- [ ] Для очередей наблюдаются backlog, oldest message и retry/DLQ.

### Для production

- [ ] Есть dashboards, alerts, structured logs и trace correlation.
- [ ] Deployment создаёт immutable artifact и управляемый rollout.
- [ ] Публичный ingress, IAM invoker и firewall rules проверены отдельно.
- [ ] Terraform владеет инфраструктурой, а CI/CD — версиями приложения.
- [ ] Зафиксированы RPO, RTO и действия при zonal/region failure.

---

## Interview-ready answer

**1. Как выбирать между Cloud Run, GKE и Compute Engine?**

- Отправная точка — Cloud Run для stateless HTTP/gRPC container без управления cluster.
- Kubernetes — GKE нужен при зависимости от Kubernetes API, operators, sidecars или общей platform model.
- Полный контроль — Compute Engine нужен для ОС, произвольных daemons, legacy software и специальных VM requirements.
- Цена выбора — чем ниже уровень абстракции, тем больше контроля и операционной ответственности получает команда.

**2. Как приложение безопасно обращается к GCP APIs?**

- Identity — приложение работает под отдельным service account.
- Authorization — IAM выдаёт этому account минимальные роли на конкретных resources.
- Credentials — клиентская библиотека использует ADC и получает краткоживущие credentials среды.
- Внешняя среда — CI/CD использует Workload Identity Federation, а не постоянный JSON key.

**3. Чем Pub/Sub отличается от Cloud Tasks?**

- Pub/Sub — событие публикуется в topic и независимо доставляется каждой subscription.
- Cloud Tasks — команда адресуется одному HTTP handler и управляет retry, rate и временем запуска.
- Общая граница — обе модели допускают повторную доставку, поэтому обработчик должен быть идемпотентным.

**4. Как выбирать основные сервисы баз данных GCP?**

- Cloud SQL — обычная реляционная OLTP-модель с joins и транзакциями.
- AlloyDB — PostgreSQL-compatible вариант для более требовательной transactional или HTAP нагрузки.
- Firestore — документы с заранее известными key/index access patterns.
- Bigtable — огромная single-keyed wide-column нагрузка с высокой пропускной способностью.
- Spanner — горизонтально масштабируемый transactional SQL, когда обычной instance недостаточно.
- BigQuery — аналитические сканы и агрегации, а не основное хранилище HTTP-приложения.

**5. Что важно знать про сеть GCP?**

- Scope — VPC глобальна, а subnets региональны.
- Доступ — IAM управляет вызовами API, firewall rules управляют сетевым трафиком.
- Публикация — Application Load Balancer используют для HTTP(S), Network Load Balancer для L4 traffic.
- Исходящий путь — Cloud NAT даёт outbound access ресурсам без external IPv4, но не принимает inbound traffic.

**6. Какие cloud-cost ошибки наиболее опасны?**

- Placement — межрегиональный и интернет egress появляется на каждом data flow.
- Idle capacity — VM, GKE nodes, Cloud SQL и Memorystore стоят денег без пользовательского трафика.
- Data processing — неограниченные logs и BigQuery scans растут вместе с объёмом данных.
- Контроль — budget уведомляет, но не является hard spending cap.

---

## Официальная документация

- [Google Cloud products](https://cloud.google.com/products)
- [Resource hierarchy](https://cloud.google.com/resource-manager/docs/cloud-platform-resource-hierarchy)
- [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials)
- [Service accounts](https://cloud.google.com/iam/docs/service-account-overview)
- [Cloud Run](https://cloud.google.com/run/docs/overview/what-is-cloud-run)
- [GKE](https://cloud.google.com/kubernetes-engine/docs/concepts/kubernetes-engine-overview)
- [Compute Engine](https://cloud.google.com/compute/docs/instances)
- [Cloud Storage](https://cloud.google.com/storage/docs/introduction)
- [Cloud SQL](https://cloud.google.com/sql/docs/introduction)
- [AlloyDB](https://cloud.google.com/alloydb/docs/overview)
- [Firestore](https://cloud.google.com/firestore/docs/overview)
- [Bigtable](https://cloud.google.com/bigtable/docs/overview)
- [Spanner](https://cloud.google.com/spanner/docs/overview)
- [BigQuery](https://cloud.google.com/bigquery/docs/introduction)
- [Memorystore](https://cloud.google.com/memorystore/docs)
- [Pub/Sub](https://cloud.google.com/pubsub/docs/overview)
- [Cloud Tasks](https://cloud.google.com/tasks/docs/dual-overview)
- [Cloud Scheduler](https://cloud.google.com/scheduler/docs/overview)
- [Workflows](https://cloud.google.com/workflows/docs/overview)
- [Eventarc](https://cloud.google.com/eventarc/docs/overview)
- [VPC](https://cloud.google.com/vpc/docs/vpc)
- [Cloud Load Balancing](https://cloud.google.com/load-balancing/docs/load-balancing-overview)
- [Cloud DNS](https://cloud.google.com/dns/docs/overview)
- [Cloud CDN](https://cloud.google.com/cdn/docs/overview)
- [Cloud Armor](https://cloud.google.com/armor/docs/cloud-armor-overview)
- [Secret Manager](https://cloud.google.com/secret-manager/docs/overview)
- [Cloud KMS](https://cloud.google.com/kms/docs)
- [Artifact Registry](https://cloud.google.com/artifact-registry/docs/overview)
- [Cloud Build](https://cloud.google.com/build/docs/overview)
- [Cloud Deploy](https://cloud.google.com/deploy/docs/overview)
- [Cloud Logging](https://cloud.google.com/logging/docs/overview)
- [Cloud Monitoring](https://cloud.google.com/monitoring/docs/monitoring-overview)
