# MongoDB: реальные сценарии использования

Companion к [04-mongodb.md](./04-mongodb.md). Здесь — конкретные production-сценарии с обоснованием выбора MongoDB, схемами документов и паттернами, которые обсуждают на интервью.

## Содержание

- [Когда MongoDB реально выигрывает](#когда-mongodb-реально-выигрывает)
- [Сценарий 1: Product catalog с переменными атрибутами](#сценарий-1-product-catalog-с-переменными-атрибутами)
- [Сценарий 2: User profile с flexible extensions](#сценарий-2-user-profile-с-flexible-extensions)
- [Сценарий 3: CMS с составным контентом](#сценарий-3-cms-с-составным-контентом)
- [Сценарий 4: Activity feed и event log](#сценарий-4-activity-feed-и-event-log)
- [Сценарий 5: Multi-tenant конфигурация](#сценарий-5-multi-tenant-конфигурация)
- [Когда кажется, что нужна MongoDB, но это не так](#когда-кажется-что-нужна-mongodb-но-это-не-так)
- [PostgreSQL JSONB vs MongoDB: когда что выбирать](#postgresql-jsonb-vs-mongodb-когда-что-выбирать)
- [Interview-ready answer](#interview-ready-answer)

## Когда MongoDB реально выигрывает

MongoDB выигрывает не потому что "NoSQL быстрее" или "не хочется писать миграции". Она выигрывает в конкретных условиях:

1. **Объект читается и пишется целиком** — вся информация о сущности живет в одном документе, JOIN не нужен.
2. **Атрибуты сущности сильно различаются по типу** — у разных категорий товаров принципиально разные поля, в SQL это JSONB-колонка или EAV-таблица.
3. **Схема активно эволюционирует** — продуктовая разработка идет быстро, структура документа меняется каждую неделю.
4. **Вложенные структуры естественны** — адреса, варианты, спецификации, blocks of content.

## Сценарий 1: Product catalog с переменными атрибутами

**Проблема в SQL**: у телевизора есть `diagonal`, `refresh_rate`, `panel_type`. У книги — `isbn`, `author`, `page_count`. У одежды — `sizes[]`, `material`, `gender`. Попытка хранить всё в одной таблице через EAV (entity-attribute-value) или сотни nullable колонок — антипаттерн.

**Почему MongoDB**: каждый продукт — документ со своей структурой. Общие поля (`name`, `price`, `status`) присутствуют у всех; специфичные — только у нужных категорий.

```javascript
// книга
{
  _id: ObjectId("..."),
  sku: "BOOK-001",
  category: "books",
  name: "Designing Data-Intensive Applications",
  price: 45.00,
  status: "active",
  attrs: {
    isbn: "978-1449373320",
    author: "Martin Kleppmann",
    pages: 616,
    language: "en"
  },
  tags: ["databases", "backend", "distributed-systems"]
}

// телевизор
{
  _id: ObjectId("..."),
  sku: "TV-065",
  category: "electronics",
  name: "Samsung 65 QLED",
  price: 1299.00,
  status: "active",
  attrs: {
    diagonal: 65,
    resolution: "4K",
    panel_type: "QLED",
    refresh_rate: 120,
    hdmi_ports: 4,
    smart_tv: true
  },
  tags: ["samsung", "qled", "4k"]
}
```

**Access patterns**:

```javascript
// поиск по общим полям — один индекс на всё
db.products.createIndex({ category: 1, status: 1, price: 1 })

// поиск по атрибуту категории
db.products.createIndex({ "attrs.author": 1 })

// полнотекстовый поиск по имени
db.products.createIndex({ name: "text" })
```

**Когда это правило нарушается**: если у всех продуктов одинаковые атрибуты или нужны сложные cross-category аналитические запросы — PostgreSQL с JSONB-колонкой покроет задачу с меньшим операционным overhead.

## Сценарий 2: User profile с flexible extensions

**Проблема в SQL**: базовые данные (email, name, created_at) — строгая схема. Но поверх накапливаются: настройки уведомлений, предпочтения, данные onboarding, linked social accounts, payment methods — и каждый feature team добавляет свои поля. Это либо десятки nullable колонок, либо отдельная `user_settings` таблица с JSON.

**Почему MongoDB**: весь профиль в одном документе. Каждый subteam добавляет свой namespace внутри документа без миграций.

```javascript
{
  _id: ObjectId("..."),
  email: "user@example.com",
  name: "Mikhail",
  status: "active",
  created_at: ISODate("2026-01-15T10:00:00Z"),

  notifications: {
    email: true,
    push: false,
    weekly_digest: true
  },

  preferences: {
    language: "ru",
    timezone: "Asia/Tbilisi",
    theme: "dark"
  },

  onboarding: {
    completed_steps: ["profile", "first_order"],
    completed_at: ISODate("2026-01-16T09:00:00Z")
  },

  social_accounts: [
    { provider: "google", provider_id: "1234567890" }
  ],

  plan: {
    type: "pro",
    expires_at: ISODate("2027-01-15T00:00:00Z")
  }
}
```

**Читается одним запросом** — нет JOIN'ов между `users`, `user_notifications`, `user_preferences`, `user_social_accounts`.

**Обновление части документа** — атомарное:

```javascript
// обновить только настройки уведомлений, не трогая остальное
db.users.updateOne(
  { _id: userId },
  { $set: { "notifications.push": true, "notifications.weekly_digest": false } }
)
```

**Граница**: если `plan` требует транзакционных инвариантов с другими коллекциями (billing, payments) — эту часть лучше держать в PostgreSQL.

## Сценарий 3: CMS с составным контентом

**Проблема в SQL**: статья состоит из блоков — текст, изображение, видео, цитата, код. Каждый тип блока имеет разные поля. В SQL это либо таблица `blocks` с десятками nullable колонок, либо type-union через наследование таблиц.

**Почему MongoDB**: документ естественно представляет составной контент.

```javascript
{
  _id: ObjectId("..."),
  slug: "kubernetes-for-backend-developers",
  title: "Kubernetes для backend-разработчика",
  status: "published",
  author_id: ObjectId("..."),
  published_at: ISODate("2026-04-20T10:00:00Z"),
  tags: ["kubernetes", "devops", "backend"],

  blocks: [
    {
      type: "text",
      content: "Главный вопрос: какие проблемы решает Kubernetes..."
    },
    {
      type: "code",
      language: "yaml",
      content: "apiVersion: apps/v1\nkind: Deployment..."
    },
    {
      type: "image",
      url: "https://cdn.example.com/k8s-arch.png",
      caption: "Архитектура Kubernetes кластера",
      alt: "Kubernetes architecture diagram"
    },
    {
      type: "callout",
      variant: "warning",
      text: "Не запускайте production без readiness probe."
    }
  ],

  seo: {
    meta_title: "Kubernetes: практическое руководство",
    meta_description: "...",
    og_image: "https://cdn.example.com/og-k8s.png"
  }
}
```

**Версионирование** — отдельная коллекция `article_versions` с теми же полями. При публикации — snapshot текущего состояния. Не нужны сложные version tables.

**Без MongoDB**: тот же результат достигается PostgreSQL с JSONB-полем `blocks`. Выбор между ними — в остальных операционных требованиях (масштаб, команда, существующая инфраструктура).

## Сценарий 4: Activity feed и event log

**Проблема в SQL**: поток событий с гетерогенными payload. `user_liked_post`, `user_commented`, `user_followed`, `order_placed`, `payment_failed` — у каждого типа свои поля. В PostgreSQL — либо JSONB-поле с payload, либо отдельные таблицы на каждый тип (много join'ов при фиде).

**Почему MongoDB**: события естественно хранятся как документы, каждое со своим payload.

```javascript
// разные типы событий в одной коллекции
{
  _id: ObjectId("..."),
  user_id: ObjectId("user-42"),
  type: "order_placed",
  occurred_at: ISODate("2026-04-20T10:30:00Z"),
  payload: {
    order_id: "ORD-2026-001",
    amount: 150.00,
    items_count: 3
  }
}

{
  _id: ObjectId("..."),
  user_id: ObjectId("user-42"),
  type: "payment_failed",
  occurred_at: ISODate("2026-04-20T10:31:00Z"),
  payload: {
    order_id: "ORD-2026-001",
    error_code: "insufficient_funds",
    gateway: "stripe"
  }
}
```

**Индекс для фида пользователя**:

```javascript
db.events.createIndex({ user_id: 1, occurred_at: -1 })
```

**Альтернатива**: если нужен высокий write throughput и retention по времени — Cassandra с time bucket partition key или ClickHouse для аналитики по событиям. MongoDB подходит для умеренного объема с гибкими queries.

## Сценарий 5: Multi-tenant конфигурация

**Проблема в SQL**: разные tenants имеют принципиально разную конфигурацию — разные feature sets, разные настройки интеграций, разные branding параметры. В SQL это `tenant_config` таблица с JSONB-колонкой или сотни nullable колонок.

**Почему MongoDB**: конфигурация каждого tenant — документ. Нет ограничений на структуру.

```javascript
{
  _id: ObjectId("..."),
  tenant_id: "tenant-acme",
  plan: "enterprise",

  features: {
    sso: { enabled: true, provider: "okta", entity_id: "https://acme.okta.com" },
    audit_log: { enabled: true, retention_days: 365 },
    custom_domain: { enabled: true, domain: "app.acme.com" }
  },

  integrations: {
    slack: { webhook_url: "https://hooks.slack.com/...", channel: "#alerts" },
    jira: { base_url: "https://acme.atlassian.net", project_key: "OPS" }
  },

  branding: {
    logo_url: "https://cdn.example.com/acme-logo.png",
    primary_color: "#0052CC",
    support_email: "support@acme.com"
  }
}
```

Читается одним запросом по `tenant_id`. Новый тип интеграции добавляется без миграции схемы.

## Когда кажется, что нужна MongoDB, но это не так

**"Не хочу писать SQL миграции"** — плохая причина. MongoDB тоже требует schema discipline: без контроля получается коллекция с документами в 10 разных форматах.

**"У нас JSON в API, значит MongoDB"** — не аргумент. PostgreSQL JSONB хранит и индексирует JSON не хуже.

**"Нам нужна гибкость"** — если через месяц добавится нужда в JOIN'ах между сущностями, это будет боль. Гибкость документной модели — не про relationships, а про атрибуты одной сущности.

**Orders, payments, inventory** — transactional инварианты (баланс не может стать отрицательным, заказ не может быть оплачен дважды) требуют строгих транзакций. MongoDB multi-document transactions есть, но они медленнее и mental model PostgreSQL здесь естественнее.

## PostgreSQL JSONB vs MongoDB: когда что выбирать

| Ситуация | Что выбрать |
|---|---|
| Часть данных flexible, часть relational | PostgreSQL + JSONB колонка |
| Весь домен document-centric, без relational части | MongoDB |
| Нужны транзакции между несколькими сущностями | PostgreSQL |
| Схема меняется очень быстро, relational constraints не нужны | MongoDB |
| Команда хорошо знает SQL, новый проект с нуля | PostgreSQL (меньше риск) |
| Уже есть MongoDB инфраструктура | MongoDB (операционный аргумент) |

PostgreSQL с JSONB покрывает большинство сценариев "flexible schema внутри одной сущности" без необходимости в отдельной MongoDB. MongoDB становится оправданным выбором, когда вся или большая часть доменной модели документная и schemas-first мышление мешает, а не помогает.

## Interview-ready answer

MongoDB стоит выбирать, когда домен действительно документный: объект читается и пишется целиком, атрибуты сущностей сильно различаются по типу (product catalog с разными категориями), или структура активно меняется. Конкретные сценарии: product catalog с per-category атрибутами, user profile с flexible extensions от разных feature teams, CMS с составными blocks, activity feed с гетерогенными event payload. Важный антипаттерн: выбирать MongoDB потому что "не хочется миграций" — это только перекладывает проблему схемы с DDL на application-level validation. Если flexible нужна только часть данных, а основная модель relational — PostgreSQL с JSONB-колонкой часто достаточно и проще в эксплуатации.
