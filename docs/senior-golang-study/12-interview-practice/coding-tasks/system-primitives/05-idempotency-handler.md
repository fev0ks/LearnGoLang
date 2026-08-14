# Задача 5: Idempotency Key Handler

## Содержание

- [Контракт задачи](#контракт-задачи)
- [Почему Get-Handle-Save некорректен](#почему-get-handle-save-некорректен)
- [Модель состояния](#модель-состояния)
- [PostgreSQL: claim и business write](#postgresql-claim-и-business-write)
- [HTTP-граница](#http-граница)
- [Redis-вариант](#redis-вариант)
- [Что сохранять и как долго](#что-сохранять-и-как-долго)
- [Тестирование и метрики](#тестирование-и-метрики)
- [Типичные ошибки](#типичные-ошибки)
- [Interview-ready answer](#interview-ready-answer)

Idempotency key связывает повторы одной логической mutation. Клиент отправляет
один и тот же ключ при retry, а сервер либо выполняет действие один раз, либо
возвращает сохранённый outcome. Это application protocol, а не свойство одного
HTTP middleware.

---

## Контракт задачи

Нужно заранее определить:

1. Для каких endpoints ключ обязателен?
2. Что входит в fingerprint: tenant, operation, path, параметры, body?
3. Что получает concurrent повтор: wait, `409`, `425` или job status?
4. Какие outcomes replay-ятся: success, validation error, `5xx`?
5. Как idempotency record атомарно связан с business side effect?
6. Каков retention и что произойдёт после expiry?

HTTP `PUT`, `DELETE` и safe methods идемпотентны по семантике RFC 9110, но
конкретная реализация всё равно может иметь дополнительные side effects. `POST`
можно сделать идемпотентным договорённостью и ключом. Метод сам по себе не решает
атомарность business operation.

Ключ namespace-ят как минимум по tenant и operation. Один raw key для всех
клиентов позволяет случайный или злонамеренный conflict между ними.

---

## Почему Get-Handle-Save некорректен

Наивный middleware делает:

```text
GET key -> handler creates order -> SAVE response
```

У него две независимые race/crash window:

```text
A: GET -> not found
B: GET -> not found
A: creates order
B: creates another order
```

и:

```text
create order -> process crashes -> idempotency record was not saved
client retries -> creates another order
```

In-memory `map[key]chan struct{}` закрывает первую гонку только внутри одного
process и может навсегда блокировать waiter без select по `ctx.Done()`. Она не
закрывает crash window между side effect и `Save`.

Поэтому generic response-caching middleware допустим только если:

- side effect уже идемпотентен по тому же ключу; или
- handler и idempotency storage используют общий атомарный commit; или
- handler лишь создаёт durable job/outbox record в этой transaction.

---

## Модель состояния

Минимальная запись имеет состояния `processing` и `completed`:

```sql
CREATE TABLE idempotency_keys (
    tenant_id     TEXT        NOT NULL,
    operation     TEXT        NOT NULL,
    key           TEXT        NOT NULL,
    request_hash  BYTEA       NOT NULL,
    state         TEXT        NOT NULL
                  CHECK (state IN ('processing', 'completed')),
    status_code   INTEGER,
    response_body BYTEA,
    headers       JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at  TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, operation, key),
    CHECK (
        (state = 'processing' AND status_code IS NULL) OR
        (state = 'completed' AND status_code IS NOT NULL)
    )
);

CREATE INDEX idempotency_keys_created_at_idx
    ON idempotency_keys (created_at);
```

`processing` — это claim на выполнение, а не готовый replay. Нужно определить,
может ли он протухнуть, кто и как делает recovery после crash и безопасно ли
повторить сам side effect. Простое удаление «старого processing» способно
запустить действие второй раз.

Fingerprint считают по каноническому набору business inputs. Простая
конкатенация `method + path + body` допускает неоднозначные границы и считает
эквивалентный JSON с другим whitespace новым запросом. Надёжнее сериализовать
versioned struct в canonical representation или хешировать length-prefixed
поля.

---

## PostgreSQL: claim и business write

Если idempotency record и business data находятся в одной PostgreSQL, их можно
связать одной transaction. `INSERT ... ON CONFLICT DO NOTHING` атомарно выбирает
владельца. При конфликте PostgreSQL дождётся исхода конкурирующей уникальной
записи; затем `SELECT ... FOR UPDATE` читает committed outcome.

```go
type CreateOrderResult struct {
    Status int
    Body   []byte
}

func (s *Service) CreateOrder(
    ctx context.Context,
    tenantID string,
    key string,
    requestHash []byte,
    req CreateOrderRequest,
) (CreateOrderResult, error) {
    tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return CreateOrderResult{}, err
    }
    defer tx.Rollback(ctx)

    tag, err := tx.Exec(ctx, `
        INSERT INTO idempotency_keys (
            tenant_id, operation, key, request_hash, state
        )
        VALUES ($1, 'create-order', $2, $3, 'processing')
        ON CONFLICT DO NOTHING
    `, tenantID, key, requestHash)
    if err != nil {
        return CreateOrderResult{}, err
    }

    if tag.RowsAffected() == 0 {
        var storedHash []byte
        var state string
        var result CreateOrderResult

        err = tx.QueryRow(ctx, `
            SELECT request_hash,
                   state,
                   COALESCE(status_code, 0),
                   response_body
            FROM idempotency_keys
            WHERE tenant_id = $1
              AND operation = 'create-order'
              AND key = $2
            FOR UPDATE
        `, tenantID, key).Scan(
            &storedHash,
            &state,
            &result.Status,
            &result.Body,
        )
        if err != nil {
            return CreateOrderResult{}, err
        }
        if !bytes.Equal(storedHash, requestHash) {
            return CreateOrderResult{}, ErrKeyConflict
        }
        if state != "completed" {
            return CreateOrderResult{}, ErrInProgress
        }
        if err := tx.Commit(ctx); err != nil {
            return CreateOrderResult{}, err
        }
        return result, nil
    }

    order, err := insertOrder(ctx, tx, tenantID, req)
    if err != nil {
        return CreateOrderResult{}, err
    }

    result := encodeCreatedOrder(order)
    tag, err = tx.Exec(ctx, `
        UPDATE idempotency_keys
        SET state = 'completed',
            status_code = $1,
            response_body = $2,
            completed_at = clock_timestamp()
        WHERE tenant_id = $3
          AND operation = 'create-order'
          AND key = $4
          AND state = 'processing'
    `, result.Status, result.Body, tenantID, key)
    if err != nil {
        return CreateOrderResult{}, err
    }
    if tag.RowsAffected() != 1 {
        return CreateOrderResult{}, fmt.Errorf("complete idempotency record")
    }

    if err := tx.Commit(ctx); err != nil {
        return CreateOrderResult{}, err
    }
    return result, nil
}
```

Внутри одной transaction конкурент обычно не увидит длительное состояние
`processing`: он ждёт завершения unique conflict и затем читает `completed`.
Поле всё равно полезно для явного инварианта и для двухфазных workflows.

Нельзя держать DB transaction открытой вокруг минутного внешнего HTTP-вызова.
Для такого flow durable transaction создаёт job/outbox record, worker вызывает
dependency с тем же idempotency key, затем отдельно фиксирует результат. Граница
exactly-once заканчивается там, где нет общего commit или idempotent downstream.

---

## HTTP-граница

Handler отвечает за transport validation, а service — за атомарность:

```go
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    key := r.Header.Get("Idempotency-Key")
    if key == "" || len(key) > 255 {
        http.Error(w, "invalid Idempotency-Key", http.StatusBadRequest)
        return
    }

    body := http.MaxBytesReader(w, r.Body, 1<<20)
    defer body.Close()

    var req CreateOrderRequest
    dec := json.NewDecoder(body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(&req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }

    fingerprint, err := hashCreateOrder(req)
    if err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }

    result, err := h.service.CreateOrder(
        r.Context(), tenantFrom(r), key, fingerprint, req,
    )
    switch {
    case errors.Is(err, ErrKeyConflict):
        http.Error(w, "idempotency key conflict", http.StatusConflict)
        return
    case errors.Is(err, ErrInProgress):
        http.Error(w, "request is processing", http.StatusConflict)
        return
    case err != nil:
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(result.Status)
    _, _ = w.Write(result.Body)
}
```

`255` здесь — выбранный API contract, совпадающий с текущим ограничением
Stripe, а не требование стандарта. Лимит body тоже является допущением и должен
соответствовать endpoint.

Response-capturing middleware должен корректно обрабатывать implicit `200`, не
вызывать `WriteHeader` дважды, ограничивать buffer и сохранять все значения
headers. Wrapper также меняет optional interfaces `http.ResponseWriter`
(`Flusher`, `Hijacker`, `Pusher`); streaming и hijacked responses обычно нужно
явно исключить из idempotency cache.

---

## Redis-вариант

`GET`, затем `SET NX` после handler не обеспечивает claim. Нужен атомарный
переход через Lua или transaction:

```text
missing    -> create processing with request hash and TTL -> owner
processing -> compare hash -> wait/reject
completed  -> compare hash -> replay
```

Завершение должно atomically проверять owner token и менять `processing` на
`completed`, иначе stale worker может перезаписать результат нового владельца.
Для Redis Cluster все ключи script передаются через `KEYS` и используют общий
hash tag.

Trade-offs:

- Redis удобен для короткого replay cache и высокой нагрузки;
- TTL встроен, но eviction и persistence policy влияют на гарантию;
- Redis нельзя считать надёжнее business DB только из-за `SET NX`;
- если side effect записывается в PostgreSQL, отдельный Redis оставляет между
  системами crash window.

Для финансовой mutation выбор делают по общей атомарности, а не по ярлыку
«PostgreSQL для critical, Redis для обычного».

---

## Что сохранять и как долго

Минимум сохраняют request fingerprint, state и replayable outcome. Headers
фильтруют: hop-by-hop и динамические tracing headers не стоит бездумно
воспроизводить. Response body ограничивают по размеру; для большого результата
можно хранить business resource ID и заново собрать представление.

Политика outcomes зависит от API:

- deterministic validation можно выполнять до claim и не сохранять;
- outcome начавшейся business operation часто сохраняют, даже если он error;
- не всякий `5xx` transient: действие могло выполниться до формирования ответа;
- не всякий `4xx` детерминирован навсегда;
- Stripe сохраняет status и body первой начавшейся операции, включая failure,
  но это конкретный protocol, а не универсальное правило.

Retention должен покрывать максимальное окно retry клиента. Stripe указывает,
что ключи можно удалять после того, как им исполнилось не менее 24 часов. Это не
означает, что 24 часа — стандарт для любого API. После удаления тот же ключ
считается новой операцией, поэтому contract нужно документировать.

Хранить raw request с PII обычно не требуется: versioned hash и минимальный
replay outcome уменьшают privacy и storage risk.

---

## Тестирование и метрики

Обязательные сценарии:

1. первый запрос создаёт одну business row и completed record;
2. последовательный повтор возвращает тот же outcome;
3. тот же key с другим fingerprint получает conflict;
4. два concurrent запроса создают ровно одну business row;
5. rollback business write не оставляет completed record;
6. commit неизвестного исхода безопасно разрешается повторным чтением;
7. cancel waiter не отменяет owner и не зависает;
8. cleanup не удаляет живой `processing` без recovery protocol;
9. тесты проходят под `go test -race` и на реальной PostgreSQL.

Полезные метрики: claims, replay, conflicts, in-progress, storage latency,
processing age, response size и cleanup duration. В labels нельзя помещать сам
idempotency key или tenant с высокой cardinality.

---

## Типичные ошибки

- Делать `Get -> side effect -> Save` без атомарного claim.
- Защищать ключ только локальным mutex при нескольких pods.
- Игнорировать ошибку `Save` после уже отправленного клиенту ответа.
- Считать `ON CONFLICT DO NOTHING` успехом, не проверяя `RowsAffected` и hash.
- Продолжать внешний side effect после потери lease на `processing`.
- Всегда не сохранять `5xx`, хотя операция могла завершиться.
- Кэшировать неограниченный response body или streaming response.
- Использовать raw key без tenant/operation namespace.
- Удалять старый `processing` и повторять недоказанно идемпотентную операцию.
- Обещать exactly-once через middleware при внешнем side effect.

---

## Interview-ready answer

1. **Как устроен idempotency protocol?**
   - **Client key —** один ключ сопровождает все повторы логической mutation.
   - **Fingerprint —** тот же ключ с другими business inputs отклоняется.
   - **Replay —** completed outcome возвращается без повторного side effect.

2. **Где главная гонка?**
   - **Concurrent claim —** `Get`, затем `Save` пропускает двух владельцев.
   - **Atomicity —** claim, business write и completed outcome по возможности
     фиксируются одной transaction.
   - **Crash window —** для внешнего API нужен его idempotency key или durable
     workflow; локальный record не даёт exactly-once.

3. **Что важно в production contract?**
   - **Outcomes —** явно определить, какие success/errors replay-ятся.
   - **Retention —** TTL покрывает retry window, после expiry ключ становится
     новым.
   - **Bounds —** ограничены key, body, response и время `processing`.

---

## Связанные материалы

- [Idempotency](../../../05-system-design/reliability-patterns/06-idempotency.md)
- [Outbox pattern](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md)
- [Distributed lock](./04-distributed-lock.md)
- [RFC 9110: Idempotent Methods](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2)
- [Stripe: Idempotent requests](https://docs.stripe.com/api/idempotent_requests)
