# Helm: charts, values и releases

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

Helm превращает шаблоны манифестов Kubernetes в устанавливаемый пакет (`chart`) и ведёт историю его установок. Он уменьшает дублирование, но добавляет второй язык поверх YAML, поэтому chart должен оставаться проще приложения, которое он разворачивает.

Примеры используют общий для Helm 3 и Helm 4 порядок работы. Helm 4 является текущей стабильной основной версией; поведение редких флагов стоит сверять с документацией той версии, которая установлена в CI/CD.

---

## Что решает Helm

Без шаблонизации для каждого окружения обычно заводят копии почти одинаковых манифестов. Helm позволяет оставить один chart и менять только входные значения (`values`):

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

- упаковки набора связанных объектов Kubernetes;
- параметризации образа, числа реплик, ресурсов и признаков включения функций;
- публикации и переиспользования charts;
- установки, обновления и отката с историей ревизий.

Helm не является единственным способом. При небольших различиях между окружениями достаточно обычного YAML, Kustomize или инструментов конкретной платформы. Helm также не управляет совместимостью версий приложения между собой и не делает миграцию данных обратимой.

---

## Основные понятия

| Понятие | Значение |
| --- | --- |
| `chart` | пакет templates, defaults и metadata |
| `template` | файл, который после рендера становится Kubernetes manifest |
| `values` | входные параметры рендера |
| `release` | установленный экземпляр chart в namespace |
| `revision` | версия состояния release после install/upgrade/rollback |

Один chart можно установить несколько раз с разными release names и values.

---

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

---

## Values и их приоритет

Значения по умолчанию должны быть понятными и пригодными хотя бы для локального рендера:

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

Если `-f` указан несколько раз, более правый файл имеет больший приоритет. Итоговый набор значений полезно проверять командой `helm get values --all` уже после установки.

`values.schema.json` позволяет ещё до рендера проверить обязательные поля, типы и допустимые значения. Это надёжнее, чем узнавать об ошибке только от Kubernetes API — или, что хуже, от упавшего Pod.

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

Копировать весь `values.yaml` не нужно: короткий файл отличий легче сравнивать, и он меньше рискует случайно зафиксировать устаревшее значение по умолчанию.

</details>

---

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

- `.Values` — переданные пользователем значения;
- `.Release.Name` и `.Release.Namespace` — текущая установка;
- `.Chart.Name` и `.Chart.Version` — метаданные chart;
- `quote` заключает значение в кавычки и защищает его от неожиданной интерпретации YAML;
- `toYaml | nindent N` выводит вложенный объект с корректным отступом;
- `required "сообщение" value` прерывает рендер, если обязательное значение не задано;
- `with` переключает текущий контекст, `range` перебирает список или отображение.

Для повторяющихся имён и меток используют `_helpers.tpl`:

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

Внутри `with` точка указывает на выбранный объект, а не на корень. Если нужен корневой контекст, используют `$`, например `$.Release.Name`.

</details>

---

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

Selector содержит только стабильные метки. Версию chart или образа включать в selector `Deployment` нельзя: после создания объекта selector изменить уже не получится, а совпадать он должен с метками шаблона Pod.

Аннотация `checksum/config` меняет шаблон Pod при изменении отрендеренного `ConfigMap` и тем самым запускает обновление. Само по себе изменение `ConfigMap` Pod не перезапускает.

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

Именно этот YAML получает Kubernetes API. Если отрендеренный манифест выглядит неожиданно, проблему исправляют до `helm upgrade`, а не после.

</details>

---

## ConfigMap и Secret

Шаблон `ConfigMap` может безопасно содержать неконфиденциальные значения:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "api.fullname" . }}
data:
  LOG_LEVEL: {{ .Values.config.logLevel | quote }}
```

Настоящее секретное значение не стоит передавать через обычный файл values или `--set`:

- значения попадают в Git, историю команд оболочки и логи CI;
- `helm template` и `--dry-run` выводят отрендеренный `Secret` в открытом виде;
- Helm хранит метаданные установки в объекте `Secret`, включая отрендеренные манифесты.

Более надёжный контракт chart — принимать имя уже существующего `Secret`:

```yaml
existingSecret: api-runtime-secrets
```

```yaml
envFrom:
  - secretRef:
      name: {{ required "existingSecret is required" .Values.existingSecret }}
```

Сам `Secret` создаёт отдельный механизм доставки секретов: External Secrets, Secrets Store CSI Driver, пайплайн на основе SOPS или встроенная интеграция платформы. Важно различать их модели риска: External Secrets Operator обычно синхронизирует значение в обычный объект `Secret`, а CSI-монтирование может доставить его прямо в том Pod, вообще не создавая такой объект.

При предварительном просмотре помогает флаг `--hide-secret`, если он есть в установленной версии Helm. Вывод CI при этом всё равно считают чувствительным.

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

Chart не знает самого значения и не включает его ни в отрендеренный манифест, ни в метаданные установки.

</details>

---

## Основной workflow

### До кластера

```bash
helm lint ./api-chart -f values-prod.yaml

helm template api ./api-chart \
  --namespace payments \
  -f values-prod.yaml > /tmp/api-rendered.yaml

kubectl apply --dry-run=server -f /tmp/api-rendered.yaml
```

`helm lint` проверяет соглашения chart и схему значений, `helm template` показывает реальный YAML, а серверный dry-run добавляет проверку схемы Kubernetes и политик приёма. Ни одна из этих проверок не моделирует работу контроллеров и внешних систем: они отвечают на вопрос «манифест корректен?», а не «приложение заработает?».

### Install или upgrade

```bash
helm upgrade --install api ./api-chart \
  --namespace payments \
  --create-namespace \
  -f values-prod.yaml \
  --wait \
  --timeout 5m
```

`--wait` ждёт готовности поддерживаемых объектов до истечения таймаута. В CI часто добавляют `--atomic`: при неудачном обновлении Helm пытается откатить установку. Это удобно, но не отменяет внешние последствия уже выполненных hooks, `Job` и миграций базы данных.

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

Номер ревизии относится к установке и не совпадает ни с `Chart.version`, ни с `appVersion`.

</details>

---

## Rollback и ограничения

```bash
helm rollback api 3 -n payments --wait --timeout 5m
helm uninstall api -n payments
```

Откат рендерит состояние выбранной ревизии и создаёт поверх него новую ревизию. Он не возвращает назад:

- миграцию схемы или данных;
- изменённый облачный ресурс;
- данные в `PersistentVolume`;
- последствия уже выполненных hooks и `Job`;
- несовместимость протокола между клиентом и сервером.

`helm uninstall` удаляет объекты установки согласно манифестам и политикам, но объект с аннотацией `helm.sh/resource-policy: keep` и любое внешнее состояние остаются.

---

## Практические правила

| Правило | Зачем |
| --- | --- |
| фиксировать образ неизменяемым тегом или digest | одна и та же установка запускает один и тот же код |
| держать selector-метки стабильными | selector `Deployment` после создания изменить нельзя |
| использовать схему values и `required` | ошибка обнаруживается до обновления, а не после |
| рендерить chart в CI | виден итоговый манифест, а не только шаблон |
| не хранить секретные значения в values | меньше путей утечки через Git, историю команд и метаданные установки |
| ограничивать логику в шаблонах | разветвлённые условия трудно тестировать и сопровождать |
| передавать структурные блоки через `toYaml` | меньше ручного дублирования схемы Kubernetes |

`imagePullPolicy: Always` не заменяет неизменяемый образ. Он лишь заставляет `kubelet` каждый раз обращаться к реестру, но изменяемый тег всё равно делает результат зависящим от момента запуска. Digest даёт более сильную гарантию.

`Namespace` задаёт область имён и прав доступа, но сам по себе не создаёт сетевую изоляцию. Для неё нужны `NetworkPolicy` и поддерживающая их сетевая реализация.

---

## Interview-ready answer

**1. Что такое chart, values и release?**

Chart — пакет шаблонов и значений по умолчанию, values — входные параметры рендера, release — установленный экземпляр chart в пространстве имён со своей историей ревизий. Один chart можно установить несколько раз с разными именами и значениями.

**2. Как безопасно проверить обновление?**

Запускаю `helm lint`, рендерю `helm template` с теми же values и namespace, затем проверяю манифест серверным dry-run. В пайплайне доставки использую `helm upgrade --install --wait --timeout`, а результат дополнительно проверяю по состоянию обновления и метрикам.

**3. Почему секреты не стоит хранить в values?**

Values и отрендеренные манифесты попадают в Git, вывод CI и метаданные установки Helm. Лучше передавать chart имя уже существующего `Secret`, а доставку самого значения выполнять отдельным механизмом управления секретами.

**4. Что реально откатывает `helm rollback`?**

Манифесты Kubernetes выбранной ревизии. Он не откатывает базу данных, содержимое `PersistentVolume` и внешние последствия, поэтому откат должен быть частью совместимой стратегии выпуска, а не единственной страховкой.

---

## Официальные источники

- [Helm charts](https://helm.sh/docs/topics/charts/)
- [Chart template guide](https://helm.sh/docs/chart_template_guide/)
- [Chart best practices](https://helm.sh/docs/chart_best_practices/)
- [helm upgrade](https://helm.sh/docs/helm/helm_upgrade/)
