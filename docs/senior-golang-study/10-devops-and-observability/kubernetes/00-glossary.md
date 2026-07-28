# Глоссарий Kubernetes

## Содержание

- [Как пользоваться](#как-пользоваться)
- [Машины и плоскости](#машины-и-плоскости)
- [Системные компоненты](#системные-компоненты)
- [Объекты рабочей нагрузки](#объекты-рабочей-нагрузки)
- [Сеть](#сеть)
- [Конфигурация и доступ](#конфигурация-и-доступ)
- [Хранилище](#хранилище)
- [Размещение и выселение](#размещение-и-выселение)
- [Обновление и доставка](#обновление-и-доставка)
- [Инструменты](#инструменты)
- [Термины, которые легко перепутать](#термины-которые-легко-перепутать)

---

## Как пользоваться

Раздел использует английские имена объектов и полей Kubernetes без перевода: именно они встречаются в YAML, выводе `kubectl` и официальной документации. Этот файл даёт короткую расшифровку каждого термина и ссылку на материал, где он разобран подробно.

Читать подряд не нужно. Достаточно возвращаться сюда, когда в тексте встречается незнакомое слово. Порядок изучения раздела описан в [README](./README.md).

Записи вида `spec.nodeName` означают путь к полю внутри YAML-объекта.

---

## Машины и плоскости

| Термин | Расшифровка | Подробно |
| --- | --- | --- |
| кластер (`cluster`) | плоскость управления и набор узлов, которыми она управляет через единый API и общее состояние | [02 кластер и HA](./02-kubernetes-cluster-and-ha.md) |
| плоскость управления (`control plane`) | группа компонентов, которые хранят желаемое состояние и принимают решения: `kube-apiserver`, `etcd`, `kube-scheduler`, контроллеры | [01 архитектура](./01-kubernetes-architecture.md) |
| рабочий узел (`worker node`) | физическая или виртуальная машина, на которой запускаются Pod приложения | [01 архитектура](./01-kubernetes-architecture.md) |
| `Node` | объект Kubernetes API, представляющий машину кластера. Объект уровня всего кластера, не принадлежит `Namespace` | [01 архитектура](./01-kubernetes-architecture.md) |
| плоскость данных (`data plane`) | часть системы, через которую идёт трафик приложения, в отличие от пути управления | [01 архитектура](./01-kubernetes-architecture.md) |
| зона и регион (`zone`, `region`) | границы отказа облачной инфраструктуры. В Kubernetes отражаются метками узлов `topology.kubernetes.io/zone` и `topology.kubernetes.io/region` | [08 прерывания](./08-node-failure-and-disruptions.md) |
| домен отказа (`failure domain`) | набор ресурсов, которые могут отказать одновременно: узел, стойка, зона, регион | [08 прерывания](./08-node-failure-and-disruptions.md) |

---

## Системные компоненты

| Термин | Расшифровка | Подробно |
| --- | --- | --- |
| `kube-apiserver` | HTTP-сервер Kubernetes API: единая точка входа, проверка прав и допустимости объектов | [01 архитектура](./01-kubernetes-architecture.md) |
| `etcd` | распределённое хранилище «ключ-значение», источник истины для состояния объектов API | [02 кластер и HA](./02-kubernetes-cluster-and-ha.md) |
| `kube-scheduler` | планировщик: выбирает `worker node` для Pod, которому узел ещё не назначен | [01 архитектура](./01-kubernetes-architecture.md) |
| контроллер (`controller`) | повторяющийся цикл, который сравнивает желаемое состояние с наблюдаемым и устраняет расхождение | [01 архитектура](./01-kubernetes-architecture.md) |
| `kube-controller-manager` | процесс, внутри которого работает множество независимых контроллеров | [01 архитектура](./01-kubernetes-architecture.md) |
| `kubelet` | агент на каждом узле: запускает назначенные этому узлу Pod и сообщает их состояние в API | [01 архитектура](./01-kubernetes-architecture.md) |
| среда выполнения контейнеров (`container runtime`) | containerd, CRI-O и аналоги: загружают образы и запускают процессы контейнеров | [01 архитектура](./01-kubernetes-architecture.md) |
| `CRI` | Container Runtime Interface — интерфейс между `kubelet` и средой выполнения контейнеров | [01 архитектура](./01-kubernetes-architecture.md) |
| `CNI` | Container Network Interface — интерфейс сетевых плагинов, которые дают Pod интерфейс и IP | [01 архитектура](./01-kubernetes-architecture.md) |
| `CSI` | Container Storage Interface — интерфейс драйверов хранилищ | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| `kube-proxy` | компонент узла, программирующий правила перенаправления трафика `Service` на адреса Pod. Часть сетевых решений заменяет его собственной плоскостью данных | [01 архитектура](./01-kubernetes-architecture.md) |
| CoreDNS | преобразует DNS-имена `Service` в адреса внутри кластера | [01 архитектура](./01-kubernetes-architecture.md) |
| Metrics Server | предоставляет метрики процессора и памяти для `kubectl top` и HPA | [01 архитектура](./01-kubernetes-architecture.md) |
| оператор (`operator`) | контроллер для пользовательского ресурса, автоматизирующий работу с конкретным приложением | [01 архитектура](./01-kubernetes-architecture.md) |
| выбор лидера (`leader election`) | механизм, при котором несколько экземпляров компонента работают одновременно, но решения принимает один активный | [02 кластер и HA](./02-kubernetes-cluster-and-ha.md) |
| кворум (`quorum`) | большинство участников `etcd`, необходимое для подтверждения записи: `floor(N / 2) + 1` | [02 кластер и HA](./02-kubernetes-cluster-and-ha.md) |

---

## Объекты рабочей нагрузки

| Термин | Расшифровка | Подробно |
| --- | --- | --- |
| контейнер (`container`) | изолированное окружение для процесса. Отдельной единицей размещения в Kubernetes не является | [04 Pod и контейнер](./04-pod-vs-container.md) |
| Pod | объект Kubernetes и атомарная единица размещения: один или несколько контейнеров с общими IP, томами и правилами жизненного цикла | [04 Pod и контейнер](./04-pod-vs-container.md) |
| `Deployment` | описывает число взаимозаменяемых реплик, шаблон Pod и стратегию обновления | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| `ReplicaSet` | поддерживает заданное число Pod одной ревизии шаблона. Обычно создаётся `Deployment`, а не вручную | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| `StatefulSet` | управляет Pod со стабильными номерами, сетевыми именами и отдельными постоянными томами | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| `DaemonSet` | запускает по одному Pod на каждом или на выбранных узлах, например агент сбора логов | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| `Job` и `CronJob` | выполняют задачу до успешного завершения; `CronJob` создаёт `Job` по расписанию | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| `HPA` | HorizontalPodAutoscaler: рассчитывает и изменяет число реплик цели по метрикам | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| init container | контейнер, который выполняется до основных и должен успешно завершиться | [04 Pod и контейнер](./04-pod-vs-container.md) |
| sidecar | вспомогательный контейнер рядом с приложением в том же Pod. Native sidecar описывается в `initContainers` с `restartPolicy: Always` | [04 Pod и контейнер](./04-pod-vs-container.md) |
| ephemeral container | отладочный контейнер, добавляемый в уже существующий Pod | [04 Pod и контейнер](./04-pod-vs-container.md) |
| `ownerReferences` | поле, фиксирующее владение: кто отвечает за жизненный цикл объекта и удаляется вместе с ним | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| метка (`label`) и `selector` | пара ключ-значение на объекте и условие, выбирающее объекты с подходящими метками | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| `finalizer` | маркер обязательной очистки: задерживает окончательное удаление объекта, пока контроллер не завершит работу | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| probe | проверка состояния контейнера: `startupProbe`, `readinessProbe`, `livenessProbe` | [07 probes и graceful shutdown](./07-probes-and-graceful-shutdown.md) |
| `QoS class` | класс качества обслуживания (`Guaranteed`, `Burstable`, `BestEffort`), который Kubernetes выводит из requests и limits Pod. Влияет на очередь вытеснения при нехватке ресурсов узла | [04 Pod и контейнер](./04-pod-vs-container.md) |
| `cgroup` | механизм ядра Linux для учёта и ограничения ресурсов процесса. Через него применяются CPU и memory limits контейнера | [namespaces и cgroups](../linux/05-namespaces-and-cgroups.md) |
| пространство имён (`namespace` ядра Linux) | механизм изоляции ядра: сеть, процессы, точки монтирования. Не путать с объектом `Namespace` в Kubernetes | [namespaces и cgroups](../linux/05-namespaces-and-cgroups.md) |
| `OOMKilled` | завершение контейнера ядром при превышении memory limit его cgroup | [07 probes и graceful shutdown](./07-probes-and-graceful-shutdown.md) |
| `CrashLoopBackOff` | состояние ожидания контейнера между повторными запусками после нескольких падений. Не является фазой Pod | [04 Pod и контейнер](./04-pod-vs-container.md) |

---

## Сеть

| Термин | Расшифровка | Подробно |
| --- | --- | --- |
| `Service` | объект со стабильными DNS-именем и виртуальным адресом, выбирающий Pod по меткам | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| `ClusterIP` | тип `Service` по умолчанию: виртуальный адрес, доступный только внутри кластера | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| `NodePort` | тип `Service`, открывающий порт на каждом узле. Чаще служит строительным блоком для других решений | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| `LoadBalancer` | тип `Service`, который интегрируется с внешним балансировщиком облачного провайдера | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| headless `Service` | `Service` с `clusterIP: None`. Не выдаёт один виртуальный адрес, а позволяет получить через DNS адреса отдельных Pod | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| `EndpointSlice` | производный объект со списком сетевых адресов Pod и их состоянием готовности для `Service` | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| `Ingress` | объект с правилами маршрутизации внешнего HTTP-трафика на `Service` по хосту и пути. Сам по себе ничего не делает: правила применяет установленный в кластере Ingress controller | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| Ingress controller | отдельный компонент кластера (например, ingress-nginx), который читает объекты `Ingress` и настраивает реальный прокси или балансировщик | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| Gateway API | более новый набор объектов маршрутизации, развиваемый как преемник `Ingress` | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| `NetworkPolicy` | объект с правилами, какому трафику разрешено приходить к выбранным Pod и уходить от них. Работает только если установленный CNI-плагин его поддерживает; без такого плагина объект принимается, но ничего не ограничивает | [10 сеть](./12-networking-service-dns-and-ingress.md) |
| service mesh | инфраструктурный слой (Istio, Linkerd и аналоги), добавляющий mTLS, маршрутизацию и телеметрию между сервисами | [10 сеть](./12-networking-service-dns-and-ingress.md) |

---

## Конфигурация и доступ

| Термин | Расшифровка | Подробно |
| --- | --- | --- |
| `Namespace` | логическое разделение объектов внутри одного кластера. Задаёт область имён, квот и прав, но сам по себе не создаёт сетевую изоляцию и не является отдельным кластером | [02 кластер и HA](./02-kubernetes-cluster-and-ha.md) |
| объект уровня namespace | объект, существующий внутри `Namespace`: Pod, `Service`, `ConfigMap`, PVC. `Service` выбирает Pod только в своём `Namespace` | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| объект уровня кластера | объект без `Namespace`: `Node`, PV, `StorageClass` | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| `ConfigMap` | объект с неконфиденциальной конфигурацией, отделённой от образа | [03 основные объекты](./03-core-objects-and-deployment-flow.md) |
| `Secret` | объект для чувствительных значений с отдельным контролем доступа. Значения в поле `data` закодированы в base64, а не зашифрованы | [10 конфигурация и секреты](./10-config-and-secret-delivery.md) |
| `ServiceAccount` | идентичность, от имени которой Pod обращается к Kubernetes API. Каждый Pod работает под каким-либо `ServiceAccount`; если он не указан явно, используется `default` из того же `Namespace` | [10 конфигурация и секреты](./10-config-and-secret-delivery.md) |
| `RBAC` | Role-Based Access Control — модель прав Kubernetes. Роль (`Role`, `ClusterRole`) перечисляет разрешённые действия над типами объектов, а привязка (`RoleBinding`, `ClusterRoleBinding`) выдаёт роль пользователю или `ServiceAccount` | [управление секретами](../../11-security/secrets-management/04-kubernetes-secrets-and-external-managers.md) |
| workload identity | механизм, позволяющий Pod получить короткоживущие учётные данные облака на основании его `ServiceAccount`, не храня постоянный ключ в `Secret` | [10 конфигурация и секреты](./10-config-and-secret-delivery.md) |
| encryption at rest | шифрование данных `Secret` в `etcd`. Настраивается отдельно и по умолчанию не включено | [управление секретами](../../11-security/secrets-management/04-kubernetes-secrets-and-external-managers.md) |
| `admission` | этап обработки запроса в API-сервере, на котором настроенные политики проверяют или изменяют объект до сохранения | [01 архитектура](./01-kubernetes-architecture.md) |
| `CustomResource` и CRD | пользовательский тип объекта и его описание, расширяющие Kubernetes API | [01 архитектура](./01-kubernetes-architecture.md) |

---

## Хранилище

| Термин | Расшифровка | Подробно |
| --- | --- | --- |
| `volume` | источник данных, объявленный в `PodSpec` и подключаемый контейнерам. Не отдельный объект API | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| `emptyDir` | временный том, живущий столько же, сколько Pod | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| PVC | `PersistentVolumeClaim` — заявка приложения на постоянный том с нужными размером, режимом доступа и классом | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| PV | `PersistentVolume` — объект уровня кластера, представляющий конкретный доступный том | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| `StorageClass` | описание класса хранилища: какой CSI-драйвер и с какими параметрами создаёт том | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| RWO, ROX, RWX, RWOP | режимы доступа тома. `ReadWriteOnce` ограничивает запись одним **узлом**, а не одним Pod; для одного Pod существует `ReadWriteOncePod` | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| `reclaimPolicy` | что делать с PV и реальным диском после удаления PVC: `Delete` или `Retain` | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| `volumeBindingMode` | когда создавать или выбирать PV: сразу (`Immediate`) или после появления Pod (`WaitForFirstConsumer`) | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| `VolumeSnapshot` | моментальный снимок тома через CSI. Не является полноценной резервной копией сам по себе | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |

---

## Размещение и выселение

| Термин | Расшифровка | Подробно |
| --- | --- | --- |
| `requests` и `limits` | запрошенные и предельные ресурсы контейнера. `requests` участвуют в выборе узла, `limits` ограничивают потребление | [04 Pod и контейнер](./04-pod-vs-container.md) |
| `Allocatable` | ресурсы узла, доступные Pod после вычета зарезервированного для системы | [04 Pod и контейнер](./04-pod-vs-container.md) |
| `Pending` | Pod принят API, но ещё не запущен. Частая причина — планировщик не нашёл подходящий узел | [04 Pod и контейнер](./04-pod-vs-container.md) |
| `taint` | «отметка отторжения» на узле: запрещает размещать на нём Pod, у которых нет соответствующего разрешения. Используется, чтобы зарезервировать узлы или увести нагрузку с проблемной машины | [08 прерывания](./08-node-failure-and-disruptions.md) |
| `toleration` | встречное разрешение в Pod: позволяет разместить его на узле с определённым `taint`. Только разрешает, но не требует такого размещения | [08 прерывания](./08-node-failure-and-disruptions.md) |
| `nodeSelector` | простое требование к меткам узла, на котором может работать Pod | [09 постоянное хранилище](./11-persistent-storage-pv-pvc-and-storageclass.md) |
| `affinity` | более выразительные правила размещения. `nodeAffinity` описывает требования к узлу, `podAffinity` притягивает Pod к другим Pod, `podAntiAffinity` разводит их | [08 прерывания](./08-node-failure-and-disruptions.md) |
| topology spread constraints | правила равномерного распределения Pod по узлам, зонам и другим доменам отказа | [08 прерывания](./08-node-failure-and-disruptions.md) |
| выселение (`eviction`) | принудительное удаление Pod с узла: при обслуживании, нехватке ресурсов или отказе узла | [08 прерывания](./08-node-failure-and-disruptions.md) |
| `drain` | вывод узла из эксплуатации командой `kubectl drain`: узел помечается неназначаемым, а его Pod выселяются через Eviction API с учётом `PDB` | [08 прерывания](./08-node-failure-and-disruptions.md) |
| `PDB` | PodDisruptionBudget: ограничивает число одновременных **добровольных** выселений. От аппаратного отказа узла не защищает | [08 прерывания](./08-node-failure-and-disruptions.md) |
| вытеснение (`preemption`) | удаление менее приоритетных Pod, чтобы освободить место более приоритетному на одном узле | [04 Pod и контейнер](./04-pod-vs-container.md) |
| autoscaler узлов | компонент, добавляющий и удаляющий узлы под текущие запросы Pod. Ресурсы разных машин не складывает | [04 Pod и контейнер](./04-pod-vs-container.md) |

---

## Обновление и доставка

| Термин | Расшифровка | Подробно |
| --- | --- | --- |
| согласование (`reconciliation`) | непрерывное сравнение желаемого и наблюдаемого состояния с постепенным устранением расхождения | [01 архитектура](./01-kubernetes-architecture.md) |
| `rollout` | процесс постепенной замены Pod после изменения шаблона | [09 стратегии обновления](./09-update-strategies.md) |
| `RollingUpdate` | стратегия постепенной замены старых Pod новыми в границах `maxSurge` и `maxUnavailable` | [09 стратегии обновления](./09-update-strategies.md) |
| `Recreate` | стратегия `Deployment`: сначала остановить все старые Pod, затем создать новые. Обычно даёт окно недоступности | [09 стратегии обновления](./09-update-strategies.md) |
| `OnDelete` | стратегия `StatefulSet` и `DaemonSet`: новый шаблон применяется только после ручного удаления Pod | [09 стратегии обновления](./09-update-strategies.md) |
| canary и blue-green | стратегии доставки трафика между версиями. Не являются допустимыми значениями `strategy.type` | [09 стратегии обновления](./09-update-strategies.md) |
| graceful shutdown | корректное завершение: прекратить приём новых запросов, доработать текущие и выйти до истечения отведённого времени | [07 probes и graceful shutdown](./07-probes-and-graceful-shutdown.md) |
| `SIGTERM` | сигнал завершения, который `kubelet` отправляет главному процессу контейнера перед принудительной остановкой | [07 probes и graceful shutdown](./07-probes-and-graceful-shutdown.md) |
| `terminationGracePeriodSeconds` | время от начала завершения Pod до принудительной остановки процессов. `preStop` выполняется внутри этого времени, а не сверх него | [07 probes и graceful shutdown](./07-probes-and-graceful-shutdown.md) |
| `preStop` | обработчик, который `kubelet` выполняет перед отправкой `SIGTERM` | [07 probes и graceful shutdown](./07-probes-and-graceful-shutdown.md) |
| `GitOps` | подход к доставке, при котором желаемое состояние кластера хранится в Git, а контроллер в кластере непрерывно приводит кластер к состоянию из репозитория. Ручное изменение через `kubectl` при этом будет отменено следующим согласованием | [05 kubectl](./05-kubectl-commands.md) |
| `drift` | расхождение между состоянием кластера и описанием в source of truth, обычно после ручных правок | [05 kubectl](./05-kubectl-commands.md) |
| immutable tag и digest | неизменяемая ссылка на образ. Digest даёт более сильную гарантию, чем изменяемый тег вида `latest` | [06 Helm](./06-helm.md) |

---

## Инструменты

| Термин | Расшифровка | Подробно |
| --- | --- | --- |
| `kubectl` | клиент Kubernetes API | [05 kubectl](./05-kubectl-commands.md) |
| `kubeconfig` | файл с адресами кластеров, учётными данными и контекстами | [05 kubectl](./05-kubectl-commands.md) |
| контекст (`context`) | сохранённая связка «кластер + пользователь + namespace» в `kubeconfig` | [05 kubectl](./05-kubectl-commands.md) |
| `kubeadm` | инструмент установки кластера на собственных машинах | [02 кластер и HA](./02-kubernetes-cluster-and-ha.md) |
| kind | запуск учебного кластера Kubernetes в контейнерах Docker на одной машине | [00 локальный кластер](./00-local-cluster.md) |
| Helm | пакетный менеджер: шаблонизирует манифесты и ведёт историю установок | [06 Helm](./06-helm.md) |
| Kustomize | наложение изменений окружения на общую основу манифестов без шаблонного языка | [11 разбор манифеста](./13-practical-manifest-review.md) |
| managed Kubernetes | вариант поставки, где плоскость управления обслуживается облачным провайдером | [02 кластер и HA](./02-kubernetes-cluster-and-ha.md) |

---

## Термины, которые легко перепутать

| Пара | В чём различие |
| --- | --- |
| контейнер и Pod | контейнер — изолированное окружение процесса, Pod — единица размещения Kubernetes, содержащая один или несколько контейнеров |
| `Service` и процесс-сервис | `Service` — объект API с адресом и правилами выбора Pod, а не запущенная программа и не системная служба |
| `Namespace` и кластер | `Namespace` разделяет объекты внутри одного API, отдельный кластер имеет собственные API, состояние и границу отказа |
| секрет и `Secret` | секрет — само чувствительное значение, `Secret` — объект Kubernetes, внутри которого оно может храниться |
| PVC и диск | PVC — заявка на хранилище, реальные байты находятся на носителе, а не «внутри» PVC или PV |
| `RWO` и «один Pod» | `ReadWriteOnce` ограничивает подключение одним узлом; несколько Pod на этом узле могут использовать том |
| `readiness` и `liveness` | readiness управляет участием в трафике и может временно проваливаться, liveness приводит к перезапуску контейнера |
| `CrashLoopBackOff` и Pod phase | это причина ожидания конкретного контейнера, а не фаза Pod |
| `taint` и `toleration` | `taint` на узле отталкивает Pod, `toleration` в Pod разрешает такое размещение |
| `PDB` и отказ узла | `PDB` ограничивает добровольные выселения и не защищает от падения машины |
| `strategy.type` и canary | `strategy.type` управляет заменой Pod, canary и blue-green управляют распределением трафика |
| снимок и резервная копия | снимок может лежать у того же провайдера и под теми же правами удаления, что и исходный диск |

---

## Официальные источники

- [Kubernetes Glossary](https://kubernetes.io/docs/reference/glossary/)
- [Kubernetes Concepts](https://kubernetes.io/docs/concepts/)
- [Kubernetes API reference](https://kubernetes.io/docs/reference/kubernetes-api/)
