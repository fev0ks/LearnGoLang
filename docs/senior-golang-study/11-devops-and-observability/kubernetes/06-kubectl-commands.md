# kubectl: команды с примерами вывода

Практический справочник команд kubectl для повседневной работы. Каждая команда — с реальным примером вывода.

## Содержание

- [Контексты и кластеры](#контексты-и-кластеры)
- [Namespace](#namespace)
- [Pods: просмотр и статус](#pods-просмотр-и-статус)
- [Pods: логи](#pods-логи)
- [Pods: exec и отладка](#pods-exec-и-отладка)
- [Deployments и rollout](#deployments-и-rollout)
- [Services и port-forward](#services-и-port-forward)
- [ConfigMap и Secret](#configmap-и-secret)
- [Ресурсы: top и requests/limits](#ресурсы-top-и-requestslimits)
- [Events: что происходит в кластере](#events-что-происходит-в-кластере)
- [Labels и selectors](#labels-и-selectors)
- [Apply, delete, scale](#apply-delete-scale)
- [Полезные флаги и алиасы](#полезные-флаги-и-алиасы)
- [Быстрый troubleshooting workflow](#быстрый-troubleshooting-workflow)

---

## Контексты и кластеры

Контекст = (кластер + пользователь + namespace). Переключение между кластерами — через контексты.

```bash
# Посмотреть все доступные контексты
kubectl config get-contexts
```
```
CURRENT   NAME                                    CLUSTER                              AUTHINFO                            NAMESPACE
          docker-desktop                          docker-desktop                       docker-desktop
*         gke_skibookers_europe-west4_skiprod     gke_skibookers_europe-west4_skiprod  gke_skibookers_europe-west4_skiprod
          minikube                                minikube                             minikube                            default
```
Звёздочка — текущий активный контекст.

```bash
# Переключить контекст
kubectl config use-context gke_skibookers_europe-west4_skiprod
```
```
Switched to context "gke_skibookers_europe-west4_skiprod".
```

```bash
# Посмотреть текущий контекст
kubectl config current-context
```
```
gke_skibookers_europe-west4_skiprod
```

```bash
# Получить credentials для GKE кластера (Google Cloud)
export KUBECONFIG=/Users/fev0ks/.kube/config
gcloud container clusters get-credentials skiprod --region europe-west4 --project skibookers
```
```
Fetching cluster endpoint and auth data.
kubeconfig entry generated for skiprod.
```

```bash
# Посмотреть полный kubeconfig
kubectl config view

# Посмотреть только текущий контекст с деталями
kubectl config view --minify
```

```bash
# Временно использовать другой namespace для всех команд
kubectl config set-context --current --namespace=my-namespace

# Или использовать --namespace/-n флаг на каждую команду (предпочтительно)
kubectl get pods -n production
```

---

## Namespace

```bash
# Список всех namespace
kubectl get namespaces
```
```
NAME              STATUS   AGE
default           Active   45d
kube-system       Active   45d
kube-public       Active   45d
kube-node-lease   Active   45d
production        Active   30d
staging           Active   30d
monitoring        Active   20d
```

```bash
# Сокращение: ns вместо namespaces
kubectl get ns

# Посмотреть ресурсы во всех namespace одновременно
kubectl get pods --all-namespaces
# Или короче:
kubectl get pods -A
```
```
NAMESPACE     NAME                                     READY   STATUS    RESTARTS   AGE
kube-system   coredns-565d847f94-8zvcj                 1/1     Running   0          45d
kube-system   kube-proxy-j7g4s                         1/1     Running   0          45d
production    my-service-7d6b8f9c4-xkp2m               1/1     Running   0          3h
production    my-service-7d6b8f9c4-rtq8n               1/1     Running   0          3h
staging       my-service-6c5b7d8e3-lmn4p               1/1     Running   1          1d
```

---

## Pods: просмотр и статус

```bash
# Список podов в namespace (default)
kubectl get pods
```
```
NAME                           READY   STATUS    RESTARTS   AGE
api-server-7d6b8f9c4-xkp2m    1/1     Running   0          3h
api-server-7d6b8f9c4-rtq8n    1/1     Running   0          3h
worker-5c4d3e2f1-abc12         1/1     Running   2          1d
postgres-0                     1/1     Running   0          10d
```

```bash
# Список в конкретном namespace
kubectl get pods -n production

# С дополнительной информацией: IP, нода
kubectl get pods -o wide
```
```
NAME                           READY   STATUS    RESTARTS   AGE   IP            NODE                    NOMINATED NODE
api-server-7d6b8f9c4-xkp2m    1/1     Running   0          3h    10.96.1.14    gke-skiprod-pool-abc12   <none>
api-server-7d6b8f9c4-rtq8n    1/1     Running   0          3h    10.96.1.15    gke-skiprod-pool-def34   <none>
```

```bash
# Детальная информация о pod
kubectl describe pod api-server-7d6b8f9c4-xkp2m -n production
```
```
Name:             api-server-7d6b8f9c4-xkp2m
Namespace:        production
Priority:         0
Node:             gke-skiprod-pool-abc12/10.132.0.5
Start Time:       Mon, 22 Apr 2026 09:00:00 +0000
Labels:           app=api-server
                  pod-template-hash=7d6b8f9c4
Status:           Running
IP:               10.96.1.14
Containers:
  api-server:
    Image:          gcr.io/skibookers/api-server:v1.2.3
    Port:           8080/TCP
    Limits:
      cpu:     500m
      memory:  512Mi
    Requests:
      cpu:     100m
      memory:  128Mi
    Liveness:   http-get http://:8080/healthz delay=30s timeout=5s period=10s
    Readiness:  http-get http://:8080/ready delay=5s timeout=3s period=5s
    Environment:
      DATABASE_URL:  <set to the key 'database_url' in secret 'api-secrets'>
      LOG_LEVEL:     info
    Mounts:
      /var/run/secrets/kubernetes.io/serviceaccount from default-token-xyz (ro)
Conditions:
  Type              Status
  Initialized       True
  Ready             True
  ContainersReady   True
  PodScheduled      True
Events:
  Type    Reason     Age   From               Message
  ----    ------     ----  ----               -------
  Normal  Scheduled  3h    default-scheduler  Successfully assigned production/api-server-7d6b8f9c4-xkp2m to gke-skiprod-pool-abc12
  Normal  Pulled     3h    kubelet            Container image "gcr.io/skibookers/api-server:v1.2.3" already present on machine
  Normal  Started    3h    kubelet            Started container api-server
```

```bash
# Наблюдать за изменениями в реальном времени
kubectl get pods -n production -w
```
```
NAME                           READY   STATUS    RESTARTS   AGE
api-server-7d6b8f9c4-xkp2m    1/1     Running   0          3h
api-server-new-8e7c9d0b5-zyx1  0/1     Pending   0          0s
api-server-new-8e7c9d0b5-zyx1  0/1     ContainerCreating   0          2s
api-server-new-8e7c9d0b5-zyx1  1/1     Running   0          8s
api-server-7d6b8f9c4-xkp2m    1/1     Terminating   0       3h
```

```bash
# Вывести в JSON / YAML
kubectl get pod api-server-7d6b8f9c4-xkp2m -o json
kubectl get pod api-server-7d6b8f9c4-xkp2m -o yaml

# Вытащить конкретное поле (jsonpath)
kubectl get pod api-server-7d6b8f9c4-xkp2m -o jsonpath='{.status.podIP}'
# → 10.96.1.14

kubectl get pods -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.phase}{"\n"}{end}'
# → api-server-7d6b8f9c4-xkp2m  Running
#   worker-5c4d3e2f1-abc12       Running
```

---

## Pods: логи

```bash
# Логи пода (последние N строк)
kubectl logs api-server-7d6b8f9c4-xkp2m --tail=100
```
```
2026-04-22T09:00:01Z INFO  server started addr=:8080
2026-04-22T09:00:05Z INFO  connected to database host=postgres port=5432
2026-04-22T09:15:23Z INFO  request method=GET path=/api/users status=200 latency=12ms
2026-04-22T09:15:24Z ERROR failed to process request error="context deadline exceeded"
```

```bash
# Follow (stream) — как tail -f
kubectl logs api-server-7d6b8f9c4-xkp2m -f

# Логи за последние 30 минут
kubectl logs api-server-7d6b8f9c4-xkp2m --since=30m

# Логи с конкретного времени
kubectl logs api-server-7d6b8f9c4-xkp2m --since-time="2026-04-22T09:00:00Z"

# Логи предыдущего контейнера (если pod перестартовал)
kubectl logs api-server-7d6b8f9c4-xkp2m --previous
```

```bash
# Pod с несколькими контейнерами — указать контейнер
kubectl logs api-server-7d6b8f9c4-xkp2m -c api-server
kubectl logs api-server-7d6b8f9c4-xkp2m -c sidecar-proxy

# Логи всех pod с label app=api-server
kubectl logs -l app=api-server --tail=50
kubectl logs -l app=api-server -f  # stream со всех реплик
```

```bash
# Логи deployment (все поды сразу)
kubectl logs deployment/api-server --tail=20

# Stern — удобная утилита для логов нескольких подов
# brew install stern
stern api-server -n production --tail=50
# → показывает логи всех подов с префиксом имени пода и цветом
```

---

## Pods: exec и отладка

```bash
# Зайти в контейнер (shell)
kubectl exec -it api-server-7d6b8f9c4-xkp2m -- /bin/sh
kubectl exec -it api-server-7d6b8f9c4-xkp2m -- /bin/bash  # если есть bash

# Выполнить команду без интерактивного режима
kubectl exec api-server-7d6b8f9c4-xkp2m -- env
kubectl exec api-server-7d6b8f9c4-xkp2m -- cat /etc/hosts
kubectl exec api-server-7d6b8f9c4-xkp2m -- curl -s http://localhost:8080/healthz
```
```
{"status":"ok","version":"v1.2.3"}
```

```bash
# В pod с несколькими контейнерами
kubectl exec -it api-server-7d6b8f9c4-xkp2m -c api-server -- /bin/sh

# Скопировать файл из/в pod
kubectl cp api-server-7d6b8f9c4-xkp2m:/app/config.yaml ./config-backup.yaml
kubectl cp ./local-config.yaml api-server-7d6b8f9c4-xkp2m:/tmp/config.yaml
```

```bash
# Запустить временный pod для отладки сети (нет curl в основном образе?)
kubectl run debug-pod --image=curlimages/curl:latest --restart=Never -it --rm -- \
  curl -s http://api-server:8080/healthz
```
```
{"status":"ok"}
pod "debug-pod" deleted
```

```bash
# Запустить netshoot для диагностики сети
kubectl run netshoot --image=nicolaka/netshoot -it --rm --restart=Never -n production -- bash
# Внутри: dig, nslookup, curl, tcpdump, ss, nmap — всё есть
```

```bash
# Посмотреть что внутри запущенного контейнера без exec (если нет shell)
kubectl debug -it api-server-7d6b8f9c4-xkp2m --image=busybox --target=api-server
# Ephemeral container — Go 1.16+ / K8s 1.23+
```

---

## Deployments и rollout

```bash
# Список deployments
kubectl get deployments -n production
```
```
NAME          READY   UP-TO-DATE   AVAILABLE   AGE
api-server    3/3     3            3           30d
worker        2/2     2            2           30d
```

```bash
# Детали deployment
kubectl describe deployment api-server -n production
```
```
Name:                   api-server
Namespace:              production
Replicas:               3 desired | 3 updated | 3 total | 3 available | 0 unavailable
StrategyType:           RollingUpdate
MinReadySeconds:        0
RollingUpdateStrategy:  25% max unavailable, 25% max surge
Pod Template:
  Labels:  app=api-server
  Containers:
   api-server:
    Image:  gcr.io/skibookers/api-server:v1.2.3
    ...
OldReplicaSets:  <none>
NewReplicaSet:   api-server-7d6b8f9c4 (3/3 replicas created)
Events:          <none>
```

```bash
# Статус rollout (во время деплоя)
kubectl rollout status deployment/api-server -n production
```
```
Waiting for deployment "api-server" rollout to finish: 1 of 3 updated replicas are available...
Waiting for deployment "api-server" rollout to finish: 2 of 3 updated replicas are available...
deployment "api-server" successfully rolled out
```

```bash
# История деплоев
kubectl rollout history deployment/api-server -n production
```
```
REVISION  CHANGE-CAUSE
1         kubectl apply --filename=deploy.yaml
2         kubectl set image deployment/api-server api-server=gcr.io/skibookers/api-server:v1.2.2
3         kubectl set image deployment/api-server api-server=gcr.io/skibookers/api-server:v1.2.3
```

```bash
# Детали конкретной ревизии
kubectl rollout history deployment/api-server --revision=2 -n production

# Откатить на предыдущую версию
kubectl rollout undo deployment/api-server -n production

# Откатить на конкретную ревизию
kubectl rollout undo deployment/api-server --to-revision=1 -n production
```
```
deployment.apps/api-server rolled back
```

```bash
# Обновить image (задеплоить новую версию)
kubectl set image deployment/api-server api-server=gcr.io/skibookers/api-server:v1.2.4 -n production

# Принудительный restart всех pod (без изменения image)
kubectl rollout restart deployment/api-server -n production
```
```
deployment.apps/api-server restarted
```

```bash
# Приостановить rollout (пауза деплоя)
kubectl rollout pause deployment/api-server -n production

# Возобновить
kubectl rollout resume deployment/api-server -n production
```

---

## Services и port-forward

```bash
# Список services
kubectl get services -n production
# Или сокращение:
kubectl get svc -n production
```
```
NAME          TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
api-server    ClusterIP   10.100.1.15     <none>        8080/TCP   30d
postgres      ClusterIP   10.100.1.20     <none>        5432/TCP   30d
kafka-ui      ClusterIP   10.100.1.25     <none>        8080/TCP   10d
```

```bash
# Детальная информация о сервисе (видно endpoints — какие pod зарегистрированы)
kubectl describe svc api-server -n production
```
```
Name:              api-server
Namespace:         production
Selector:          app=api-server
Type:              ClusterIP
IP:                10.100.1.15
Port:              http  8080/TCP
TargetPort:        8080/TCP
Endpoints:         10.96.1.14:8080,10.96.1.15:8080,10.96.1.16:8080
Session Affinity:  None
```

```bash
# Port-forward: пробросить service на локальный порт
# Формат: kubectl port-forward svc/<name> <local>:<remote> -n <namespace>

# Пробросить kafka-ui на localhost:8080
kubectl port-forward svc/kafka-ui 8080:8080 -n default

# Пробросить postgres на localhost:5433 (чтобы не конфликтовать с локальным)
kubectl port-forward svc/postgres 5433:5432 -n default

# Пробросить сервис на порт 9080 (удалённый порт — 9081)
kubectl port-forward svc/skibookers-platform-core 9080:9081 -n default
```
```
Forwarding from 127.0.0.1:5433 -> 5432
Forwarding from [::1]:5433 -> 5432
Handling connection for 5433
```

После этого: `psql -h localhost -p 5433 -U postgres`

```bash
# Port-forward к конкретному pod (а не через service)
kubectl port-forward pod/api-server-7d6b8f9c4-xkp2m 8080:8080 -n production

# Слушать на всех интерфейсах (по умолчанию только localhost)
kubectl port-forward svc/api-server 8080:8080 --address=0.0.0.0 -n production
```

```bash
# Endpoints — к каким pod идут запросы через service
kubectl get endpoints api-server -n production
```
```
NAME         ENDPOINTS                                            AGE
api-server   10.96.1.14:8080,10.96.1.15:8080,10.96.1.16:8080   30d
```

---

## ConfigMap и Secret

```bash
# Список configmap
kubectl get configmap -n production
```
```
NAME               DATA   AGE
api-config         3      30d
nginx-config       1      30d
kube-root-ca.crt   1      45d
```

```bash
# Посмотреть содержимое configmap
kubectl get configmap api-config -o yaml -n production
```
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: api-config
  namespace: production
data:
  LOG_LEVEL: "info"
  FEATURE_FLAGS: "new-ui=true,dark-mode=false"
  MAX_CONNECTIONS: "100"
```

```bash
# Список secrets
kubectl get secrets -n production
```
```
NAME                  TYPE                                  DATA   AGE
api-secrets           Opaque                                3      30d
default-token-xyz     kubernetes.io/service-account-token  3      45d
registry-credentials  kubernetes.io/dockerconfigjson        1      30d
```

```bash
# Посмотреть имена ключей в secret (значения — base64)
kubectl describe secret api-secrets -n production
```
```
Name:         api-secrets
Namespace:    production
Type:  Opaque

Data
====
database_url:  45 bytes
jwt_secret:    32 bytes
redis_url:     38 bytes
```

```bash
# Декодировать значение секрета
kubectl get secret api-secrets -o jsonpath='{.data.database_url}' -n production | base64 -d
# → postgres://user:password@postgres:5432/mydb

# Все значения сразу
kubectl get secret api-secrets -o go-template='{{range $k,$v := .data}}{{$k}}: {{$v | base64decode}}{{"\n"}}{{end}}' -n production
```

---

## Ресурсы: top и requests/limits

```bash
# Потребление CPU и памяти по pod (нужен metrics-server)
kubectl top pods -n production
```
```
NAME                           CPU(cores)   MEMORY(bytes)
api-server-7d6b8f9c4-xkp2m    45m          128Mi
api-server-7d6b8f9c4-rtq8n    38m          122Mi
worker-5c4d3e2f1-abc12         120m         256Mi
postgres-0                     15m          512Mi
```

```bash
# Потребление по нодам
kubectl top nodes
```
```
NAME                          CPU(cores)   CPU%   MEMORY(bytes)   MEMORY%
gke-skiprod-pool-abc12        823m         20%    3840Mi          50%
gke-skiprod-pool-def34        412m         10%    2048Mi          26%
gke-skiprod-pool-ghi56        1124m        28%    4352Mi          56%
```

```bash
# Посмотреть requests/limits у всех pod
kubectl get pods -n production -o custom-columns=\
'NAME:.metadata.name,CPU_REQ:.spec.containers[*].resources.requests.cpu,CPU_LIM:.spec.containers[*].resources.limits.cpu,MEM_REQ:.spec.containers[*].resources.requests.memory,MEM_LIM:.spec.containers[*].resources.limits.memory'
```
```
NAME                           CPU_REQ   CPU_LIM   MEM_REQ   MEM_LIM
api-server-7d6b8f9c4-xkp2m    100m      500m      128Mi     512Mi
worker-5c4d3e2f1-abc12         200m      1000m     256Mi     1Gi
```

---

## Events: что происходит в кластере

Events — первое место куда смотреть при проблемах с pod.

```bash
# События в namespace (отсортированы по времени)
kubectl get events -n production --sort-by='.lastTimestamp'
```
```
LAST SEEN   TYPE      REASON              OBJECT                                MESSAGE
5m          Normal    Scheduled           Pod/api-server-new-8e7c9d0b5-zyx1    Successfully assigned to gke-skiprod-pool-abc12
5m          Normal    Pulling             Pod/api-server-new-8e7c9d0b5-zyx1    Pulling image "gcr.io/skibookers/api-server:v1.2.4"
4m          Normal    Pulled              Pod/api-server-new-8e7c9d0b5-zyx1    Successfully pulled image
4m          Normal    Started             Pod/api-server-new-8e7c9d0b5-zyx1    Started container api-server
2m          Warning   BackOff             Pod/worker-5c4d3e2f1-abc12           Back-off restarting failed container
1m          Warning   OOMKilled           Pod/worker-5c4d3e2f1-abc12           Container worker exceeded memory limit
```

```bash
# Только Warning events
kubectl get events -n production --field-selector type=Warning

# События для конкретного объекта
kubectl get events -n production --field-selector involvedObject.name=api-server-7d6b8f9c4-xkp2m

# Наблюдать за событиями в реальном времени
kubectl get events -n production -w
```

---

## Labels и selectors

```bash
# Посмотреть labels у pod
kubectl get pods --show-labels -n production
```
```
NAME                           READY   STATUS    LABELS
api-server-7d6b8f9c4-xkp2m    1/1     Running   app=api-server,pod-template-hash=7d6b8f9c4,version=v1.2.3
worker-5c4d3e2f1-abc12         1/1     Running   app=worker,pod-template-hash=5c4d3e2f1
```

```bash
# Выбрать pod по label (selector)
kubectl get pods -l app=api-server -n production
kubectl get pods -l app=api-server,version=v1.2.3 -n production

# Отрицательный selector
kubectl get pods -l 'app!=worker' -n production

# Selector с in
kubectl get pods -l 'app in (api-server, worker)' -n production

# Добавить label к pod
kubectl label pod api-server-7d6b8f9c4-xkp2m debug=true -n production

# Удалить label
kubectl label pod api-server-7d6b8f9c4-xkp2m debug- -n production
```

---

## Apply, delete, scale

```bash
# Применить манифест (создать или обновить)
kubectl apply -f deployment.yaml
kubectl apply -f ./k8s/  # все файлы в директории
kubectl apply -f ./k8s/ -R  # рекурсивно

# Dry-run: показать что изменится, не применяя
kubectl apply -f deployment.yaml --dry-run=client
kubectl apply -f deployment.yaml --dry-run=server  # отправить в API server, не сохранять

# Diff: посмотреть разницу между текущим и манифестом
kubectl diff -f deployment.yaml
```
```diff
-  image: gcr.io/skibookers/api-server:v1.2.3
+  image: gcr.io/skibookers/api-server:v1.2.4
```

```bash
# Удалить ресурс
kubectl delete pod api-server-7d6b8f9c4-xkp2m -n production  # pod пересоздастся
kubectl delete deployment api-server -n production             # удалить deployment

# Удалить по манифесту
kubectl delete -f deployment.yaml

# Удалить все pod в namespace (deployment их пересоздаст)
kubectl delete pods --all -n staging
```

```bash
# Масштабирование
kubectl scale deployment api-server --replicas=5 -n production
```
```
deployment.apps/api-server scaled
```

```bash
# Автомасштабирование (HPA)
kubectl get hpa -n production
```
```
NAME         REFERENCE              TARGETS   MINPODS   MAXPODS   REPLICAS   AGE
api-server   Deployment/api-server  45%/70%   2         10        3          30d
```

```bash
# Редактировать ресурс прямо в кластере (откроет $EDITOR)
kubectl edit deployment api-server -n production
```

---

## Полезные флаги и алиасы

### Сокращения ресурсов

| Полное название | Сокращение |
|---|---|
| `namespaces` | `ns` |
| `pods` | `po` |
| `services` | `svc` |
| `deployments` | `deploy` |
| `configmaps` | `cm` |
| `persistentvolumeclaims` | `pvc` |
| `replicasets` | `rs` |
| `statefulsets` | `sts` |
| `horizontalpodautoscalers` | `hpa` |
| `ingresses` | `ing` |

### Shell алиасы (в ~/.zshrc или ~/.bashrc)

```bash
alias k='kubectl'
alias kgp='kubectl get pods'
alias kgpw='kubectl get pods -w'
alias kgs='kubectl get svc'
alias kgd='kubectl get deployment'
alias kl='kubectl logs'
alias klf='kubectl logs -f'
alias ke='kubectl exec -it'
alias kd='kubectl describe'
alias kdp='kubectl describe pod'

# Быстрое переключение контекста
alias kprod='kubectl config use-context gke_skibookers_europe-west4_skiprod'
alias klocal='kubectl config use-context docker-desktop'
```

### kubectx + kubens: удобное переключение

```bash
# Установить
brew install kubectx

# Показать контексты и переключить
kubectx                       # список контекстов
kubectx gke_skibookers_...    # переключить
kubectx -                     # вернуться к предыдущему

# Переключить namespace
kubens production
kubens -  # вернуться к предыдущему
```

### Вывод в разных форматах

```bash
kubectl get pods -o wide          # дополнительные колонки (IP, node)
kubectl get pods -o yaml          # полный YAML
kubectl get pods -o json          # JSON
kubectl get pods -o name          # только имена: pod/api-server-xxx
kubectl get pods -o jsonpath=...  # конкретное поле
kubectl get pods -o custom-columns=NAME:.metadata.name,STATUS:.status.phase
```

---

## Быстрый troubleshooting workflow

### Pod не стартует

```bash
# 1. Смотрим статус
kubectl get pods -n production
# CrashLoopBackOff, OOMKilled, ImagePullBackOff, Pending, Error

# 2. Смотрим события pod
kubectl describe pod <pod-name> -n production
# Раздел Events внизу — причина обычно там

# 3. Смотрим логи (в том числе предыдущего контейнера)
kubectl logs <pod-name> -n production --previous
kubectl logs <pod-name> -n production --tail=100
```

### Типичные статусы и причины

| Статус | Что значит | Куда смотреть |
|---|---|---|
| `Pending` | Нет ресурсов или нода не найдена | `describe pod` → Events: FailedScheduling |
| `ImagePullBackOff` | Не может скачать образ | `describe pod` → Events: неверный tag или credentials |
| `CrashLoopBackOff` | Pod падает при старте | `logs --previous` → что пишет приложение перед смертью |
| `OOMKilled` | Превысил limits.memory | `describe pod` → Last State: OOMKilled; увеличить limit |
| `Terminating` висит | Не может завершиться | Проверить finalizers: `kubectl patch pod ... -p '{"metadata":{"finalizers":[]}}' --type=merge` |
| `0/1 Ready` | Probe не проходит | `describe pod` → Readiness probe failed |

```bash
# Деплой завис: новые pod не становятся Ready
kubectl rollout status deployment/api-server -n production
# Waiting for... (не завершается)

# Посмотреть состояние replicaset
kubectl get rs -n production

# Посмотреть события deployment
kubectl describe deployment api-server -n production
# Раздел Events покажет почему новые pod не стартуют

# Если нужно быстро откатить
kubectl rollout undo deployment/api-server -n production
```

```bash
# Service не достигает pod
kubectl get endpoints api-server -n production
# Если ENDPOINTS = <none> — selector в Service не совпадает с labels у pod

kubectl get pods --show-labels -n production  # проверить labels
kubectl describe svc api-server -n production  # проверить selector
```

```bash
# Нужно срочно посмотреть переменные окружения
kubectl exec api-server-7d6b8f9c4-xkp2m -n production -- env | sort

# Нет ли OOM за последнее время
kubectl get events -n production --field-selector reason=OOMKilling
```
