# HCL: язык конфигурации Terraform

HCL (HashiCorp Configuration Language) — специальный язык для описания конфигурации. Разработан так, чтобы быть читаемым и для людей, и для машин. Все `.tf` файлы написаны на HCL.

## Содержание

- [Базовый синтаксис](#базовый-синтаксис)
- [Типы данных](#типы-данных)
- [Переменные (variable)](#переменные-variable)
- [Локальные значения (locals)](#локальные-значения-locals)
- [Выходные значения (output)](#выходные-значения-output)
- [Выражения и функции](#выражения-и-функции)
- [count и for_each](#count-и-for_each)
- [dynamic блоки](#dynamic-блоки)
- [Условия и фильтрация](#условия-и-фильтрация)

---

## Базовый синтаксис

HCL состоит из **блоков** и **атрибутов**:

```hcl
# Блок: тип "label_1" "label_2" { ... }
resource "google_storage_bucket" "my_bucket" {
  # Атрибут: имя = значение
  name     = "my-uploads-bucket"
  location = "europe-west4"

  # Вложенный блок
  versioning {
    enabled = true
  }
}
```

Правила синтаксиса:
- Строки — в двойных кавычках: `"hello"`
- Числа — без кавычек: `42`, `3.14`
- Булевы — `true` / `false`
- Комментарии — `#` или `//` для строчных, `/* */` для блочных
- Имена атрибутов — строчные буквы и подчёркивания

---

## Типы данных

```hcl
# string
name = "europe-west4"

# number
port = 8080

# bool
enabled = true

# list (порядок важен)
regions = ["europe-west4", "us-central1"]

# map (ключ → значение)
labels = {
  env  = "production"
  team = "backend"
}

# object (структура с известными полями)
# Используется в типах переменных (см. ниже)

# null — отсутствие значения
service_account = null
```

### Интерполяция строк

```hcl
variable "env" {
  default = "dev"
}

locals {
  bucket_name = "my-app-${var.env}-uploads"  # "my-app-dev-uploads"
  service_url = "https://${var.service_name}.${var.region}.run.app"
}
```

---

## Переменные (variable)

Переменные — входные параметры конфигурации. Объявляются в `variables.tf`:

```hcl
# Простая переменная с типом и описанием
variable "region" {
  description = "GCP region for all resources"
  type        = string
  default     = "europe-west4"
}

# Обязательная переменная (нет default — Terraform спросит при запуске)
variable "project_id" {
  description = "GCP project ID"
  type        = string
}

# Переменная с валидацией
variable "environment" {
  description = "Deployment environment"
  type        = string

  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment)
    error_message = "environment must be dev, staging, or prod"
  }
}

# Сложный тип — объект
variable "database" {
  description = "PostgreSQL instance configuration"
  type = object({
    tier              = string
    availability_type = optional(string, "ZONAL")  # optional с default
    disk_size_gb      = optional(number, 20)
  })
}

# Список объектов
variable "services" {
  description = "Cloud Run services to create"
  type = map(object({
    image              = string
    min_instance_count = optional(number, 0)
    max_instance_count = optional(number, 10)
    cpu                = optional(string, "1")
    memory             = optional(string, "512Mi")
  }))
  default = {}
}
```

### Использование переменных

```hcl
# Обращение к переменной — var.<имя>
resource "google_sql_database_instance" "main" {
  name             = "db-${var.environment}"
  database_version = "POSTGRES_15"
  region           = var.region

  settings {
    tier = var.database.tier
  }
}
```

### Передача значений переменных

```bash
# Через флаг командной строки
terraform apply -var="project_id=my-project" -var="environment=dev"

# Через файл (terraform.tfvars или *.tfvars)
terraform apply -var-file="dev.tfvars"

# Через переменные окружения — TF_VAR_<имя>
export TF_VAR_project_id="my-project"
terraform apply

# Файл автоматически читается если называется terraform.tfvars
# или *.auto.tfvars
```

```hcl
# dev.tfvars
project_id  = "my-project-dev"
environment = "dev"
region      = "europe-west4"
```

---

## Локальные значения (locals)

Locals — промежуточные вычисления. Используются чтобы не повторять одно и то же выражение несколько раз.

```hcl
locals {
  # Простое значение
  common_prefix = "${var.project_id}-${var.environment}"

  # Вычисленное из других locals
  bucket_name = "${local.common_prefix}-uploads"

  # Объект с несколькими полями
  common_labels = {
    environment = var.environment
    managed_by  = "terraform"
    project     = var.project_id
  }

  # Условное значение
  is_production = var.environment == "prod"
  min_instances = local.is_production ? 2 : 0

  # Трансформация map
  service_names = {
    for name, spec in var.services : name => "${name}-${var.environment}"
  }
}

# Использование: local.<имя>
resource "google_storage_bucket" "uploads" {
  name   = local.bucket_name
  labels = local.common_labels
}
```

Locals удобны для:
- вычисления имён ресурсов по шаблону
- общих меток (labels/tags) для всех ресурсов
- флагов на основе окружения
- трансформации входных данных перед использованием

---

## Выходные значения (output)

Outputs — способ отдать значения наружу. Используются в двух случаях:
1. Показать пользователю результат `terraform apply` (например, URL сервиса)
2. Передать значение в другой модуль или Terragrunt dependency

```hcl
# outputs.tf

output "service_url" {
  description = "Cloud Run service URL"
  value       = google_cloud_run_v2_service.api.uri
}

output "database_connection_name" {
  description = "CloudSQL connection name for Cloud Run"
  value       = google_sql_database_instance.main.connection_name
}

# Sensitive output — значение скрыто в логах
output "database_password" {
  description = "Database password"
  value       = random_password.db.result
  sensitive   = true
}

# Объект из нескольких значений
output "service_info" {
  value = {
    name     = google_cloud_run_v2_service.api.name
    url      = google_cloud_run_v2_service.api.uri
    location = google_cloud_run_v2_service.api.location
  }
}
```

```bash
# Посмотреть outputs после apply
terraform output
terraform output service_url
terraform output -json  # все outputs в JSON
```

---

## Выражения и функции

### Условный оператор

```hcl
# condition ? value_if_true : value_if_false
min_instance_count = var.environment == "prod" ? 2 : 0
deletion_protection = var.environment == "prod" ? true : false

# Для опциональных атрибутов часто используют null
service_account = var.use_custom_sa ? var.service_account_email : null
```

### Функции

HCL имеет встроенные функции. Самые используемые:

```hcl
locals {
  # merge — объединить несколько map
  all_labels = merge(
    local.common_labels,
    { component = "api" },
    var.extra_labels,
  )

  # try — вернуть первое значение которое не вызывает ошибку
  # Полезно для опциональных атрибутов объектов
  port = try(var.service.port, 8080)

  # lookup — получить значение из map с default
  tier = lookup(var.tier_by_env, var.environment, "db-f1-micro")

  # concat — объединить списки
  all_members = concat(var.admin_members, var.viewer_members)

  # flatten — развернуть список списков в плоский список
  all_instances = flatten([for svc in var.services : svc.instances])

  # toset — список в set (убирает дубли, используется в for_each)
  regions_set = toset(var.regions)

  # tomap — конвертация в map
  # jsonencode — сериализовать в JSON строку
  config_json = jsonencode({
    database_url = "postgres://..."
    redis_url    = "redis://..."
  })

  # format — форматирование строки как printf
  bucket_name = format("%s-%s-uploads", var.project_id, var.environment)

  # lower / upper / replace
  safe_name = lower(replace(var.service_name, "_", "-"))

  # length — длина списка или map
  service_count = length(var.services)

  # contains — проверить наличие в списке
  is_gcp = contains(["gcp", "google"], var.cloud_provider)

  # split / join
  parts  = split("/", "project/region/service")  # ["project", "region", "service"]
  path   = join("/", ["project", "region", "service"])  # "project/region/service"

  # coalesce — вернуть первое не-null и не-пустое значение
  name = coalesce(var.custom_name, local.generated_name)

  # file — прочитать файл
  startup_script = file("${path.module}/scripts/startup.sh")
}
```

### For выражения

For выражения трансформируют списки и map:

```hcl
locals {
  # list → list: преобразовать каждый элемент
  service_names = [for name in var.services : "${name}-${var.environment}"]

  # list → map: сделать map из списка
  services_by_name = {
    for svc in var.service_list : svc.name => svc
  }

  # map → map: преобразовать значения
  prefixed_services = {
    for name, spec in var.services : "${var.environment}-${name}" => spec
  }

  # map → map с фильтрацией
  prod_services = {
    for name, spec in var.services : name => spec
    if spec.environment == "prod"
  }

  # map → list (flatten pattern)
  all_env_pairs = flatten([
    for name, spec in var.services : [
      for member in spec.invoker_members : {
        service = name
        member  = member
      }
    ]
  ])

  # Превратить список объектов в map по ключу (нужно для for_each)
  # ключ должен быть уникальным
  bindings_map = {
    for pair in local.all_env_pairs : "${pair.service}:${pair.member}" => pair
  }
}
```

---

## count и for_each

Terraform может создавать несколько экземпляров ресурса из одного блока.

### count

Простое число — создать N одинаковых ресурсов:

```hcl
# Создать 3 одинаковых ресурса
resource "google_compute_instance" "worker" {
  count = 3

  name = "worker-${count.index}"  # worker-0, worker-1, worker-2
  # ...
}

# Использование: google_compute_instance.worker[0], [1], [2]
# или google_compute_instance.worker (список всех)

# Условное создание ресурса
resource "google_monitoring_alert_policy" "latency" {
  count = var.environment == "prod" ? 1 : 0
  # ...
}
# Если count = 0 — ресурс не создаётся
# Если count = 1 — создаётся один экземпляр: google_monitoring_alert_policy.latency[0]
```

### for_each

Предпочтительный способ для динамического создания ресурсов из коллекции. Каждый ресурс идентифицируется ключом, а не индексом:

```hcl
variable "buckets" {
  default = {
    uploads  = { location = "europe-west4" }
    backups  = { location = "us-central1" }
    archives = { location = "europe-west4" }
  }
}

resource "google_storage_bucket" "buckets" {
  for_each = var.buckets

  name     = "${var.project_id}-${each.key}"  # my-project-uploads, etc.
  location = each.value.location
}

# Обращение: google_storage_bucket.buckets["uploads"]
# Список всех: values(google_storage_bucket.buckets)
```

**Почему for_each лучше count для коллекций:**

```
Список: ["uploads", "backups", "archives"]  →  count = 3
Удалить "backups": ["uploads", "archives"]

С count:
  worker[0] = uploads   ✅
  worker[1] = archives  ✅ (было backups, стало archives → пересоздание!)
  worker[2] = УДАЛЁН

С for_each:
  buckets["uploads"]  = uploads   ✅ не трогать
  buckets["backups"]  = backups   → УДАЛИТЬ
  buckets["archives"] = archives  ✅ не трогать

for_each удаляет только то что нужно. count может пересоздать соседние ресурсы.
```

For_each требует `set(string)` или `map`:
```hcl
# Из списка строк — конвертировать в set
resource "google_project_service" "apis" {
  for_each = toset(["run.googleapis.com", "sqladmin.googleapis.com", "secretmanager.googleapis.com"])

  service = each.value  # each.key == each.value для set
}
```

---

## dynamic блоки

Dynamic позволяет создавать вложенные блоки в зависимости от данных:

```hcl
variable "env_vars" {
  default = {
    DATABASE_URL = "postgres://..."
    REDIS_URL    = "redis://..."
    LOG_LEVEL    = "info"
  }
}

resource "google_cloud_run_v2_service" "api" {
  name     = "my-api"
  location = var.region

  template {
    containers {
      image = var.image

      # Без dynamic пришлось бы писать блок env {} для каждой переменной вручную
      dynamic "env" {
        for_each = var.env_vars
        content {
          name  = env.key    # DATABASE_URL
          value = env.value  # postgres://...
        }
      }

      # Опциональный блок — создаётся только если есть данные
      dynamic "volume_mounts" {
        for_each = length(var.cloudsql_instances) > 0 ? [1] : []
        content {
          name       = "cloudsql"
          mount_path = "/cloudsql"
        }
      }
    }
  }
}
```

Трюк `for_each = condition ? [1] : []` — создаёт блок если условие true, не создаёт если false. Список `[1]` — один элемент, блок создастся один раз.

---

## Условия и фильтрация

```hcl
locals {
  # Включить дополнительный мониторинг только в prod
  enable_detailed_monitoring = var.environment == "prod"

  # Выбрать тип инстанса по окружению
  instance_type = {
    dev     = "db-f1-micro"
    staging = "db-g1-small"
    prod    = "db-custom-2-7680"
  }[var.environment]  # обращение к map по ключу

  # Фильтровать сервисы которым нужен VPC
  vpc_services = {
    for name, spec in var.services : name => spec
    if try(spec.needs_vpc, false)
  }
}

# Ресурс существует только в prod
resource "google_monitoring_uptime_check_config" "api_uptime" {
  count = var.environment == "prod" ? 1 : 0

  display_name = "API uptime check"
  # ...
}
```

### Работа с опциональными значениями

```hcl
locals {
  # try() перехватывает ошибку если атрибут не существует
  # Полезно когда объект может не иметь поля
  custom_port = try(var.service_config.port, 8080)
  vpc_egress  = try(var.service_config.vpc_egress, null)

  # coalesce — первое не-null значение
  service_name = coalesce(var.override_name, "default-service")

  # lookup с default
  region_short = lookup({
    "europe-west4" = "ew4"
    "us-central1"  = "uc1"
  }, var.region, "unknown")
}
```
