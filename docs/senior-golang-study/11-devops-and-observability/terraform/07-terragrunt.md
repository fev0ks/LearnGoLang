# Terragrunt

Terragrunt — тонкая обёртка над Terraform, которая решает проблему дублирования конфигурации при работе с несколькими окружениями (dev, staging, prod) и регионами.

## Содержание

- [Проблема без Terragrunt](#проблема-без-terragrunt)
- [Что делает Terragrunt](#что-делает-terragrunt)
- [Структура проекта](#структура-проекта)
- [terragrunt.hcl](#terragruntthcl)
- [root.hcl — общая конфигурация](#roothcl--общая-конфигурация)
- [dependency — зависимости между модулями](#dependency--зависимости-между-модулями)
- [generate — генерация файлов](#generate--генерация-файлов)
- [run-all — применить несколько модулей](#run-all--применить-несколько-модулей)
- [Практические паттерны](#практические-паттерны)

---

## Проблема без Terragrunt

Представь три окружения. В каждом нужно настроить remote backend:

```hcl
# envs/dev/backend.tf — дублирование!
terraform {
  backend "gcs" {
    bucket = "my-terraform-state"
    prefix = "dev/cloud-run"
  }
}

# envs/staging/backend.tf
terraform {
  backend "gcs" {
    bucket = "my-terraform-state"
    prefix = "staging/cloud-run"  # только prefix отличается
  }
}

# envs/prod/backend.tf
terraform {
  backend "gcs" {
    bucket = "my-terraform-state"
    prefix = "prod/cloud-run"
  }
}
```

И так для каждого компонента (cloud-run, database, buckets, iam...). Десятки одинаковых backend.tf файлов с минимальными отличиями.

То же с провайдером:
```hcl
# В каждой директории нужен одинаковый provider "google" {}
# И одинаковый required_providers
```

**Terragrunt решает это**: один `root.hcl` содержит общую конфигурацию, каждый модуль наследует её.

---

## Что делает Terragrunt

1. **DRY backend** — генерирует `backend.tf` автоматически с правильным prefix для каждой директории
2. **DRY provider** — генерирует `provider.tf` с общей конфигурацией
3. **Dependency management** — модуль может читать outputs другого модуля
4. **run-all** — запустить plan/apply для нескольких модулей одновременно с учётом зависимостей

Terragrunt не заменяет Terraform — он его вызывает:
```
terragrunt plan  →  генерирует backend.tf, provider.tf  →  terraform plan
```

---

## Структура проекта

```
infrastructure/
├── modules/                          # переиспользуемые Terraform модули
│   └── gcp/
│       ├── cloud-run-services/
│       │   ├── main.tf
│       │   ├── variables.tf
│       │   ├── outputs.tf
│       │   └── versions.tf
│       ├── cloudsql-postgres/
│       └── gcs-buckets/
│
└── envs/                             # Terragrunt конфигурации
    ├── dev/
    │   └── eu/
    │       ├── root.hcl              # общая конфигурация для dev/eu
    │       └── gcp/
    │           ├── cloud-run-services/
    │           │   └── terragrunt.hcl
    │           ├── cloudsql-postgres/
    │           │   └── terragrunt.hcl
    │           └── gcs-buckets/
    │               └── terragrunt.hcl
    ├── staging/
    │   └── eu/
    │       ├── root.hcl
    │       └── gcp/ ...
    └── prod/
        └── eu/
            ├── root.hcl
            └── gcp/ ...
```

---

## terragrunt.hcl

Каждый `terragrunt.hcl` описывает один компонент инфраструктуры:

```hcl
# envs/dev/eu/gcp/cloud-run-services/terragrunt.hcl

# Наследовать общую конфигурацию из родительского root.hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}

# Указать на Terraform модуль
# path_relative_from_include() — путь от root.hcl до данного файла
# path_relative_to_include() — путь данного файла относительно root.hcl
# Результат: "../../../modules/gcp/cloud-run-services"
terraform {
  source = "${path_relative_from_include()}/../../../modules/${path_relative_to_include()}"
}

# Локальные переменные для этого компонента
locals {
  env    = "dev"
  region = "europe-west4"
}

# Входные переменные для Terraform модуля
inputs = {
  region = local.region

  services = {
    "my-api-dev" = {
      image              = "europe-west4-docker.pkg.dev/my-project/repo/api:main-latest"
      min_instance_count = 0
      max_instance_count = 3
      cpu                = "1"
      memory             = "512Mi"
      public_invoker     = true
    }
    "my-worker-dev" = {
      image              = "europe-west4-docker.pkg.dev/my-project/repo/worker:main-latest"
      min_instance_count = 0
      max_instance_count = 2
      cpu                = "1"
      memory             = "1Gi"
    }
  }
}
```

---

## root.hcl — общая конфигурация

`root.hcl` содержит то что общее для всего окружения: backend, провайдер, общие переменные.

```hcl
# envs/dev/eu/root.hcl

locals {
  project_id     = "my-project-dev"
  project_number = "123456789012"
  region         = "europe-west4"

  # Аутентификация: файл или переменная окружения
  credentials_file = "${get_parent_terragrunt_dir()}/config/credentials.json"
  credentials_json = fileexists(local.credentials_file) \
    ? file(local.credentials_file) \
    : get_env("GOOGLE_CREDENTIALS_JSON", "")
}

# Автоматически генерировать backend.tf в каждой поддиректории
remote_state {
  backend                         = "gcs"
  disable_dependency_optimization = true

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    bucket = "my-project-terraform-state"
    # path_relative_to_include() — путь от root.hcl до текущего terragrunt.hcl
    # Для dev/eu/gcp/cloud-run-services/ → "dev/eu/gcp/cloud-run-services"
    prefix = "dev/eu/${path_relative_to_include()}"
  }
}

# Настройки Terraform
terraform {
  # Добавить флаг -lock-timeout для команд которые используют лок
  extra_arguments "retry_lock" {
    commands  = get_terraform_commands_that_need_locking()
    arguments = ["-lock-timeout=20m"]
  }

  # Передать переменные окружения для всех команд
  extra_arguments "common_vars" {
    commands = ["init", "apply", "refresh", "import", "plan", "destroy"]
    env_vars = {
      TF_VAR_google_project_id     = local.project_id
      TF_VAR_google_project_number = local.project_number
      TF_VAR_default_region        = local.region
    }
  }
}

# Автоматически генерировать provider.tf в каждой поддиректории
generate "global_providers" {
  path      = "global_provider_override.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "google" {
  project     = "${local.project_id}"
  credentials = ${local.credentials_json != "" ? jsonencode(local.credentials_json) : "null"}
  region      = "${local.region}"
}
EOF
}
```

Каждая поддиректория наследует это через:
```hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}
```

`find_in_parent_folders("root.hcl")` ищет файл с именем `root.hcl` в родительских директориях — поднимается вверх пока не найдёт.

---

## dependency — зависимости между модулями

Часто один модуль нуждается в outputs другого. Например, Cloud Run сервисам нужен IP адрес Redis и connection name CloudSQL.

```hcl
# envs/dev/eu/gcp/cloud-run-services/terragrunt.hcl

# Зависимость от CloudSQL модуля
dependency "cloudsql" {
  # Путь до terragrunt.hcl зависимого модуля
  config_path = "../cloudsql-postgres"

  # mock_outputs используются при terraform plan
  # чтобы не запускать зависимый модуль
  mock_outputs_allowed_terraform_commands = ["init", "plan", "validate"]
  mock_outputs = {
    connection_name    = "my-project:europe-west4:dev-db"
    public_ip_address  = "34.0.0.1"
    private_ip_address = ""
  }
}

# Зависимость от Redis
dependency "redis" {
  config_path = "../redis-vm"

  mock_outputs_allowed_terraform_commands = ["init", "plan", "validate"]
  mock_outputs = {
    internal_ip = "10.164.0.10"
  }
}

# Зависимость от service accounts
dependency "service_accounts" {
  config_path = "../runtime-service-accounts"

  mock_outputs_allowed_terraform_commands = ["init", "plan", "validate"]
  mock_outputs = {
    service_account_emails = {
      "dev-api"    = "dev-api@my-project.iam.gserviceaccount.com"
      "dev-worker" = "dev-worker@my-project.iam.gserviceaccount.com"
    }
  }
}

inputs = {
  services = {
    "my-api-dev" = {
      image                 = "europe-west4-docker.pkg.dev/my-project/repo/api:main-latest"
      service_account_email = dependency.service_accounts.outputs.service_account_emails["dev-api"]
      cloudsql_instances    = [dependency.cloudsql.outputs.connection_name]
      env = {
        DB_HOST    = "/cloudsql/${dependency.cloudsql.outputs.connection_name}"
        DB_PORT    = "5432"
        REDIS_HOST = dependency.redis.outputs.internal_ip
        REDIS_PORT = "6379"
      }
    }
  }
}
```

**Как работает dependency:**

```
terragrunt apply для cloud-run-services:
  1. Проверить что cloudsql, redis, service_accounts уже применены
  2. Прочитать их outputs из state
  3. Передать как inputs в cloud-run-services
```

**mock_outputs** нужны для `plan`: при планировании cloud-run-services не нужно запускать cloudsql. Mock-значения заполняют outputs зависимостей, чтобы Terraform мог вычислить план.

---

## generate — генерация файлов

`generate` блок создаёт `.tf` файлы в директории перед запуском Terraform. Используется для кода который нельзя параметризировать через variables (например, `provider` и `terraform { backend }` блоки).

```hcl
# Генерировать файл с провайдером
generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite_terragrunt"  # перезаписывать при каждом запуске
  contents  = <<EOF
provider "google" {
  project = "my-project"
  region  = "europe-west4"
}
EOF
}

# Генерировать версии провайдеров
generate "versions" {
  path      = "versions_override.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0, < 7.0"
    }
  }
}
EOF
}
```

`if_exists` варианты:
- `"overwrite_terragrunt"` — перезаписывать если файл создан Terragrunt (рекомендуется)
- `"overwrite"` — перезаписывать всегда
- `"skip"` — не перезаписывать если файл уже есть
- `"error"` — ошибка если файл уже есть

---

## run-all — применить несколько модулей

```bash
# Запустить plan для всего окружения dev/eu
cd envs/dev/eu
terragrunt run-all plan

# Apply для всего окружения
terragrunt run-all apply

# Только для конкретного поддерева
cd envs/dev/eu/gcp
terragrunt run-all apply

# Destroy для всего окружения (опасно!)
terragrunt run-all destroy
```

`run-all` учитывает dependency блоки — запускает модули в правильном порядке.

```
Порядок применения (run-all apply):
  1. runtime-service-accounts  (нет зависимостей)
  2. cloudsql-postgres         (нет зависимостей)
  3. redis-vm                  (нет зависимостей)
  4. gcs-buckets               (нет зависимостей)
  --- параллельно выше ---
  5. cloud-run-services        (зависит от 1, 2, 3, 4)
```

Модули без зависимостей друг от друга применяются параллельно.

---

## Практические паттерны

### Вычисление source пути

Стандартный паттерн для source:

```hcl
# В terragrunt.hcl:
# envs/dev/eu/gcp/cloud-run-services/terragrunt.hcl

terraform {
  source = "${path_relative_from_include()}/../../../modules/${path_relative_to_include()}"
}

# path_relative_from_include() = "../../../.." (до корня, где root.hcl)
# path_relative_to_include()   = "gcp/cloud-run-services"
# Результат: "../../../../modules/gcp/cloud-run-services"
#
# Смысл: найти модуль с таким же путём в modules/
```

### Использование run_cmd для динамических данных

```hcl
locals {
  # Выполнить shell команду и получить результат
  # Полезно для данных которые нельзя хардкодить в .hcl
  git_tag = run_cmd("git", "describe", "--tags", "--abbrev=0")

  # Прочитать JSON файл с runtime данными
  runtime_config = jsondecode(run_cmd(
    "bash", "-c",
    "cat ${get_parent_terragrunt_dir()}/generated/runtime.json"
  ))
}
```

### Структура для нескольких регионов

```
envs/
└── prod/
    ├── eu/
    │   ├── root.hcl        # prod/eu конфигурация
    │   └── gcp/
    │       └── cloud-run/
    └── us/
        ├── root.hcl        # prod/us конфигурация (другой регион)
        └── gcp/
            └── cloud-run/
```

```hcl
# envs/prod/eu/root.hcl
locals {
  region = "europe-west4"
}

# envs/prod/us/root.hcl
locals {
  region = "us-central1"
}
```

### Общий root для всех окружений

```
envs/
├── _common/
│   └── root.hcl          # общие настройки для ВСЕХ окружений
├── dev/
│   └── eu/
│       └── root.hcl      # наследует _common + добавляет dev-специфику
└── prod/
    └── eu/
        └── root.hcl      # наследует _common + добавляет prod-специфику
```

```hcl
# envs/dev/eu/root.hcl
locals {
  project_id  = "my-project-dev"
  environment = "dev"
}

# Включить общий root
generate "common" {
  path = "common.tf"
  # ...
}
```

### Команды Terragrunt

```bash
# Инициализация всех модулей
terragrunt run-all init

# Plan с игнорированием зависимостей (только текущий модуль)
terragrunt plan --terragrunt-ignore-dependency-errors

# Не скачивать source при init (использовать кешированный)
terragrunt init --terragrunt-no-auto-init

# Подробные логи Terragrunt
terragrunt plan --terragrunt-log-level debug

# Исключить конкретный модуль из run-all
terragrunt run-all apply --terragrunt-exclude-dir envs/dev/eu/gcp/redis-vm

# Вывести граф зависимостей
terragrunt graph-dependencies
```
