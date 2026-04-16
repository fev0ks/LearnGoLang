# TLS Termination, Re-encryption And mTLS

Эта заметка нужна, чтобы не путать три разные модели:
- `TLS termination`
- `re-encryption`
- `mTLS`

Они часто звучат рядом, но решают немного разные задачи.

## Самая короткая интуиция

### TLS termination at edge

```text
client --TLS--> edge --HTTP--> backend
```

Снаружи трафик шифруется, внутри после edge может идти обычный HTTP.

### Re-encryption

```text
client --TLS--> edge --TLS--> backend
```

На edge внешний TLS заканчивается, но дальше начинается новый внутренний TLS hop.

### mTLS

```text
service A --TLS + client cert--> service B
```

Обе стороны предъявляют сертификаты и аутентифицируют друг друга.

## Что такое TLS termination

`TLS termination` значит:
- клиент устанавливает защищенное соединение с edge/proxy/gateway;
- edge расшифровывает трафик;
- дальше может отправить его в backend.

Это полезно, когда:
- не хочется держать внешний TLS на каждом приложении;
- сертификаты, renewal и policy удобнее централизовать;
- нужен единый perimeter для публичного traffic.

Что это решает:
- упрощает certificate management;
- снимает TLS overhead и config complexity с приложений;
- дает единый внешний security policy.

Чего это не гарантирует:
- что внутри сети теперь можно не шифровать вообще;
- что east-west traffic автоматически безопасен;
- что внутренние hop'ы не надо защищать.

## Что такое re-encryption

`Re-encryption` значит:
- внешний TLS заканчивается на edge;
- edge открывает новый TLS к внутреннему сервису.

То есть:

```text
client --TLS--> edge
edge decrypts
edge --new TLS--> backend
```

Когда это делают:
- внутренняя сеть не считается доверенной;
- есть cloud/zero-trust требования;
- есть compliance требования шифровать трафик end-to-end по всем hop'ам;
- platform team не хочет передавать sensitive traffic по plain HTTP внутри кластера/VPC.

Что это решает:
- трафик внутри тоже шифруется;
- меньше риск утечки/inspection на внутренней сети;
- проще соответствовать security/compliance правилам.

Что важно:
- backend в этом случае тоже должен уметь принимать TLS;
- значит ему уже нужен серверный сертификат.

## Нужны ли сертификаты внутренним сервисам

Да, если есть `re-encryption` или `mTLS`, то внутренним сервисам нужны сертификаты.

Почему:
- сервис должен предъявить себя как TLS server;
- вызывающая сторона должна проверить, что подключилась к правильному сервису;
- без сертификата полноценного TLS hop не получится.

То есть:

### При plain HTTP внутри

- сертификат внутреннему сервису не нужен.

### При re-encryption

- внутреннему сервису нужен серверный сертификат.

### При mTLS

- обеим сторонам нужны сертификаты.

## Что такое mTLS

`mTLS` = `mutual TLS`.

В обычном TLS:
- клиент проверяет сервер;
- сервер не обязательно проверяет клиента через сертификат.

В `mTLS`:
- клиент тоже предъявляет сертификат;
- сервер проверяет его;
- получается взаимная аутентификация.

Это полезно для:
- service-to-service auth;
- zero-trust;
- platform-level identity between workloads;
- internal APIs, где хочется не только encryption, но и strong workload identity.

Что это решает:
- сервис знает, кто к нему пришел;
- можно строить auth policy на workload identity;
- уменьшается риск lateral movement внутри сети.

## Когда plain HTTP внутри еще встречается

Да, это до сих пор бывает.

Например:
- маленький internal system в одной доверенной сети;
- локальная dev среда;
- старый datacenter stack;
- простой perimeter security model.

Почему так делают:
- проще;
- меньше operational overhead;
- не надо управлять внутренним PKI.

Почему это может быть плохим решением:
- “внутренняя сеть безопасна” часто ложное допущение;
- lateral movement после компрометации становится проще;
- сложнее соответствовать zero-trust и compliance требованиям.

## Откуда берутся внутренние сертификаты

Частые варианты:
- internal CA;
- cert-manager в Kubernetes;
- cloud/private PKI;
- service mesh;
- SPIRE/SPIFFE-like identity systems.

Главная идея:
- внутренние certs не должны обычно выпускаться вручную руками разработчика;
- это platform/security automation concern.

## Где тут service mesh

Service mesh часто берет на себя:
- автоматическую выдачу сертификатов;
- rotation;
- `mTLS` между сервисами;
- policy enforcement.

То есть mesh не “изобретает TLS заново”, а автоматизирует внутренний TLS/mTLS lifecycle.

## Как думать про выбор модели

### TLS only at edge

Подходит, если:
- система небольшая;
- внутренний risk acceptably low;
- operational simplicity важнее.

### Re-encryption

Подходит, если:
- нужен encrypted traffic внутри;
- но mutual identity еще не обязательна.

### mTLS

Подходит, если:
- нужен и encryption, и service identity;
- zero-trust model;
- много сервисов и высокий security bar.

## Practical Rule

Запомнить полезно так:

- `TLS termination` отвечает на вопрос: где заканчивается внешний HTTPS?
- `re-encryption` отвечает на вопрос: шифруем ли мы внутренний hop заново?
- `mTLS` отвечает на вопрос: аутентифицируют ли сервисы друг друга через сертификаты?

И самое важное:
- если внутри есть новый TLS hop, внутренним сервисам тоже нужны сертификаты;
- если есть `mTLS`, сертификаты нужны обеим сторонам.
