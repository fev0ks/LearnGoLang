# Helm

Helm — менеджер пакетов для Kubernetes. Если Kubernetes описывает инфраструктуру через YAML-манифесты, то Helm добавляет поверх них параметризацию, версионирование и удобное управление.

## Содержание

- [Проблема без Helm](#проблема-без-helm)
- [Основные понятия](#основные-понятия)
- [Структура чарта](#структура-чарта)
- [Chart.yaml](#chartyaml)
- [values.yaml — параметры чарта](#valuesyaml--параметры-чарта)
- [Template синтаксис](#template-синтаксис)
- [_helpers.tpl — переиспользуемые шаблоны](#_helperstpl--переиспользуемые-шаблоны)
- [ConfigMap и Secret](#configmap-и-secret)
- [Deployment](#deployment)
- [Service](#service)
- [Helm команды](#helm-команды)
- [Releases и namespaces](#releases-и-namespaces)
- [Антипаттерны](#антипаттерны)
- [Interview-ready answer](#interview-ready-answer)

---

## Проблема без Helm

Без Helm для каждого окружения нужно отдельно описывать манифесты. Два сервиса и три окружения — уже 6 почти одинаковых Deployment-файлов; изменение числа реплик — руками в каждом.

```
# Без Helm: отдельные файлы на каждое окружение
manifests/
├── dev/
│   ├── shortener-deployment.yaml    # replicas: 1, image: :dev
│   └── analytics-deployment.yaml
├── staging/
│   ├── shortener-deployment.yaml    # replicas: 2, image: :staging
│   └── analytics-deployment.yaml
└── prod/
    ├── shortener-deployment.yaml    # replicas: 5, image: :prod
    └── analytics-deployment.yaml
```

С Helm:

```
# С Helm: один чарт, разные values
helm/url-shortener/
├── templates/
│   └── shortener-deployment.yaml   # один шаблон с {{ .Values.replicaCount }}
└── values.yaml                     # значения по умолчанию

helm upgrade --install myapp ./url-shortener -f values-prod.yaml
```

---

## Основные понятия

**Chart** — пакет Helm. Директория с шаблонами манифестов и файлом значений. Аналог npm-пакета.

**Values** — входные параметры чарта. Задаются в `values.yaml` и могут быть переопределены при установке.

**Template** — YAML-файл с плейсхолдерами (`{{ .Values.something }}`). Helm подставляет значения и получает обычный Kubernetes манифест.

**Release** — конкретная установка чарта в кластер. Один и тот же чарт можно установить несколько раз под разными именами. Helm хранит историю каждого release.

```
Chart (шаблон) + Values (параметры) = Kubernetes Manifests (реальные YAML)
                                               ↓
                                          Release (установленная версия)
```

---

## Структура чарта

```
url-shortener/           # имя чарта — директория
├── Chart.yaml           # метаданные: имя, версия, описание
├── values.yaml          # значения по умолчанию
├── values-kind.yaml     # переопределения для конкретной среды (не стандарт, просто файл)
└── templates/           # шаблоны манифестов
    ├── _helpers.tpl     # переиспользуемые именованные шаблоны (не создаёт ресурсы)
    ├── shortener-deployment.yaml
    ├── shortener-service.yaml
    ├── shortener-config.yaml    # ConfigMap + Secret
    ├── analytics-deployment.yaml
    ├── analytics-service.yaml
    └── analytics-config.yaml
```

Файлы в `templates/` которые начинаются с `_` (подчёркивание) — вспомогательные. Helm их не рендерит напрямую, а только если другой шаблон вызовет `include`.

---

## Chart.yaml

```yaml
apiVersion: v2           # версия Helm API (v2 для Helm 3)
name: url-shortener      # имя чарта
description: Local Kubernetes chart for the sandbox URL shortener services.
type: application        # application (деплоит приложение) или library (только helpers)
version: 0.1.0           # версия самого чарта (SemVer)
appVersion: "0.1.0"      # версия приложения — информационное поле
```

`version` и `appVersion` — разные вещи:
- `version` — версия чарта (меняется когда меняется сам шаблон)
- `appVersion` — версия деплоимого приложения (информационное поле)

---

## values.yaml — параметры чарта

`values.yaml` — это объект с любой структурой. Внутри шаблонов к нему обращаются через `.Values`.

```yaml
# values.yaml учебного чарта (локальный kind-стенд)
global:
  appEnv: local
  imagePullPolicy: IfNotPresent   # не тянуть образ если он уже есть на ноде

shortener:
  replicaCount: 1
  image:
    repository: sandbox-url-shortener/shortener
    tag: local
  service:
    type: ClusterIP
    port: 8080
    nodePort: null               # null = значение не задано
  env:
    APP_PORT: "8080"
    LOG_LEVEL: INFO
    REDIS_ADDR: host.docker.internal:6379
    KAFKA_PRODUCER_ENABLED: "false"
  secretEnv:
    DB_DSN: postgres://shortener:shortener@host.docker.internal:5432/shortener?sslmode=disable
  probes:
    liveness:
      path: /health/live
      initialDelaySeconds: 5
      periodSeconds: 10
      timeoutSeconds: 2
      failureThreshold: 3
    readiness:
      path: /health/ready
      initialDelaySeconds: 5
      periodSeconds: 10
      failureThreshold: 6
  resources: {}                  # {} = пустой объект, resource limits не заданы
```

### Переопределение значений — values-kind.yaml

Можно создать любое количество файлов `values-*.yaml`. Они накладываются поверх `values.yaml`:

```yaml
# values-kind.yaml — только то, что отличается от values.yaml
global:
  appEnv: local-k8s              # переопределить одно значение

shortener:
  service:
    type: NodePort               # в kind нет LoadBalancer, нужен NodePort
    nodePort: 30080              # статичный порт для kind extraPortMappings
  env:
    BASE_URL: http://localhost:18080

analytics:
  service:
    type: NodePort
    nodePort: 30090
```

При `helm upgrade --install ... -f values-kind.yaml` Helm сначала берёт `values.yaml`, затем мержит поверх `values-kind.yaml`. Итоговые values — объединение обоих файлов.

---

## Template синтаксис

Helm использует шаблонизатор Go (`text/template`) с дополнениями от библиотеки Sprig.

### Основной синтаксис

```yaml
# {{ }} — блок шаблонизатора
# .Values — объект со всеми values
# . — текущий контекст (обычно корневой объект чарта)

replicas: {{ .Values.shortener.replicaCount }}
# → replicas: 1
```

Дефис убирает пробелы вокруг блока:
```yaml
{{- .Values.name -}}   # убрать пробел/перенос до и после
{{- .Values.name }}    # убрать только слева
```

### Объекты в контексте шаблона

```yaml
# .Values — значения из values.yaml (и переопределений)
image: "{{ .Values.shortener.image.repository }}:{{ .Values.shortener.image.tag }}"
# → image: "sandbox-url-shortener/shortener:local"

# .Release — информация о release
name: {{ .Release.Name }}         # имя release (задаётся при helm install)
namespace: {{ .Release.Namespace }}
service: {{ .Release.Service }}   # "Helm"

# .Chart — данные из Chart.yaml
version: {{ .Chart.Version }}     # "0.1.0"
appVersion: {{ .Chart.AppVersion }}

# .Files — доступ к файлам в чарте (не шаблонам)
config: {{ .Files.Get "config/app.json" }}
```

### Пайплайны и функции

Пайплайн — передача значения через функции с помощью `|`:

```yaml
# quote — обернуть в кавычки (строка → "строка")
name: {{ .Values.shortener.env.LOG_LEVEL | quote }}
# → name: "INFO"

# nindent N — добавить N пробелов отступа перед каждой строкой (+ перенос в начале)
labels:
  {{- include "url-shortener.labels" . | nindent 4 }}
# → labels:
#       helm.sh/chart: url-shortener-0.1.0
#       app.kubernetes.io/name: url-shortener
#       ...

# indent N — то же но без переноса в начале
# upper / lower — изменить регистр
# default — значение по умолчанию если пустое
port: {{ .Values.shortener.service.port | default 8080 }}

# toYaml — сериализовать объект в YAML (нужно для вложенных структур)
resources:
  {{- toYaml .Values.shortener.resources | nindent 12 }}
# Если resources: {limits: {cpu: "1", memory: "512Mi"}}
# →   resources:
#       limits:
#         cpu: "1"
#         memory: 512Mi

# trunc — обрезать строку
{{ .Release.Name | trunc 63 }}  # имена в K8s ограничены 63 символами

# trimSuffix — убрать суффикс
{{ "my-name-" | trimSuffix "-" }}  # → "my-name"

# contains — проверить вхождение
{{ contains "shortener" .Release.Name }}  # true/false

# printf — форматирование строки
{{ printf "%s-%s" .Release.Name "shortener" }}  # → "sandbox-url-shortener-shortener"
```

### if / else

```yaml
# Простое условие
{{- if .Values.shortener.service.nodePort }}
nodePort: {{ .Values.shortener.service.nodePort }}
{{- end }}

# Условие с and/eq/not
{{- if and (eq .Values.shortener.service.type "NodePort") .Values.shortener.service.nodePort }}
nodePort: {{ .Values.shortener.service.nodePort }}
{{- end }}

# eq — равенство, ne — неравенство, lt/le/gt/ge — сравнение чисел
# and / or / not — логические операторы

{{- if not .Values.shortener.service.nodePort }}
# nodePort не задан
{{- end }}
```

### range — итерация

```yaml
# range по map — $key и $value
data:
  APP_ENV: {{ .Values.global.appEnv | quote }}
  {{- range $key, $value := .Values.shortener.env }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
# Итерирует по всем ключам shortener.env и выводит их как поля ConfigMap

# range по списку
{{- range .Values.shortener.volumeMounts }}
- name: {{ .name }}
  mountPath: {{ .mountPath }}
{{- end }}
```

### Переменные

```yaml
# Присвоить значение переменной
{{- $name := include "url-shortener.fullname" . -}}
name: {{ $name }}-config

# Переменная из range
{{- range $key, $value := .Values.env }}
  {{ $key }}: {{ $value | quote }}
{{- end }}
```

---

## _helpers.tpl — переиспользуемые шаблоны

`_helpers.tpl` содержит именованные шаблоны — фрагменты которые можно вызывать из любого шаблона чарта. Файл начинается с `_` чтобы Helm не рендерил его напрямую.

```go
{{/*
  Именованный шаблон определяется через define.
  Вызывается через include.
  Комментарии: {{/* ... */}}
*/}}

{{- define "url-shortener.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}
```

Разбор построчно:
```
{{- define "url-shortener.name" -}}
  │                                │
  └── начало именованного шаблона  └── дефис убирает пробелы/переносы

{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
            │            │                      │              │
            │            └── если задан nameOverride в values — использовать его
            └── иначе использовать Chart.Name ("url-shortener")
                                                  └── обрезать до 63 символов
                                                                 └── убрать trailing "-"
{{- end -}}
  └── конец определения
```

```go
{{- define "url-shortener.fullname" -}}
{{- if .Values.fullnameOverride -}}
  {{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
  {{- $name := default .Chart.Name .Values.nameOverride -}}
  {{- if contains $name .Release.Name -}}
    {{- .Release.Name | trunc 63 | trimSuffix "-" -}}
    {{/*
      Если release уже содержит имя чарта — не дублировать.
      Release "url-shortener" + chart "url-shortener" → просто "url-shortener"
      а не "url-shortener-url-shortener"
    */}}
  {{- else -}}
    {{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
    {{/*
      Иначе: "sandbox-url-shortener" + "url-shortener" → "sandbox-url-shortener-url-shortener"
      (обрезается до 63 символов)
    */}}
  {{- end -}}
{{- end -}}
{{- end -}}
```

```go
{{- define "url-shortener.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "url-shortener.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}
```

Стандартные Kubernetes labels. `replace "+" "_"` — потому что `+` не допустим в label values.

```go
{{/*
  Кастомный шаблон с параметром.
  Вызывается через: include "url-shortener.serviceName" (dict "root" . "service" "shortener")
  dict создаёт map: {"root": <текущий контекст>, "service": "shortener"}
*/}}
{{- define "url-shortener.serviceName" -}}
{{- printf "%s-%s" (include "url-shortener.fullname" .root) .service | trunc 63 | trimSuffix "-" -}}
{{- end -}}
```

Вызов:
```yaml
# В шаблоне:
name: {{ include "url-shortener.serviceName" (dict "root" . "service" "shortener") }}

# Что происходит:
# 1. (dict "root" . "service" "shortener") создаёт map {"root": ., "service": "shortener"}
# 2. Этот map становится контекстом (.) внутри serviceName
# 3. .root = исходный контекст чарта, .service = "shortener"
# 4. include "url-shortener.fullname" .root — вызвать fullname с исходным контекстом

# Результат (при release name "sandbox-url-shortener"):
# → "sandbox-url-shortener-shortener"
```

Почему `(dict "root" . "service" "shortener")` а не просто `.`:
В Helm именованный шаблон принимает один аргумент — контекст `.`. Если нужно передать несколько параметров — упаковать их в `dict`.

### include vs template

```yaml
# include — вернуть строку (можно пайплайнить дальше)
labels:
  {{- include "url-shortener.labels" . | nindent 4 }}

# template — вывести напрямую (нельзя пайплайнить)
# template "url-shortener.labels" .  ← так не получится пропустить через nindent
```

Правило: всегда `include`, не `template`.

---

## ConfigMap и Secret

### ConfigMap — незащищённые конфигурации

ConfigMap хранит конфигурацию в открытом виде (не для паролей).

```yaml
# templates/shortener-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "url-shortener.serviceName" (dict "root" . "service" "shortener") }}-config
  # → "sandbox-url-shortener-shortener-config"
  labels:
    {{- include "url-shortener.labels" . | nindent 4 }}
    app.kubernetes.io/component: shortener
data:
  APP_ENV: {{ .Values.global.appEnv | quote }}
  {{- range $key, $value := .Values.shortener.env }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
```

Что рендерится при `helm template`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sandbox-url-shortener-shortener-config
  labels:
    helm.sh/chart: url-shortener-0.1.0
    app.kubernetes.io/name: url-shortener
    app.kubernetes.io/instance: sandbox-url-shortener
    app.kubernetes.io/managed-by: Helm
    app.kubernetes.io/component: shortener
data:
  APP_ENV: "local"
  APP_PORT: "8080"
  LOG_LEVEL: "INFO"
  BASE_URL: "http://localhost:18080"
  REDIS_ADDR: "host.docker.internal:6379"
  # ... все ключи из shortener.env
```

### Secret — защищённые конфигурации

Secret отличается от ConfigMap двумя вещами:
1. Kubernetes хранит значения в base64 (но это не шифрование — просто кодирование)
2. Kubernetes не логирует их содержимое при `describe`

```yaml
# Продолжение того же файла shortener-config.yaml (после ---)
---
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "url-shortener.serviceName" (dict "root" . "service" "shortener") }}-secret
  labels:
    {{- include "url-shortener.labels" . | nindent 4 }}
    app.kubernetes.io/component: shortener
type: Opaque   # Opaque = произвольные данные (в отличие от kubernetes.io/tls и др.)
stringData:    # stringData — передать строки, Kubernetes сам закодирует в base64
  {{- range $key, $value := .Values.shortener.secretEnv }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
```

**stringData vs data:**

```yaml
# stringData — передать строку как есть, K8s сохранит в base64
stringData:
  DB_DSN: "postgres://user:pass@host/db"

# data — передать уже закодированное в base64 значение
data:
  DB_DSN: cG9zdGdyZXM6Ly91c2VyOnBhc3NAaG9zdC9kYg==

# В шаблонах удобнее stringData — читается и не требует ручного кодирования
```

При `helm template` Secret со stringData выглядит так:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sandbox-url-shortener-shortener-secret
type: Opaque
stringData:
  DB_DSN: "postgres://shortener:shortener@host.docker.internal:5432/shortener?sslmode=disable"
```

После `kubectl apply` Kubernetes конвертирует stringData → data (base64).

---

## Deployment

Deployment — основной объект для запуска приложений. Описывает желаемое состояние: сколько реплик, какой образ, как проверять готовность.

```yaml
# templates/shortener-deployment.yaml (полная аннотированная версия)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "url-shortener.serviceName" (dict "root" . "service" "shortener") }}
  # → "sandbox-url-shortener-shortener"
  labels:
    {{- include "url-shortener.labels" . | nindent 4 }}
    app.kubernetes.io/component: shortener   # дополнительный label для этого сервиса
spec:
  replicas: {{ .Values.shortener.replicaCount }}
  # → replicas: 1

  selector:
    matchLabels:
      {{- include "url-shortener.selectorLabels" . | nindent 6 }}
      app.kubernetes.io/component: shortener
  # selector — по каким labels Deployment находит свои Pods.
  # Должен совпадать с template.metadata.labels ниже.
  # selectorLabels содержит только app.kubernetes.io/name и app.kubernetes.io/instance —
  # достаточно уникальная пара для этого release.

  template:             # шаблон Pod'а — каждая реплика создаётся по этому шаблону
    metadata:
      labels:
        {{- include "url-shortener.selectorLabels" . | nindent 8 }}
        app.kubernetes.io/component: shortener
    spec:
      containers:
        - name: shortener   # имя контейнера внутри Pod'а
          image: "{{ .Values.shortener.image.repository }}:{{ .Values.shortener.image.tag }}"
          # → image: "sandbox-url-shortener/shortener:local"

          imagePullPolicy: {{ .Values.global.imagePullPolicy }}
          # IfNotPresent — использовать локальный образ если есть (нужно для kind)
          # Always       — всегда тянуть из registry (для prod)
          # Never        — только локальный, никогда не тянуть

          ports:
            - name: http                  # имя порта — используется в probes и Service
              containerPort: {{ .Values.shortener.service.port }}
              # containerPort — информационное поле, не открывает порт сам по себе
              # (все порты контейнера всегда открыты внутри кластера)

          envFrom:                        # загрузить env vars из ресурсов K8s
            - configMapRef:
                name: {{ include "url-shortener.serviceName" (dict "root" . "service" "shortener") }}-config
                # загрузить все ключи ConfigMap как env переменные
            - secretRef:
                name: {{ include "url-shortener.serviceName" (dict "root" . "service" "shortener") }}-secret
                # загрузить все ключи Secret как env переменные

          # После этого в контейнере будут доступны DB_DSN, APP_PORT, LOG_LEVEL и т.д.

          livenessProbe:
            httpGet:
              path: {{ .Values.shortener.probes.liveness.path }}   # /health/live
              port: http   # ссылка на именованный порт выше
            initialDelaySeconds: {{ .Values.shortener.probes.liveness.initialDelaySeconds }}
            # initialDelaySeconds — ждать N секунд после старта контейнера перед первой проверкой
            periodSeconds: {{ .Values.shortener.probes.liveness.periodSeconds }}
            # periodSeconds — интервал между проверками
            timeoutSeconds: {{ .Values.shortener.probes.liveness.timeoutSeconds }}
            # timeoutSeconds — максимальное время ожидания ответа
            failureThreshold: {{ .Values.shortener.probes.liveness.failureThreshold }}
            # failureThreshold — сколько раз можно провалить проверку перед рестартом
            # При failureThreshold=3: 3 ошибки подряд → контейнер перезапускается

          readinessProbe:
            httpGet:
              path: {{ .Values.shortener.probes.readiness.path }}  # /health/ready
              port: http
            initialDelaySeconds: {{ .Values.shortener.probes.readiness.initialDelaySeconds }}
            periodSeconds: {{ .Values.shortener.probes.readiness.periodSeconds }}
            timeoutSeconds: {{ .Values.shortener.probes.readiness.timeoutSeconds }}
            failureThreshold: {{ .Values.shortener.probes.readiness.failureThreshold }}
            # При failureThreshold=6: 6 ошибок → Pod убирается из Service endpoints
            # (трафик не идёт, но контейнер НЕ перезапускается)

          resources:
            {{- toYaml .Values.shortener.resources | nindent 12 }}
            # resources: {} → рендерится как пустой блок (нет limits/requests)
            # В prod стоит задать:
            # resources:
            #   requests: { cpu: "100m", memory: "128Mi" }
            #   limits:   { cpu: "500m", memory: "512Mi" }
```

Что получается после `helm template` (с values-kind.yaml):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sandbox-url-shortener-shortener
  labels:
    helm.sh/chart: url-shortener-0.1.0
    app.kubernetes.io/name: url-shortener
    app.kubernetes.io/instance: sandbox-url-shortener
    app.kubernetes.io/managed-by: Helm
    app.kubernetes.io/component: shortener
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: url-shortener
      app.kubernetes.io/instance: sandbox-url-shortener
      app.kubernetes.io/component: shortener
  template:
    metadata:
      labels:
        app.kubernetes.io/name: url-shortener
        app.kubernetes.io/instance: sandbox-url-shortener
        app.kubernetes.io/component: shortener
    spec:
      containers:
        - name: shortener
          image: "sandbox-url-shortener/shortener:local"
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: 8080
          envFrom:
            - configMapRef:
                name: sandbox-url-shortener-shortener-config
            - secretRef:
                name: sandbox-url-shortener-shortener-secret
          livenessProbe:
            httpGet:
              path: /health/live
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
            timeoutSeconds: 2
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /health/ready
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
            timeoutSeconds: 2
            failureThreshold: 6
          resources: {}
```

### Связка selector / template / labels

Это критически важно понять:

```
Deployment.spec.selector.matchLabels = { app: myapp }
                │
                └── Deployment управляет Pod'ами с этими labels
                    (находит их по selector)

Deployment.spec.template.metadata.labels = { app: myapp }
                │
                └── Все Pod'ы создаются с этими labels
                    (должны совпадать с selector!)

# Если labels в template не совпадают с selector — Deployment не сможет
# найти свои Pod'ы и будет создавать новые бесконечно.
```

В чарте используются отдельные `selectorLabels` (name + instance) — они уникальны для release и не меняются. Полные `labels` включают также версию чарта и managed-by, которые могут меняться — их нельзя использовать в selector (нельзя менять selector после создания Deployment).

---

## Service

Service — абстракция над Pod'ами. Даёт стабильный DNS-адрес и балансировку нагрузки между репликами.

```yaml
# templates/shortener-service.yaml (аннотированный)
apiVersion: v1
kind: Service
metadata:
  name: {{ include "url-shortener.serviceName" (dict "root" . "service" "shortener") }}
  # → "sandbox-url-shortener-shortener"
  labels:
    {{- include "url-shortener.labels" . | nindent 4 }}
    app.kubernetes.io/component: shortener
spec:
  type: {{ .Values.shortener.service.type }}
  # ClusterIP — доступен только внутри кластера (по умолчанию)
  # NodePort   — доступен на порту каждой ноды кластера (для локальной разработки)
  # LoadBalancer — создаёт внешний балансировщик (для облачных кластеров)

  selector:
    {{- include "url-shortener.selectorLabels" . | nindent 4 }}
    app.kubernetes.io/component: shortener
  # selector — по каким labels Service находит Pod'ы для балансировки
  # Должен совпадать с labels Pod'ов из Deployment

  ports:
    - name: http
      port: {{ .Values.shortener.service.port }}
      # port — порт самого Service (на котором он принимает трафик внутри кластера)
      targetPort: http
      # targetPort — порт контейнера куда перенаправить трафик
      # "http" — ссылка на именованный порт контейнера (containerPort с name: http)

      {{- if and (eq .Values.shortener.service.type "NodePort") .Values.shortener.service.nodePort }}
      nodePort: {{ .Values.shortener.service.nodePort }}
      # nodePort — только для type: NodePort
      # Если указан — использовать этот конкретный порт (30080)
      # Если не указан — Kubernetes назначит случайный из диапазона 30000-32767
      {{- end }}
```

### ClusterIP vs NodePort в локальном kind-окружении

В `values.yaml` (по умолчанию) — `ClusterIP`: сервис доступен только внутри кластера.

В `values-kind.yaml` — `NodePort: 30080`: сервис доступен на каждой ноде по порту 30080. Kind пробрасывает `containerPort: 30080 → hostPort: 18080` в `cluster.yaml`.

```
Запрос:  curl localhost:18080
           │
           └── kind extraPortMapping: host:18080 → node:30080
                                                      │
                                                      └── NodePort Service: node:30080 → pod:8080
                                                                                           │
                                                                                           └── приложение
```

### DNS внутри кластера

Сервисы доступны по DNS-имени из других Pod'ов:

```
<service-name>.<namespace>.svc.cluster.local
# или просто <service-name> если Pod в том же namespace

sandbox-url-shortener-shortener.sandbox-url-shortener.svc.cluster.local
# или: sandbox-url-shortener-shortener (если из того же namespace)
```

---

## Helm команды

### helm template — рендерить без применения

```bash
# Рендерить чарт и вывести на stdout (ничего не применяет к кластеру)
helm template sandbox-url-shortener ./deploy/helm/url-shortener

# С override values
helm template sandbox-url-shortener ./deploy/helm/url-shortener \
  -f ./deploy/helm/url-shortener/values-kind.yaml

# Рендерить конкретный шаблон
helm template sandbox-url-shortener ./deploy/helm/url-shortener \
  --show-only templates/shortener-deployment.yaml

# С указанием namespace (влияет на .Release.Namespace в шаблонах)
helm template sandbox-url-shortener ./deploy/helm/url-shortener \
  --namespace sandbox-url-shortener
```

`helm template` — главный инструмент отладки: показывает реальные YAML, которые будут применены.

### helm upgrade --install

```bash
# Установить или обновить release
helm upgrade --install \
  sandbox-url-shortener \               # имя release
  ./deploy/helm/url-shortener \         # путь к чарту
  --namespace sandbox-url-shortener \   # namespace в K8s
  --create-namespace \                  # создать namespace если не существует
  -f ./deploy/helm/url-shortener/values-kind.yaml  # override values

# Передать одно значение без файла
helm upgrade --install myapp ./chart \
  --set shortener.replicaCount=3 \
  --set shortener.image.tag=v1.2.0

# Дождаться готовности (ждёт пока все Pod'ы пройдут readiness probe)
helm upgrade --install myapp ./chart --wait --timeout 5m
```

`upgrade --install` = установить если не существует, обновить если существует. Удобно для CI/CD — не нужно разделять первую установку и обновления.

### Просмотр состояния

```bash
# Список всех release в кластере
helm list
helm list -n sandbox-url-shortener   # только в этом namespace
helm list --all-namespaces

# Статус release
helm status sandbox-url-shortener -n sandbox-url-shortener

# Посмотреть values с которыми установлен release
helm get values sandbox-url-shortener -n sandbox-url-shortener
helm get values sandbox-url-shortener -n sandbox-url-shortener --all  # включая defaults

# Посмотреть реальные YAML которые Helm применил
helm get manifest sandbox-url-shortener -n sandbox-url-shortener

# История release (версии)
helm history sandbox-url-shortener -n sandbox-url-shortener
```

### Откат

```bash
# Откатить к предыдущей версии
helm rollback sandbox-url-shortener -n sandbox-url-shortener

# Откатить к конкретной версии
helm rollback sandbox-url-shortener 2 -n sandbox-url-shortener
# 2 — номер revision из helm history
```

### Удаление

```bash
# Удалить release (удаляет все K8s ресурсы созданные Helm)
helm uninstall sandbox-url-shortener -n sandbox-url-shortener
```

---

## Releases и namespaces

**Release** — установленный экземпляр чарта. Helm хранит метаданные release как Secret в том же namespace.

```bash
# Два release из одного чарта в разных namespaces
helm upgrade --install myapp-dev  ./chart --namespace dev
helm upgrade --install myapp-prod ./chart --namespace prod

# helm list покажет оба
helm list --all-namespaces
# NAME        NAMESPACE  REVISION  STATUS
# myapp-dev   dev        3         deployed
# myapp-prod  prod       1         deployed
```

**Namespace** — логическое разделение ресурсов внутри кластера. Ресурсы в разных namespace изолированы (разные RBAC права, разные ConfigMap/Secret и т.д.).

```bash
# Создать namespace
kubectl create namespace sandbox-url-shortener
# или при helm: --create-namespace

# Посмотреть ресурсы в namespace
kubectl -n sandbox-url-shortener get all
kubectl -n sandbox-url-shortener get pods,svc,deploy,configmap,secret
```

---

## Антипаттерны

**Хранить настоящие секреты в values.yaml**

```yaml
# values.yaml — ПЛОХО
secretEnv:
  DB_DSN: "postgres://admin:real-password@prod-db:5432/app"
  # Этот файл в git → пароль утёк
```

Решения:
- helm-secrets (шифрование через SOPS)
- External Secrets Operator (читать из Secret Manager)
- передавать при деплое через `--set` из CI/CD переменных (не коммитить)

**Одинаковые labels в selector и полных labels**

```yaml
# ПЛОХО — version в selector
selector:
  matchLabels:
    app: myapp
    version: "1.0"  # при обновлении версии → конфликт (нельзя изменить selector)
```

**Не задавать resources в prod**

```yaml
# ПЛОХО для prod
resources: {}

# ХОРОШО — K8s знает сколько ресурсов выделить поду
resources:
  requests:
    cpu: "100m"
    memory: "128Mi"
  limits:
    cpu: "500m"
    memory: "512Mi"
```

**imagePullPolicy: IfNotPresent в prod**

В kind нужен `IfNotPresent` (образы загружены через `kind load`). В prod нужен `Always` — иначе обновление тега `:latest` не приведёт к обновлению образа.

**Не использовать `helm template` перед apply**

`helm template` перед `helm upgrade --install` показывает ошибки шаблонов до применения к кластеру — пропуск этого шага означает отладку прямо на кластере.

---

## Interview-ready answer

**1. Что такое chart, values и release?**

- Chart — пакет шаблонов манифестов; values — входные параметры (values.yaml + переопределения `-f`/`--set`, мержатся поверх); release — конкретная установка чарта в кластер со своей историей ревизий (метаданные Helm хранит как Secret в namespace). Один чарт → много releases с разными values.

**2. Чем `version` отличается от `appVersion` в Chart.yaml?**

- `version` — версия самого чарта (меняется при изменении шаблонов, SemVer), `appVersion` — версия деплоящегося приложения, чисто информационное поле. Bump-ается независимо.

**3. Чем `include` отличается от `template` и зачем `dict`?**

- Оба вызывают именованный шаблон, но `include` возвращает строку, которую можно пайплайнить (`| nindent 4`), а `template` выводит напрямую — поэтому используется только `include`. Именованный шаблон принимает один аргумент-контекст; несколько параметров передаются упаковкой в `dict "root" . "service" "x"`.

**4. Почему нельзя менять labels в selector?**

- `spec.selector` Deployment immutable: если полные labels (с версией чарта) попадут в selector, любое обновление версии сломает деплой. Поэтому в selector идут только стабильные `selectorLabels` (name + instance), а изменяемые labels — только в metadata.

**5. Как Helm работает с секретами?**

- Настоящие секреты в values.yaml — антипаттерн (файл в git). Варианты: helm-secrets (SOPS-шифрование values), External Secrets Operator (подтягивает из Vault/Secret Manager), передача через `--set` из переменных CI/CD.
