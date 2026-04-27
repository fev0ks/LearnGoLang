# Что такое Terraform

## Содержание

- [Проблема без IaC](#проблема-без-iac)
- [Infrastructure as Code](#infrastructure-as-code)
- [Что делает Terraform](#что-делает-terraform)
- [Как работает цикл plan/apply](#как-работает-цикл-planapply)
- [Terraform vs альтернативы](#terraform-vs-альтернативы)
- [Что Terraform не делает](#что-terraform-не-делает)

---

## Проблема без IaC

Без IaC инфраструктура создаётся вручную через веб-консоль или CLI. Это порождает несколько проблем:

**Нет воспроизводимости.** Создал базу данных в dev — никто не знает точных настроек. В prod создашь немного по-другому. Баги только в prod.

**Нет истории изменений.** Кто-то изменил настройки безопасности три месяца назад — не известно кто, зачем, что именно.

**Не масштабируется.** Создать 10 одинаковых окружений вручную — несколько дней работы. И они всё равно будут отличаться.

**Disaster recovery — катастрофа.** Упал весь регион. Нужно восстановить 50 сервисов — никто не помнит все настройки.

```
Ручное создание:
  Разработчик → Консоль GCP → Кликает, заполняет формы → Готово
  
  Проблема: этот процесс нигде не записан,
            не версионирован, не повторяем
```

---

## Infrastructure as Code

IaC — подход, при котором инфраструктура описывается в коде (текстовых файлах), которые:
- хранятся в git (история, ревью, откат)
- могут быть применены автоматически
- дают одинаковый результат при каждом применении

```
IaC подход:
  Файл main.tf → git commit → CI запускает terraform apply → Инфраструктура
  
  Преимущества: файл всегда описывает что должно быть,
                git log показывает что и когда менялось,
                можно создать dev/staging/prod из одного кода
```

Terraform — наиболее распространённый IaC инструмент. Работает с AWS, GCP, Azure, и сотнями других провайдеров.

---

## Что делает Terraform

Terraform работает с **ресурсами** — объектами инфраструктуры: серверами, базами данных, сетями, правилами доступа и т.д.

Ты описываешь желаемое состояние в `.tf` файлах:

```hcl
# Хочу GCS bucket с именем "my-app-uploads"
resource "google_storage_bucket" "uploads" {
  name     = "my-app-uploads"
  location = "europe-west4"
}

# Хочу Cloud Run сервис
resource "google_cloud_run_v2_service" "api" {
  name     = "my-api"
  location = "europe-west4"

  template {
    containers {
      image = "europe-west4-docker.pkg.dev/my-project/repo/api:latest"
    }
  }
}
```

Terraform:
1. Читает твои `.tf` файлы
2. Читает текущее состояние инфраструктуры (через API провайдера)
3. Вычисляет разницу
4. Применяет только нужные изменения

---

## Как работает цикл plan/apply

```
terraform init     # скачать провайдеры и модули
terraform plan     # показать что изменится (ничего не меняет!)
terraform apply    # применить изменения (спрашивает подтверждение)
```

### terraform plan — самая важная команда

Plan показывает изменения до их применения. Это ключевое отличие от скриптов:

```
# Вывод terraform plan:
Terraform will perform the following actions:

  # google_storage_bucket.uploads will be created
  + resource "google_storage_bucket" "uploads" {
      + name     = "my-app-uploads"
      + location = "europe-west4"
    }

  # google_cloud_run_v2_service.api will be updated in-place
  ~ resource "google_cloud_run_v2_service" "api" {
      ~ template {
          ~ containers {
              ~ image = "api:v1.0" -> "api:v1.1"  # изменился тег
            }
        }
    }

Plan: 1 to add, 1 to change, 0 to destroy.
```

Символы в плане:
- `+` — ресурс будет создан
- `-` — ресурс будет удалён
- `~` — ресурс будет изменён
- `-/+` — ресурс будет пересоздан (удалён и создан заново, опасно!)

### terraform apply

Apply запрашивает подтверждение, затем применяет изменения:

```
Do you want to perform these actions?
  Terraform will perform the actions described above.
  Only 'yes' will be accepted to approve.

  Enter a value: yes

google_storage_bucket.uploads: Creating...
google_storage_bucket.uploads: Creation complete after 2s

Apply complete! Resources: 1 added, 1 changed, 0 destroyed.
```

---

## Terraform vs альтернативы

| Инструмент | Подход | Что делает | Когда использовать |
|---|---|---|---|
| **Terraform** | Декларативный IaC | Создаёт/изменяет инфраструктуру | Управление облачными ресурсами |
| **Ansible** | Императивный, конфигурация | Настраивает существующие серверы | Установка ПО на VMs, настройка ОС |
| **Pulumi** | Декларативный IaC (код) | То же что Terraform, но на Python/Go/TS | Когда нужна полная мощь языка программирования |
| **CloudFormation** | Декларативный IaC | Только AWS ресурсы | Если только AWS и нет Terraform |
| **CDK** | Код → CloudFormation | AWS через TypeScript/Python | AWS с типизированными конструктами |

**Terraform vs Ansible** — самый частый вопрос. Разные задачи:

```
Terraform: "создай базу данных PostgreSQL 15 в GCP с 2 CPU и 4GB RAM"
Ansible:   "зайди на этот сервер и установи nginx 1.24, поправь конфиг"

Terraform не умеет: настраивать ОС, устанавливать пакеты, запускать команды на сервере
Ansible не умеет: создавать облачные ресурсы декларативно
```

**Terraform vs Pulumi** — идеологическая разница:

```hcl
# Terraform (HCL) — специальный язык, понятный не-программистам
resource "aws_s3_bucket" "example" {
  bucket = "my-bucket"
}
```

```python
# Pulumi (Python) — настоящий язык со всеми его возможностями
bucket = aws.s3.Bucket("example", bucket="my-bucket")
```

Terraform популярнее из-за простоты и экосистемы.

---

## Что Terraform не делает

Terraform управляет **инфраструктурой**, не приложением:

```
Terraform делает:
  ✅ создать Cloud Run сервис
  ✅ создать базу данных
  ✅ настроить правила IAM
  ✅ создать сеть и подсети
  ✅ создать DNS записи

Terraform не делает:
  ❌ деплоить новую версию кода (это делает CI/CD)
  ❌ мигрировать базу данных (это делает goose/migrate)
  ❌ настраивать nginx внутри VM (это делает Ansible/cloud-init)
  ❌ мониторить работу сервисов (это делает Prometheus)
```

**Разделение ответственности в реальных проектах:**

```
git push → CI/CD pipeline
    │
    ├─► terraform apply    # обновить инфраструктуру (если изменились .tf файлы)
    │
    ├─► docker build/push  # собрать новый образ
    │
    └─► deploy new image   # задеплоить образ в Cloud Run / Kubernetes
                           # (terraform обычно НЕ деплоит код — это lifecycle.ignore_changes)
```

Типичный паттерн: Terraform создаёт Cloud Run сервис с placeholder image. CI/CD деплоит реальный образ через `gcloud run deploy` или `kubectl`. Terraform в `lifecycle` игнорирует изменения образа, чтобы не откатывать деплои.

---

## Структура файлов

Terraform читает все `.tf` файлы в директории. Имена файлов — соглашение, не требование:

```
my-service/
├── main.tf        # основные ресурсы
├── variables.tf   # входные переменные
├── outputs.tf     # выходные значения
├── versions.tf    # версии Terraform и провайдеров
└── locals.tf      # промежуточные вычисления (опционально)
```

```hcl
# versions.tf — фиксировать версии чтобы избежать неожиданных апгрейдов
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
