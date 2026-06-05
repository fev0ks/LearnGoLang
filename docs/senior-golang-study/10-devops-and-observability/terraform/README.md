# Terraform

Terraform — инструмент Infrastructure as Code (IaC). Описываешь инфраструктуру в файлах, Terraform сравнивает её с реальным состоянием и применяет только нужные изменения.

## Материалы

- [01 Что такое Terraform](./01-what-is-terraform.md) — IaC, зачем нужен, как работает цикл plan/apply
- [02 HCL: язык конфигурации](./02-hcl-language.md) — синтаксис, типы, переменные, locals, outputs, for_each, dynamic
- [03 Провайдеры и ресурсы](./03-providers-and-resources.md) — провайдеры, ресурсы, data sources, lifecycle, depends_on
- [04 State и бэкенды](./04-state-and-backends.md) — state файл, remote backends, блокировка, импорт
- [05 Модули](./05-modules.md) — структура модуля, вызов, outputs, когда выносить
- [06 Workflow](./06-workflow.md) — init, plan, apply, destroy, CI/CD паттерн
- [07 Terragrunt](./07-terragrunt.md) — DRY конфиги, root.hcl, dependency, mock_outputs, generate
- [08 GCP паттерны](./08-gcp-patterns.md) — Cloud Run, IAM, CloudSQL, Secret Manager, GCS

## Ключевые концепции

**Декларативность** — описываешь что должно быть, а не как это сделать. Terraform сам решает порядок операций.

**Идемпотентность** — `terraform apply` можно запускать сколько угодно раз. Если инфраструктура уже соответствует конфигурации — ничего не изменится.

**Plan before apply** — всегда видишь что изменится до применения. Это ключевое преимущество перед скриптами.

**State** — Terraform хранит слепок реальной инфраструктуры. State — источник правды о том, что Terraform уже создал.

## Структура типичного проекта

```
infrastructure/
├── modules/                  # переиспользуемые модули
│   └── cloud-run-service/
│       ├── main.tf
│       ├── variables.tf
│       ├── outputs.tf
│       └── versions.tf
└── envs/                     # конфигурации окружений
    ├── dev/
    │   └── cloud-run/
    │       └── terragrunt.hcl
    ├── staging/
    └── prod/
```

## Вопросы

- в чём разница между Terraform и Ansible;
- что такое state файл и зачем он нужен;
- почему нельзя хранить state файл в git;
- что происходит если два инженера запустят `terraform apply` одновременно;
- зачем нужен Terragrunt если есть Terraform;
- что такое `terraform plan` и почему его запускают в CI на каждый PR;
- как добавить существующий ресурс под управление Terraform без пересоздания.
