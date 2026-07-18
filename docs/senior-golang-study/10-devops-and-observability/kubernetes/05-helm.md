# Helm: charts, values и releases

Helm превращает шаблоны Kubernetes manifests в устанавливаемый пакет — chart — и ведёт историю его установок. Он уменьшает дублирование, но добавляет второй язык поверх YAML, поэтому chart должен оставаться проще приложения, которое он разворачивает.

Примеры используют общий для Helm 3 и Helm 4 workflow. Helm 4 является текущей stable major version; перед использованием редких flags проверяйте документацию версии, установленной в CI/CD.

## Содержание

- [Что решает Helm](#что-решает-helm)
- [Основные понятия](#основные-понятия)
- [Структура chart](#структура-chart)
- [Values и их приоритет](#values-и-их-приоритет)
- [Минимум template-синтаксиса](#минимум-template-синтаксиса)
- [Практический template](#практический-template)
- [ConfigMap и Secret](#configmap-и-secret)
- [Основной workflow](#основной-workflow)
- [Rollback и ограничения](#rollback-и-ограничения)
- [Практические правила](#практические-правила)
- [Interview-ready answer](#interview-ready-answer)

## Что решает Helm

Без шаблонизации окружения часто получают копии почти одинаковых manifests. Helm позволяет оставить один chart и менять только входные values:

```text
chart templates + default values + environment values
                         |
                         v
              rendered Kubernetes manifests
                         |
                         v
                       release
```

Helm полезен для:

- упаковки набора связанных Kubernetes resources;
- параметризации image, replicas, resources и feature flags;
- публикации и переиспользования charts;
- upgrade/rollback с историей revisions.

Helm не является единственным способом. Для небольших различий подходят plain YAML, Kustomize или platform-specific tooling. Helm также не управляет бизнес-совместимостью релизов и не делает migration обратимой.

## Основные понятия

| Понятие | Значение |
| --- | --- |
| `chart` | пакет templates, defaults и metadata |
| `template` | файл, который после рендера становится Kubernetes manifest |
| `values` | входные параметры рендера |
| `release` | установленный экземпляр chart в namespace |
| `revision` | версия состояния release после install/upgrade/rollback |

Один chart можно установить несколько раз с разными release names и values.

## Структура chart

```text
api-chart/
├── Chart.yaml
├── values.yaml
├── values.schema.json
└── templates/
    ├── _helpers.tpl
    ├── deployment.yaml
    ├── service.yaml
    └── configmap.yaml
```

`Chart.yaml`:

```yaml
apiVersion: v2
name: api
description: Helm chart for the API service
type: application
version: 0.3.0
appVersion: "1.8.2"
```

- `version` — версия chart и его templates;
- `appVersion` — информационная версия приложения;
- реальный image tag/digest всё равно задаётся в values/template.

Файлы templates с именем, начинающимся на `_`, используются как helpers и не рендерятся в самостоятельные manifests.

## Values и их приоритет

Безопасные defaults должны быть понятными и пригодными хотя бы для локального render:

```yaml
replicaCount: 2

image:
  repository: registry.example/api
  tag: "1.8.2"
  digest: ""
  pullPolicy: IfNotPresent

service:
  port: 80
  targetPort: 8080

resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    memory: 512Mi

config:
  logLevel: info
```

Приоритет возрастает слева направо:

```text
values.yaml
  < parent chart values
  < -f values-prod.yaml
  < -f values-region.yaml
  < --set / --set-string / --set-file
```

Если `-f` указан несколько раз, более правый файл имеет больший приоритет. Финальные values полезно проверять через `helm get values --all` после установки.

`values.schema.json` позволяет до рендера проверить required fields, типы и допустимые значения. Это надёжнее, чем узнавать об ошибке только от Kubernetes API.

<details>
<summary>Пример environment override</summary>

`values-prod.yaml` содержит только отличия от defaults:

```yaml
replicaCount: 5

image:
  tag: "1.8.4"

resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    memory: 1Gi

config:
  logLevel: warn

existingSecret: api-runtime-secrets
```

```bash
helm template api ./api-chart -f values-prod.yaml
```

Не нужно копировать весь `values.yaml`: короткий override легче сравнивать и меньше рискует случайно заморозить устаревший default.

</details>

## Минимум template-синтаксиса

Чаще всего достаточно нескольких конструкций:

```yaml
replicas: {{ .Values.replicaCount }}
imagePullPolicy: {{ .Values.image.pullPolicy }}

env:
  - name: LOG_LEVEL
    value: {{ .Values.config.logLevel | quote }}

resources:
  {{- toYaml .Values.resources | nindent 10 }}
```

- `.Values` — пользовательские values;
- `.Release.Name` и `.Release.Namespace` — текущий release;
- `.Chart.Name` и `.Chart.Version` — metadata chart;
- `quote` защищает строковое значение YAML;
- `toYaml | nindent N` выводит вложенный объект с корректным отступом;
- `required "message" value` останавливает render для обязательного значения;
- `with` меняет текущий context, `range` перебирает list/map.

Для повторяемых names и labels используют `_helpers.tpl`:

```gotemplate
{{- define "api.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "api.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
```

`include` возвращает строку, поэтому результат можно передать в pipeline:

```yaml
labels:
  {{- include "api.selectorLabels" . | nindent 4 }}
```

<details>
<summary>Примеры if, with и range</summary>

```gotemplate
{{- if .Values.podAnnotations }}
annotations:
  {{- toYaml .Values.podAnnotations | nindent 2 }}
{{- end }}

{{- with .Values.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 8 }}
{{- end }}

env:
  {{- range $name, $value := .Values.extraEnv }}
  - name: {{ $name }}
    value: {{ $value | quote }}
  {{- end }}
```

Внутри `with` точка указывает на выбранный объект. Если нужен корневой context, используют `$`, например `$.Release.Name`.

</details>

## Практический template

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "api.fullname" . }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "api.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "api.selectorLabels" . | nindent 8 }}
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
    spec:
      containers:
        - name: api
          {{- if .Values.image.digest }}
          image: "{{ .Values.image.repository }}@{{ .Values.image.digest }}"
          {{- else }}
          image: "{{ .Values.image.repository }}:{{ required "image.tag is required" .Values.image.tag }}"
          {{- end }}
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: {{ .Values.service.targetPort }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "api.fullname" . }}
spec:
  selector:
    {{- include "api.selectorLabels" . | nindent 4 }}
  ports:
    - name: http
      port: {{ .Values.service.port }}
      targetPort: http
```

Selector содержит только стабильные labels. Версию chart или image нельзя включать в Deployment selector: selector immutable и должен совпадать с labels Pod template.

Annotation `checksum/config` меняет Pod template при изменении отрендеренного ConfigMap и тем самым запускает rollout. Само изменение ConfigMap Deployment не перезапускает.

<details>
<summary>Фрагмент результата helm template</summary>

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prod-api
spec:
  replicas: 5
  selector:
    matchLabels:
      app.kubernetes.io/name: api
      app.kubernetes.io/instance: prod
  template:
    metadata:
      labels:
        app.kubernetes.io/name: api
        app.kubernetes.io/instance: prod
      annotations:
        checksum/config: 4e843d8c7d5b...
    spec:
      containers:
        - name: api
          image: registry.example/api:1.8.4
          resources:
            requests:
              cpu: 500m
              memory: 512Mi
            limits:
              memory: 1Gi
```

Именно этот YAML проверяет Kubernetes API. Если rendered manifest выглядит неожиданно, проблему нужно исправить до `helm upgrade`.

</details>

## ConfigMap и Secret

ConfigMap template может безопасно содержать неконфиденциальные values:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "api.fullname" . }}
data:
  LOG_LEVEL: {{ .Values.config.logLevel | quote }}
```

Настоящий secret не стоит передавать через обычный values file или `--set`:

- values могут попасть в Git, shell history и CI logs;
- `helm template`/`--dry-run` способен вывести rendered Secret;
- Helm хранит release metadata в Kubernetes Secret, включая rendered manifests.

Предпочтительный chart contract — принять имя уже существующего Secret:

```yaml
existingSecret: api-runtime-secrets
```

```yaml
envFrom:
  - secretRef:
      name: {{ required "existingSecret is required" .Values.existingSecret }}
```

Сам Secret создаёт отдельный secret-delivery механизм: External Secrets, Secrets Store CSI Driver, SOPS-based pipeline или управляемая platform integration. Важно понимать различие: External Secrets Operator обычно синхронизирует значение в Kubernetes Secret, а CSI mount может доставлять его без такого объекта — модель риска разная.

При preview используйте `--hide-secret`, если эта возможность есть в вашей версии Helm, и всё равно считайте CI output чувствительным.

<details>
<summary>Пример chart contract для существующего Secret</summary>

`values.yaml`:

```yaml
existingSecret: ""
```

Deployment template:

```yaml
env:
  - name: DATABASE_URL
    valueFrom:
      secretKeyRef:
        name: {{ required "existingSecret is required" .Values.existingSecret }}
        key: database-url
```

Environment override содержит только имя:

```yaml
existingSecret: payments-api-runtime
```

Chart не знает secret value и не включает его в rendered manifest или release metadata.

</details>

## Основной workflow

### До кластера

```bash
helm lint ./api-chart -f values-prod.yaml

helm template api ./api-chart \
  --namespace payments \
  -f values-prod.yaml > /tmp/api-rendered.yaml

kubectl apply --dry-run=server -f /tmp/api-rendered.yaml
```

`helm lint` проверяет chart conventions и schema, `helm template` показывает реальный YAML, а server dry-run добавляет Kubernetes schema и admission checks. Ни одна из этих проверок полностью не моделирует работающие controllers и external systems.

### Install или upgrade

```bash
helm upgrade --install api ./api-chart \
  --namespace payments \
  --create-namespace \
  -f values-prod.yaml \
  --wait \
  --timeout 5m
```

`--wait` ждёт готовности поддерживаемых resources до timeout. Для CI часто используют `--atomic`: при неуспешном upgrade Helm пытается откатить release. Это удобно, но не отменяет внешние side effects hooks, Jobs или database migrations.

### Диагностика release

```bash
helm list --all-namespaces
helm status api -n payments
helm get values api -n payments --all
helm get manifest api -n payments
helm history api -n payments
```

<details>
<summary>Пример истории release</summary>

```text
REVISION  UPDATED                   STATUS      CHART      APP VERSION  DESCRIPTION
1         2026-07-10 09:20 +0400    superseded  api-0.2.0  1.8.1        Install complete
2         2026-07-10 14:05 +0400    superseded  api-0.3.0  1.8.2        Upgrade complete
3         2026-07-11 11:40 +0400    deployed    api-0.3.1  1.8.4        Upgrade complete
```

Revision относится к release, а не совпадает с `Chart.version` или `appVersion`.

</details>

## Rollback и ограничения

```bash
helm rollback api 3 -n payments --wait --timeout 5m
helm uninstall api -n payments
```

Rollback рендерит состояние выбранной revision и создаёт новую revision. Он не гарантирует откат:

- schema/data migration;
- внешнего cloud resource;
- данных в PersistentVolume;
- side effect hook или Job;
- несовместимого client/server protocol.

`helm uninstall` удаляет resources release согласно manifests и policies, но resource с `helm.sh/resource-policy: keep` или внешнее состояние может остаться.

## Практические правила

| Правило | Зачем |
| --- | --- |
| pin image immutable tag или digest | одинаковый release запускает одинаковый код |
| держать selector labels стабильными | Deployment selector нельзя менять |
| использовать values schema и `required` | ошибка обнаруживается до rollout |
| рендерить chart в CI | виден итоговый manifest, а не только template |
| не хранить secret material в values | меньше утечек через Git, history и release metadata |
| ограничивать template-логику | сложный branching трудно тестировать и сопровождать |
| передавать структурные блоки через `toYaml` | меньше ручного дублирования Kubernetes schema |

`imagePullPolicy: Always` не заменяет immutable image. Он заставляет kubelet проверять registry reference, но mutable tag всё равно делает результат зависимым от времени. Digest даёт более сильную гарантию.

Namespace задаёт scope имён и RBAC, но сам по себе не создаёт network isolation. Для неё нужны NetworkPolicy и поддерживающий их dataplane.

## Interview-ready answer

**1. Что такое chart, values и release?**

Chart — пакет templates и defaults, values — входные параметры рендера, release — установленный экземпляр chart в namespace со своей историей revisions. Один chart можно установить несколько раз.

**2. Как безопасно проверить upgrade?**

Запускаю `helm lint`, рендерю `helm template` с теми же values и namespace, затем проверяю manifest server-side dry-run. В deployment pipeline использую `helm upgrade --install --wait --timeout`, а результат дополнительно проверяю по rollout и метрикам.

**3. Почему Secret не стоит хранить в values?**

Values и rendered manifests могут попасть в Git, CI output и Helm release metadata. Лучше передавать chart имя существующего Secret, а доставку значения выполнять отдельным secret-management механизмом.

**4. Что реально откатывает `helm rollback`?**

Kubernetes manifests выбранной revision. Он не откатывает автоматически БД, PersistentVolume и внешние side effects, поэтому rollback должен быть частью совместимой release strategy.

## Официальные источники

- [Helm charts](https://helm.sh/docs/topics/charts/)
- [Chart template guide](https://helm.sh/docs/chart_template_guide/)
- [Chart best practices](https://helm.sh/docs/chart_best_practices/)
- [helm upgrade](https://helm.sh/docs/helm/helm_upgrade/)
