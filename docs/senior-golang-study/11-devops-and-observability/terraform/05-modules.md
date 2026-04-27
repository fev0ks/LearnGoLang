# Модули

Модуль — переиспользуемый, параметризованный блок Terraform конфигурации. Это способ упаковать инфраструктуру так, чтобы её можно было вызывать с разными параметрами.

## Содержание

- [Зачем модули](#зачем-модули)
- [Структура модуля](#структура-модуля)
- [Вызов модуля](#вызов-модуля)
- [Outputs из модуля](#outputs-из-модуля)
- [Источники модулей](#источники-модулей)
- [Когда выносить в модуль](#когда-выносить-в-модуль)
- [Антипаттерны](#антипаттерны)

---

## Зачем модули

Без модулей конфигурация для dev, staging, prod будет трёх копиями почти одинакового кода:

```
# Без модулей — дублирование:
envs/dev/main.tf     — создать Cloud Run, CloudSQL, GCS почти одинаково
envs/staging/main.tf — то же самое
envs/prod/main.tf    — то же самое, но больше CPU и replica
```

С модулями — один раз описать, вызвать с разными параметрами:

```
modules/cloud-run-service/  — один раз описать как создавать Cloud Run сервис
envs/dev/main.tf            — вызвать модуль с параметрами для dev
envs/prod/main.tf           — вызвать тот же модуль с параметрами для prod
```

---

## Структура модуля

Стандартная структура файлов модуля:

```
modules/
└── cloud-run-service/
    ├── main.tf        # ресурсы — что создаёт модуль
    ├── variables.tf   # входные параметры
    ├── outputs.tf     # что модуль возвращает наружу
    └── versions.tf    # требуемые версии Terraform и провайдеров
```

### variables.tf — входные параметры

```hcl
# modules/cloud-run-service/variables.tf

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
}

variable "name" {
  description = "Cloud Run service name"
  type        = string
}

variable "image" {
  description = "Container image to deploy"
  type        = string
}

variable "port" {
  description = "Container port"
  type        = number
  default     = 8080
}

variable "cpu" {
  description = "CPU limit (e.g. '1', '2')"
  type        = string
  default     = "1"
}

variable "memory" {
  description = "Memory limit (e.g. '512Mi', '1Gi')"
  type        = string
  default     = "512Mi"
}

variable "min_instances" {
  description = "Minimum number of instances"
  type        = number
  default     = 0
}

variable "max_instances" {
  description = "Maximum number of instances"
  type        = number
  default     = 10
}

variable "env_vars" {
  description = "Environment variables"
  type        = map(string)
  default     = {}
}

variable "secret_env" {
  description = "Environment variables from Secret Manager"
  type = map(object({
    secret  = string
    version = optional(string, "latest")
  }))
  default = {}
}

variable "service_account_email" {
  description = "Service account email to run as"
  type        = string
  default     = null
}

variable "public_access" {
  description = "Allow unauthenticated access"
  type        = bool
  default     = false
}

variable "cloudsql_instances" {
  description = "List of CloudSQL instance connection names"
  type        = list(string)
  default     = []
}

variable "labels" {
  description = "Resource labels"
  type        = map(string)
  default     = {}
}
```

### main.tf — ресурсы

```hcl
# modules/cloud-run-service/main.tf

resource "google_cloud_run_v2_service" "this" {
  name     = var.name
  location = var.region
  project  = var.project_id
  ingress  = var.public_access ? "INGRESS_TRAFFIC_ALL" : "INGRESS_TRAFFIC_INTERNAL_ONLY"

  labels = var.labels

  template {
    service_account = var.service_account_email

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    dynamic "volumes" {
      for_each = length(var.cloudsql_instances) > 0 ? [1] : []
      content {
        name = "cloudsql"
        cloud_sql_instance {
          instances = var.cloudsql_instances
        }
      }
    }

    containers {
      image = var.image

      ports {
        container_port = var.port
      }

      resources {
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
      }

      dynamic "env" {
        for_each = var.env_vars
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = var.secret_env
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value.secret
              version = env.value.version
            }
          }
        }
      }

      dynamic "volume_mounts" {
        for_each = length(var.cloudsql_instances) > 0 ? [1] : []
        content {
          name       = "cloudsql"
          mount_path = "/cloudsql"
        }
      }
    }
  }

  lifecycle {
    ignore_changes = ["template[0].containers[0].image"]
  }
}

# IAM — разрешить публичный доступ если нужно
resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  count = var.public_access ? 1 : 0

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.this.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
```

### outputs.tf — что отдаёт наружу

```hcl
# modules/cloud-run-service/outputs.tf

output "name" {
  description = "Cloud Run service name"
  value       = google_cloud_run_v2_service.this.name
}

output "uri" {
  description = "Cloud Run service URL"
  value       = google_cloud_run_v2_service.this.uri
}

output "id" {
  description = "Cloud Run service full resource ID"
  value       = google_cloud_run_v2_service.this.id
}
```

### versions.tf — требования к версиям

```hcl
# modules/cloud-run-service/versions.tf

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0, < 7.0"
    }
  }
}
```

---

## Вызов модуля

```hcl
# envs/prod/main.tf

module "api_service" {
  source = "../../modules/cloud-run-service"  # путь к модулю

  # Обязательные переменные
  project_id = var.project_id
  region     = var.region
  name       = "prod-api"
  image      = "europe-west4-docker.pkg.dev/${var.project_id}/repo/api:bootstrap"

  # Опциональные переменные
  cpu           = "2"
  memory        = "1Gi"
  min_instances = 2
  max_instances = 20
  public_access = true

  env_vars = {
    ENVIRONMENT = "prod"
    LOG_LEVEL   = "warn"
  }

  secret_env = {
    DATABASE_URL = {
      secret = "prod-database-url"
    }
  }

  service_account_email = google_service_account.api.email

  cloudsql_instances = [
    google_sql_database_instance.main.connection_name
  ]

  labels = {
    environment = "prod"
    component   = "api"
  }
}

module "worker_service" {
  source = "../../modules/cloud-run-service"  # тот же модуль, другие параметры

  project_id = var.project_id
  region     = var.region
  name       = "prod-worker"
  image      = "europe-west4-docker.pkg.dev/${var.project_id}/repo/worker:bootstrap"

  cpu           = "1"
  memory        = "2Gi"
  min_instances = 1
  max_instances = 5
  public_access = false  # worker не публичный

  env_vars = {
    ENVIRONMENT  = "prod"
    WORKER_QUEUE = "tasks"
  }
}
```

### Несколько экземпляров одного модуля через for_each

```hcl
variable "services" {
  default = {
    api    = { cpu = "2", memory = "1Gi", public = true }
    worker = { cpu = "1", memory = "2Gi", public = false }
    cron   = { cpu = "1", memory = "512Mi", public = false }
  }
}

module "services" {
  for_each = var.services
  source   = "../../modules/cloud-run-service"

  project_id    = var.project_id
  region        = var.region
  name          = "prod-${each.key}"
  image         = "gcr.io/${var.project_id}/${each.key}:bootstrap"
  cpu           = each.value.cpu
  memory        = each.value.memory
  public_access = each.value.public
}

# Обращение: module.services["api"].uri
output "api_url" {
  value = module.services["api"].uri
}
```

---

## Outputs из модуля

```hcl
# Обращение к output модуля: module.<имя_модуля>.<имя_output>

output "api_url" {
  value = module.api_service.uri
}

# Использовать output модуля в другом ресурсе
resource "google_cloud_run_v2_service_iam_member" "internal_invoker" {
  name     = module.worker_service.name  # ← output из модуля
  location = var.region
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.scheduler.email}"
}

# При for_each модулей
output "all_service_urls" {
  value = {
    for name, mod in module.services : name => mod.uri
  }
}
```

---

## Источники модулей

```hcl
# Локальный путь
module "service" {
  source = "./modules/cloud-run-service"
  source = "../shared/modules/database"
}

# Git репозиторий
module "service" {
  source = "git::https://github.com/my-org/terraform-modules.git//cloud-run-service?ref=v1.2.0"
}

# Terraform Registry (публичные модули)
module "gke" {
  source  = "terraform-google-modules/kubernetes-engine/google"
  version = "~> 29.0"
  # ...
}

# GCS bucket
module "service" {
  source = "gcs::https://www.googleapis.com/storage/v1/my-tf-modules/cloud-run-service.zip"
}
```

Для локальных путей `version` не используется. Для Registry и Git — указывать `version` обязательно чтобы избежать неожиданных изменений.

---

## Когда выносить в модуль

**Хорошие кандидаты для модулей:**

- Комбинация ресурсов которая повторяется 3+ раз
- Набор ресурсов с нетривиальной конфигурацией (Cloud Run + IAM + VPC + Secret refs)
- Стандарт компании: "все Cloud Run сервисы должны иметь такую конфигурацию"
- Абстракция над сложным API: вместо 15 полей Google Cloud Run — 5 основных

**Не нужен модуль когда:**

- Ресурс один и не будет повторяться
- Модуль был бы тонкой обёрткой без логики (просто перекидывает переменные)
- Конфигурации слишком разные чтобы параметризировать

```
Правило: не создавай модуль чтобы "структурировать код".
Создавай когда есть реальное переиспользование или стандартизация.
```

---

## Антипаттерны

**Слишком много переменных** — модуль с 40+ переменными сложнее использовать чем написать с нуля. Если каждый атрибут ресурса выставляется как переменная — это не абстракция.

```hcl
# Плохо — просто перекидывает всё наружу
variable "service_timeout" { ... }
variable "service_max_instance_request_concurrency" { ... }
variable "service_session_affinity" { ... }
# ... ещё 30 переменных ...

# Хорошо — прячет детали, выставляет только важное
variable "cpu" { default = "1" }
variable "memory" { default = "512Mi" }
variable "scaling" {
  type = object({
    min = optional(number, 0)
    max = optional(number, 10)
  })
}
```

**Обёртки ради обёрток** — модуль который просто вызывает другой модуль без добавления логики.

**Нет versioning для внешних модулей:**
```hcl
# Плохо — может сломаться при обновлении
module "gke" {
  source = "terraform-google-modules/kubernetes-engine/google"
}

# Хорошо — зафиксирована версия
module "gke" {
  source  = "terraform-google-modules/kubernetes-engine/google"
  version = "~> 29.0"
}
```

**Outputs которые не нужны** — не нужно выставлять все атрибуты ресурса. Только те, которые реально использует вызывающий код.
