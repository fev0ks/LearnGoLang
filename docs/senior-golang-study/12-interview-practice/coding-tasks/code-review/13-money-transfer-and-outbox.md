# Задача 13: денежный перевод и transactional outbox

## Содержание

- [Формулировка](#формулировка)
- [Исходный код](#исходный-код)
- [Порядок review](#порядок-review)
- [Вопросы к контракту](#вопросы-к-контракту)
- [Основные проблемы](#основные-проблемы)
- [Целевая модель](#целевая-модель)
- [Исправленное решение](#исправленное-решение)
- [Транзакция, блокировки и retry](#транзакция-блокировки-и-retry)
- [События и transactional outbox](#события-и-transactional-outbox)
- [Метрики и аналитика](#метрики-и-аналитика)
- [Пример теста](#пример-теста)
- [Что проверить тестами](#что-проверить-тестами)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связанные-материалы)

Задача проверяет денежные инварианты, конкурентные транзакции, идемпотентность,
границу между observability и точной аналитикой, а также согласованную публикацию
событий. Главный навык — начать с риска частичного или повторного перевода, а не
с косметики и перестановки интерфейсов.

---

## Формулировка

Нужно провести review сервиса денежных переводов:

1. Найти проблемы и доказать их последствия.
2. Предложить production-вариант операции `Transfer`.
3. Обеспечить уведомления об изменении обоих балансов.
4. Определить, как считать число завершённых переводов.
5. Назвать тесты для конкурентных вызовов и повторов.

API-handler можно не реализовывать. Считаем, что используется PostgreSQL, а
`transferID` приходит с внешней границы как idempotency key одной логической
операции.

Ориентир по времени:

- 5 минут — компиляция, контракт и денежные типы;
- 10 минут — транзакция и конкурентные сценарии;
- 10 минут — идемпотентность, события и метрика;
- 15 минут — исправление главного пути;
- 10 минут — тесты и оставшиеся trade-offs.

---

## Исходный код

Код намеренно оставлен без комментариев-подсказок.

```go
package transfer

import (
    "database/sql"
    "errors"
    "fmt"
    "time"
)

type EventPublisher interface {
    Publish(string, any) error
}

type MetricCollector interface {
    Send(string, int) error
}

type Account struct {
    ID      string
    Balance float32
}

type AccountRepository struct {
    db                sql.DB
    eventPublisher    EventPublisher
    metricCollector   MetricCollector
    totalTransactions int
}

func NewAccountRepository(
    db sql.DB,
    eventPublisher EventPublisher,
    metricCollector MetricCollector,
) AccountRepository {
    repository := AccountRepository{
        db:              db,
        eventPublisher:  eventPublisher,
        metricCollector: metricCollector,
    }

    go func() {
        for range time.Tick(time.Minute) {
            _ = repository.metricCollector.Send(
                "TotalTransactions",
                repository.totalTransactions,
            )
        }
    }()

    return repository
}

func (repository *AccountRepository) Transfer(
    fromID string,
    toID string,
    amount float32,
) error {
    from, err := repository.find(fromID)
    if err != nil {
        return err
    }

    to, err := repository.find(toID)
    if err != nil {
        return err
    }

    from.Balance -= amount
    to.Balance += amount

    if err := repository.save(from); err != nil {
        return err
    }
    if err := repository.save(to); err != nil {
        return err
    }

    repository.totalTransactions++
    return nil
}

func (repository *AccountRepository) find(id string) (Account, error) {
    var account Account
    err := repository.db.QueryRow(
        "SELECT * FROM accounts WHERE id = " + id,
    ).Scan(&account)
    if errors.Is(err, sql.ErrNoRows) {
        return Account{}, errors.New("account not found")
    }
    if err != nil {
        return Account{}, err
    }
    return account, nil
}

func (repository *AccountRepository) save(account Account) error {
    query := fmt.Sprintf(
        "UPDATE accounts SET balance = %s WHERE id = %s",
        account.Balance,
        account.ID,
    )
    if _, err := repository.db.Exec(query); err != nil {
        return err
    }

    if err := repository.eventPublisher.Publish(
        "BalanceChanged",
        account,
    ); err != nil {
        return err
    }
    return nil
}
```

---

## Порядок review

Сначала сформулируй инвариант:

> После успешного `Transfer` сумма денег на двух счетах не меняется, debit и
> credit фиксируются атомарно, один `transferID` применяется не больше одного
> раза, а события создаются только для committed-перевода.

Затем проверь четыре сценария:

1. Списание прошло, а зачисление упало.
2. Два вызова одновременно прочитали один исходный баланс.
3. Ответ потерялся после commit, и caller повторил запрос.
4. Событие было опубликовано, а метод затем вернул ошибку.

Если хотя бы один сценарий нарушает инвариант, обсуждение структуры packages и
названий abstractions откладывается до исправления денежного пути.

---

## Вопросы к контракту

До исправления кода полезно уточнить:

- метрика считает attempts, committed transfers или оба значения;
- аналитике нужна точность после рестартов или достаточно operational counter;
- повтор с тем же `transferID` должен вернуть сохранённый результат или только
  успешный статус;
- разрешены ли переводы между разными валютами;
- является ли перевод самому себе ошибкой или явно описанным no-op;
- нужны два `BalanceChanged` или одно `TransferCompleted`;
- допустимы ли дубли уведомлений и умеет ли consumer их дедуплицировать.

Ни уровень изоляции, ни outbox не отвечают на эти продуктовые вопросы
автоматически.

---

## Основные проблемы

| Приоритет | Проблема | Последствие |
| --- | --- | --- |
| Блокер | `Scan(&account)` пытается прочитать набор колонок в один struct | Запрос не заполняет модель и завершается ошибкой |
| Критично | Деньги представлены через `float32` | Округление разрушает точные сравнения и накопительные вычисления |
| Критично | Debit и credit выполняются без транзакции | Возможен частичный перевод |
| Критично | Используется `read → calculate → write` без locks | Конкурентные вызовы теряют обновления |
| Критично | Не проверяются `amount > 0` и `fromID != toID` | Отрицательная сумма меняет направление, self-transfer создаёт деньги |
| Критично | Нет `transferID` и уникального ограничения | Retry может провести тот же перевод повторно |
| Критично | Event публикуется между несогласованными SQL-операциями | Уведомление может описывать состояние, которое затем не будет committed |
| Критично | Ошибка publisher возвращается после изменения баланса | Caller видит failure и безопасно повторить вызов уже не может |
| Важно | Счётчик читается и меняется конкурентно | Возникает data race |
| Важно | Constructor возвращает repository по значению | Goroutine и caller могут работать с разными копиями счётчика |
| Важно | `sql.DB` хранится и передаётся по значению | Копируется объект connection pool, который не должен копироваться после использования |
| Важно | SQL строится конкатенацией и `Sprintf` | SQL injection, ошибки quoting и неверное форматирование `float32` через `%s` |
| Важно | Нет `context.Context` | Нельзя отменить ожидание pool, lock и запросы |
| Важно | Нет порядка захвата locks | Встречные переводы могут попасть в deadlock |
| Важно | Не проверяется число изменённых строк | Успех возможен при фактическом отсутствии update |
| Важно | У фоновой goroutine нет shutdown | Она продолжает жить после прекращения использования repository |

`sql.DB` уже является concurrency-safe connection pool. Его лимиты
`SetMaxOpenConns`, `SetMaxIdleConns` и `SetConnMaxLifetime` задаются при сборке
приложения. Repository хранит указатель и не создаёт отдельный pool на запрос.

### Почему self-transfer особенно опасен

При балансе `100` две независимые копии получают одно исходное значение:

```text
from.Balance = 100 - 10 = 90
to.Balance   = 100 + 10 = 110

save(from) -> 90
save(to)   -> 110
```

Итоговый баланс равен `110`. Проверка `fromID != toID` должна выполняться до
начала транзакции.

### Почему atomic не исправляет аналитику

`atomic.Int64` уберёт data race, но счётчик останется:

- отдельным для каждой реплики;
- обнуляемым при рестарте;
- несогласованным с commit при падении процесса;
- неоднозначным без определения, является `Send` новым значением или delta.

Точная аналитика требует durable-источника. Process-local counter подходит для
observability, но имеет другую гарантию.

---

## Целевая модель

Минимальная схема хранит баланс, логическую операцию и outbox:

```sql
CREATE TABLE accounts (
    id            uuid PRIMARY KEY,
    currency      char(3) NOT NULL,
    balance_minor bigint NOT NULL CHECK (balance_minor >= 0),
    version       bigint NOT NULL DEFAULT 0
);

CREATE TABLE transfers (
    id                 uuid PRIMARY KEY,
    from_account_id    uuid NOT NULL REFERENCES accounts (id),
    to_account_id      uuid NOT NULL REFERENCES accounts (id),
    amount_minor       bigint NOT NULL CHECK (amount_minor > 0),
    currency           char(3) NOT NULL,
    status             text NOT NULL
        CHECK (status IN ('processing', 'completed')),
    from_balance_after bigint,
    to_balance_after   bigint,
    created_at         timestamptz NOT NULL
);

CREATE TABLE outbox (
    event_id      text PRIMARY KEY,
    event_type    text NOT NULL,
    aggregate_id  uuid NOT NULL,
    payload       jsonb NOT NULL,
    created_at    timestamptz NOT NULL,
    published_at  timestamptz
);
```

`amount_minor` хранит целое число минимальных единиц валюты. Одного поля
`amount` недостаточно для multi-currency системы: scale зависит от валюты.
Перевод между разными валютами требует отдельного FX-контракта и здесь
отклоняется.

`transfers.id` одновременно является идентификатором операции и ключом
идемпотентности. Строка `completed` становится видимой только после commit,
потому что запись создаётся и обновляется внутри одной транзакции.

---

## Исправленное решение

Ниже reference implementation основного пути. Генерация и проверка UUID обычно
выполняются на API-границе; сервис всё равно валидирует непустые значения и
денежные инварианты.

<details>
<summary>Показать решение</summary>

```go
var (
    ErrInvalidTransfer     = errors.New("invalid transfer")
    ErrAccountNotFound     = errors.New("account not found")
    ErrInsufficientFunds   = errors.New("insufficient funds")
    ErrCurrencyMismatch    = errors.New("currency mismatch")
    ErrIdempotencyConflict = errors.New("idempotency conflict")
    ErrBalanceOverflow     = errors.New("balance overflow")
)

const maxBalanceMinor int64 = 1<<63 - 1

type TransferCommand struct {
    TransferID  string
    FromID      string
    ToID        string
    AmountMinor int64
    Currency    string
}

type TransferOutcome string

const (
    TransferCommitted       TransferOutcome = "committed"
    TransferIdempotentReplay TransferOutcome = "idempotent_replay"
    TransferRejected        TransferOutcome = "rejected"
    TransferFailed          TransferOutcome = "failed"
    TransferCanceled        TransferOutcome = "canceled"
)

type TransferMetrics interface {
    ObserveAttempt(TransferOutcome, time.Duration)
    IncCommitted()
}

type TransferService struct {
    db      *sql.DB
    metrics TransferMetrics
    now     func() time.Time
}

type storedAccount struct {
    ID           string
    Currency     string
    BalanceMinor int64
    Version      int64
}

type balanceChanged struct {
    EventID      string `json:"event_id"`
    TransferID   string `json:"transfer_id"`
    AccountID    string `json:"account_id"`
    DeltaMinor   int64  `json:"delta_minor"`
    BalanceAfter int64  `json:"balance_after_minor"`
    Currency     string `json:"currency"`
    Version      int64  `json:"version"`
    OccurredAt   string `json:"occurred_at"`
}

func NewTransferService(
    db *sql.DB,
    metrics TransferMetrics,
) (*TransferService, error) {
    if db == nil {
        return nil, errors.New("nil database")
    }
    if metrics == nil {
        return nil, errors.New("nil metrics")
    }
    return &TransferService{
        db:      db,
        metrics: metrics,
        now:     time.Now,
    }, nil
}

func (service *TransferService) Transfer(
    ctx context.Context,
    command TransferCommand,
) (err error) {
    startedAt := time.Now()
    var outcome TransferOutcome
    defer func() {
        if outcome == "" {
            outcome = classifyTransferOutcome(err)
        }
        service.metrics.ObserveAttempt(outcome, time.Since(startedAt))
    }()

    if err := validateTransfer(command); err != nil {
        return err
    }

    tx, err := service.db.BeginTx(ctx, &sql.TxOptions{
        Isolation: sql.LevelReadCommitted,
    })
    if err != nil {
        return fmt.Errorf("begin transfer: %w", err)
    }
    defer func() {
        rollbackErr := tx.Rollback()
        if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
            err = errors.Join(
                err,
                fmt.Errorf("rollback transfer: %w", rollbackErr),
            )
        }
    }()

    firstID, secondID := command.FromID, command.ToID
    if firstID > secondID {
        firstID, secondID = secondID, firstID
    }

    first, err := loadAccountForUpdate(ctx, tx, firstID)
    if err != nil {
        return err
    }
    second, err := loadAccountForUpdate(ctx, tx, secondID)
    if err != nil {
        return err
    }

    accounts := map[string]storedAccount{
        first.ID:  first,
        second.ID: second,
    }
    from := accounts[command.FromID]
    to := accounts[command.ToID]

    isNew, err := reserveTransfer(ctx, tx, command, service.now().UTC())
    if err != nil {
        return err
    }
    if !isNew {
        outcome = TransferIdempotentReplay
        return nil
    }

    if from.Currency != command.Currency ||
        to.Currency != command.Currency {
        return ErrCurrencyMismatch
    }
    if from.BalanceMinor < command.AmountMinor {
        return ErrInsufficientFunds
    }
    if to.BalanceMinor > maxBalanceMinor-command.AmountMinor {
        return ErrBalanceOverflow
    }

    from.BalanceMinor -= command.AmountMinor
    from.Version++
    to.BalanceMinor += command.AmountMinor
    to.Version++

    if err := storeAccount(ctx, tx, from); err != nil {
        return fmt.Errorf("store debit account: %w", err)
    }
    if err := storeAccount(ctx, tx, to); err != nil {
        return fmt.Errorf("store credit account: %w", err)
    }

    occurredAt := service.now().UTC()
    events := []balanceChanged{
        {
            EventID:      command.TransferID + ":debit",
            TransferID:   command.TransferID,
            AccountID:    from.ID,
            DeltaMinor:   -command.AmountMinor,
            BalanceAfter: from.BalanceMinor,
            Currency:     command.Currency,
            Version:      from.Version,
            OccurredAt:   occurredAt.Format(time.RFC3339Nano),
        },
        {
            EventID:      command.TransferID + ":credit",
            TransferID:   command.TransferID,
            AccountID:    to.ID,
            DeltaMinor:   command.AmountMinor,
            BalanceAfter: to.BalanceMinor,
            Currency:     command.Currency,
            Version:      to.Version,
            OccurredAt:   occurredAt.Format(time.RFC3339Nano),
        },
    }
    for _, event := range events {
        if err := insertOutboxEvent(ctx, tx, event, occurredAt); err != nil {
            return fmt.Errorf("insert outbox event %q: %w", event.EventID, err)
        }
    }

    if _, err := tx.ExecContext(
        ctx,
        `UPDATE transfers
         SET status = 'completed',
             from_balance_after = $1,
             to_balance_after = $2
         WHERE id = $3`,
        from.BalanceMinor,
        to.BalanceMinor,
        command.TransferID,
    ); err != nil {
        return fmt.Errorf("complete transfer: %w", err)
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("commit transfer: %w", err)
    }

    outcome = TransferCommitted
    service.metrics.IncCommitted()
    return nil
}

func classifyTransferOutcome(err error) TransferOutcome {
    switch {
    case err == nil:
        return TransferCommitted
    case errors.Is(err, context.Canceled),
        errors.Is(err, context.DeadlineExceeded):
        return TransferCanceled
    case errors.Is(err, ErrInvalidTransfer),
        errors.Is(err, ErrAccountNotFound),
        errors.Is(err, ErrInsufficientFunds),
        errors.Is(err, ErrCurrencyMismatch),
        errors.Is(err, ErrIdempotencyConflict),
        errors.Is(err, ErrBalanceOverflow):
        return TransferRejected
    default:
        return TransferFailed
    }
}

func validateTransfer(command TransferCommand) error {
    switch {
    case command.TransferID == "":
        return fmt.Errorf("%w: empty transfer id", ErrInvalidTransfer)
    case command.FromID == "" || command.ToID == "":
        return fmt.Errorf("%w: empty account id", ErrInvalidTransfer)
    case command.FromID == command.ToID:
        return fmt.Errorf("%w: equal account ids", ErrInvalidTransfer)
    case command.AmountMinor <= 0:
        return fmt.Errorf("%w: amount must be positive", ErrInvalidTransfer)
    case command.Currency == "":
        return fmt.Errorf("%w: empty currency", ErrInvalidTransfer)
    default:
        return nil
    }
}

func reserveTransfer(
    ctx context.Context,
    tx *sql.Tx,
    command TransferCommand,
    createdAt time.Time,
) (bool, error) {
    var insertedID string
    err := tx.QueryRowContext(
        ctx,
        `INSERT INTO transfers (
             id,
             from_account_id,
             to_account_id,
             amount_minor,
             currency,
             status,
             created_at
         )
         VALUES ($1, $2, $3, $4, $5, 'processing', $6)
         ON CONFLICT (id) DO NOTHING
         RETURNING id`,
        command.TransferID,
        command.FromID,
        command.ToID,
        command.AmountMinor,
        command.Currency,
        createdAt,
    ).Scan(&insertedID)
    if err == nil {
        return true, nil
    }
    if !errors.Is(err, sql.ErrNoRows) {
        return false, fmt.Errorf("reserve transfer: %w", err)
    }

    var existing TransferCommand
    var status string
    err = tx.QueryRowContext(
        ctx,
        `SELECT id, from_account_id, to_account_id,
                amount_minor, currency, status
         FROM transfers
         WHERE id = $1`,
        command.TransferID,
    ).Scan(
        &existing.TransferID,
        &existing.FromID,
        &existing.ToID,
        &existing.AmountMinor,
        &existing.Currency,
        &status,
    )
    if err != nil {
        return false, fmt.Errorf("read existing transfer: %w", err)
    }
    if existing != command {
        return false, ErrIdempotencyConflict
    }
    if status != "completed" {
        return false, fmt.Errorf("unexpected transfer status %q", status)
    }
    return false, nil
}

func loadAccountForUpdate(
    ctx context.Context,
    tx *sql.Tx,
    accountID string,
) (storedAccount, error) {
    var account storedAccount
    err := tx.QueryRowContext(
        ctx,
        `SELECT id, currency, balance_minor, version
         FROM accounts
         WHERE id = $1
         FOR UPDATE`,
        accountID,
    ).Scan(
        &account.ID,
        &account.Currency,
        &account.BalanceMinor,
        &account.Version,
    )
    if errors.Is(err, sql.ErrNoRows) {
        return storedAccount{}, fmt.Errorf(
            "%w: %s",
            ErrAccountNotFound,
            accountID,
        )
    }
    if err != nil {
        return storedAccount{}, fmt.Errorf(
            "lock account %q: %w",
            accountID,
            err,
        )
    }
    return account, nil
}

func storeAccount(
    ctx context.Context,
    tx *sql.Tx,
    account storedAccount,
) error {
    result, err := tx.ExecContext(
        ctx,
        `UPDATE accounts
         SET balance_minor = $1, version = $2
         WHERE id = $3`,
        account.BalanceMinor,
        account.Version,
        account.ID,
    )
    if err != nil {
        return err
    }

    affected, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("read affected rows: %w", err)
    }
    if affected != 1 {
        return fmt.Errorf("affected rows = %d, want 1", affected)
    }
    return nil
}

func insertOutboxEvent(
    ctx context.Context,
    tx *sql.Tx,
    event balanceChanged,
    createdAt time.Time,
) error {
    payload, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("marshal event: %w", err)
    }

    _, err = tx.ExecContext(
        ctx,
        `INSERT INTO outbox (
             event_id,
             event_type,
             aggregate_id,
             payload,
             created_at
         )
         VALUES ($1, 'BalanceChanged', $2, $3, $4)`,
        event.EventID,
        event.AccountID,
        string(payload),
        createdAt,
    )
    return err
}
```

</details>

`TransferService` координирует use case. Внешний publisher не участвует в
транзакции: сервис записывает только durable outbox rows. Account repository при
желании можно выделить отдельно, но интерфейс ради самого интерфейса не улучшает
корректность.

Повтор существующего `transferID` не увеличивает метрику второй раз. Если API
должен вернуть receipt, `reserveTransfer` читает и возвращает сохранённые
`from_balance_after` и `to_balance_after`.

---

## Транзакция, блокировки и retry

`READ COMMITTED` сам по себе не предотвращает lost update. В reference solution
корректность обеспечивают row locks:

1. Оба account ID сортируются.
2. `SELECT ... FOR UPDATE` выполняется сначала для меньшего ID.
3. Балансы проверяются и изменяются только после получения обоих locks.
4. Locks удерживаются до commit или rollback.

Стабильный порядок нужен для встречных операций `A → B` и `B → A`. Без него
первая транзакция может держать `A` и ждать `B`, а вторая — держать `B` и ждать
`A`.

Другой вариант debit — атомарный условный update:

```sql
UPDATE accounts
SET balance_minor = balance_minor - $1,
    version = version + 1
WHERE id = $2
  AND balance_minor >= $1
RETURNING balance_minor, version;
```

Он устраняет отдельный `read → write` для одного счёта, но перевод двух счетов
всё равно требует транзакции. При нескольких обновляемых строках остаётся вопрос
порядка locks.

Deadlock или serialization failure требуют повтора всей транзакции, а не только
последнего `UPDATE`. Retry должен быть ограничен, использовать backoff с jitter и
тот же `transferID`. Если `Commit` вернул неопределённую сетевую ошибку,
idempotency key позволяет безопасно прочитать или повторить итог.

---

## События и transactional outbox

Прямой `Publish` нельзя согласовать с локальной SQL-транзакцией:

- publish до commit может сообщить о будущем rollback;
- publish после commit может не выполниться из-за crash;
- возврат ошибки после publish провоцирует повтор уже применённого эффекта.

Outbox rows создаются рядом с balances и transfer record в одной транзакции.
Простой worker открывает транзакцию, получает batch через
`FOR UPDATE SKIP LOCKED`, публикует события, выставляет `published_at` и делает
commit. Этот вариант удерживает DB-транзакцию во время сетевого вызова.

Более масштабируемый worker короткой транзакцией атомарно записывает
`claim_token` и `claim_until`, затем публикует вне транзакции и отдельным
`UPDATE` отмечает успех. Для этого в схему outbox добавляются поля lease, а
протухший claim разрешается взять повторно.

`FOR UPDATE SKIP LOCKED` защищает claim только внутри транзакции. Нельзя
выполнить такой `SELECT` вне транзакции, отпустить lock и считать запись
зарезервированной.

Crash между publish и отметкой создаёт повторную доставку. Поэтому гарантия
обычно `at-least-once`, а consumer дедуплицирует по `event_id`. Поля
`account_id + version` помогают сохранять или проверять порядок изменений
одного счёта.

---

## Метрики и аналитика

Нужно разделить два требования:

| Инструмент | Для чего | Гарантия |
| --- | --- | --- |
| Process-local counter | Alerts, dashboards, throughput | Может потерять increment при crash после commit |
| Таблица `transfers` | Точная история завершённых операций | Согласована с балансами транзакцией |
| Событие `TransferCompleted` или CDC | Загрузка аналитики | Durable, но downstream обычно принимает дубли |

Operational counter увеличивается один раз после успешного commit:

```text
balance_service_transfer_attempts_total{outcome="committed"}
balance_service_transfer_attempts_total{outcome="idempotent_replay"}
balance_service_transfer_attempts_total{outcome="rejected"}
balance_service_transfer_attempts_total{outcome="failed"}
balance_service_transfer_attempts_total{outcome="canceled"}

balance_service_transfers_committed_total
```

Первая метрика считает каждый завершившийся вызов сервиса. Вторая увеличивается
только после commit нового transfer. Идемпотентный повтор считается attempt, но
не создаёт ещё один committed transfer.

### Реализация через `client_golang`

Prometheus использует pull model: приложение меняет метрики в памяти и отдаёт их
через `/metrics`. Отдельные `time.Tick` и `Send` для long-running сервиса не
нужны. `Counter` и `CounterVec` уже безопасны для конкурентного вызова `Inc`.

```go
type PrometheusTransferMetrics struct {
    attempts  *prometheus.CounterVec
    committed prometheus.Counter
    duration  *prometheus.HistogramVec
}

func NewPrometheusTransferMetrics(
    registry prometheus.Registerer,
) *PrometheusTransferMetrics {
    metrics := &PrometheusTransferMetrics{
        attempts: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: "balance_service",
                Name:      "transfer_attempts_total",
                Help:      "Number of transfer calls by terminal outcome.",
            },
            []string{"outcome"},
        ),
        committed: prometheus.NewCounter(
            prometheus.CounterOpts{
                Namespace: "balance_service",
                Name:      "transfers_committed_total",
                Help:      "Number of newly committed money transfers.",
            },
        ),
        duration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: "balance_service",
                Name:      "transfer_duration_seconds",
                Help:      "Money transfer duration in seconds.",
                Buckets: []float64{
                    0.005,
                    0.01,
                    0.025,
                    0.05,
                    0.1,
                    0.25,
                    0.5,
                    1,
                    2.5,
                },
            },
            []string{"outcome"},
        ),
    }

    registry.MustRegister(
        metrics.attempts,
        metrics.committed,
        metrics.duration,
    )

    for _, outcome := range []TransferOutcome{
        TransferCommitted,
        TransferIdempotentReplay,
        TransferRejected,
        TransferFailed,
        TransferCanceled,
    } {
        metrics.attempts.WithLabelValues(string(outcome))
        metrics.duration.WithLabelValues(string(outcome))
    }

    return metrics
}

func (metrics *PrometheusTransferMetrics) ObserveAttempt(
    outcome TransferOutcome,
    duration time.Duration,
) {
    label := string(outcome)
    metrics.attempts.WithLabelValues(label).Inc()
    metrics.duration.WithLabelValues(label).Observe(duration.Seconds())
}

func (metrics *PrometheusTransferMetrics) IncCommitted() {
    metrics.committed.Inc()
}
```

`prometheus.Registerer` передаётся зависимостью, поэтому production может
использовать общий registry, а тест — новый изолированный
`prometheus.NewRegistry()`. Инициализация известных outcomes создаёт нулевые
series до первого события и упрощает dashboards.

Подключение endpoint:

```go
registry := prometheus.NewRegistry()
transferMetrics := NewPrometheusTransferMetrics(registry)

transferService, err := NewTransferService(db, transferMetrics)
if err != nil {
    return err
}

mux.Handle(
    "/metrics",
    promhttp.HandlerFor(
        registry,
        promhttp.HandlerOpts{},
    ),
)
```

Prometheus сам добавляет target labels вроде `instance` и `pod` через
service discovery и scrape configuration. Их не нужно передавать в каждый
`Inc`.

### Labels и cardinality

Набор labels должен быть маленьким и ограниченным. В примере `outcome` принимает
пять заранее известных значений. Нельзя помещать в labels:

- `transferID`;
- `accountID` и `userID`;
- текст ошибки;
- произвольный URL или payload.

Каждая уникальная комбинация labels создаёт новую time series. Идентификаторы
операций остаются в structured logs и traces.

### Запросы PromQL

```promql
# Новые committed-переводы за последние 24 часа.
sum(increase(balance_service_transfers_committed_total[24h]))

# Среднее число новых переводов в секунду за последние 5 минут.
sum(rate(balance_service_transfers_committed_total[5m]))

# Скорость вызовов по каждому terminal outcome.
sum by (outcome) (
  rate(balance_service_transfer_attempts_total[5m])
)

# Доля инфраструктурных ошибок.
sum(rate(balance_service_transfer_attempts_total{outcome="failed"}[5m]))
/
sum(rate(balance_service_transfer_attempts_total[5m]))
```

Counter обнуляется при рестарте процесса. `rate` и `increase` учитывают этот
reset, а `sum` объединяет series разных реплик. Raw-значение counter означает
накопление текущего процесса и не является lifetime total всей системы.

Если нужен точный business count, аналитика считает committed rows в
`transfers` либо обрабатывает durable `TransferCompleted`. Обновление одной
общей строки `total = total + 1` внутри каждой транзакции создаст hot row и
сериализует независимые переводы.

Даже increment после commit может потеряться, если процесс упадёт между commit
и `IncCommitted` или Prometheus пропустит scrape. Это приемлемо для monitoring,
но не для финансовой сверки.

---

## Пример теста

Конкурентный повтор одного `transferID` должен изменить балансы один раз,
создать две outbox-записи и увеличить committed counter один раз. Такой тест
нужен на реальном PostgreSQL: mock драйвера не воспроизводит unique conflict,
ожидание транзакции и row locks.

```go
func TestTransfer_ConcurrentDuplicateCommitsOnce(test *testing.T) {
    fixture := newTransferFixture(test)
    const (
        fromID = "00000000-0000-7000-8000-000000000101"
        toID   = "00000000-0000-7000-8000-000000000102"
    )
    fixture.insertAccount(fromID, "RUB", 1_000)
    fixture.insertAccount(toID, "RUB", 0)

    command := TransferCommand{
        TransferID:  "00000000-0000-7000-8000-000000000001",
        FromID:      fromID,
        ToID:        toID,
        AmountMinor: 250,
        Currency:    "RUB",
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    const callers = 10
    start := make(chan struct{})
    errorsByCaller := make(chan error, callers)

    var waitGroup sync.WaitGroup
    for range callers {
        waitGroup.Add(1)
        go func() {
            defer waitGroup.Done()
            <-start
            errorsByCaller <- fixture.service.Transfer(ctx, command)
        }()
    }

    close(start)
    waitGroup.Wait()
    close(errorsByCaller)

    for err := range errorsByCaller {
        if err != nil {
            test.Errorf("Transfer: %v", err)
        }
    }
    if test.Failed() {
        return
    }

    if got := fixture.balance(fromID); got != 750 {
        test.Fatalf("from balance = %d, want 750", got)
    }
    if got := fixture.balance(toID); got != 250 {
        test.Fatalf("to balance = %d, want 250", got)
    }
    if got := fixture.countTransfers(command.TransferID); got != 1 {
        test.Fatalf("transfer rows = %d, want 1", got)
    }
    if got := fixture.countOutbox(command.TransferID); got != 2 {
        test.Fatalf("outbox rows = %d, want 2", got)
    }
    if got := fixture.metrics.Committed(); got != 1 {
        test.Fatalf("committed metric = %d, want 1", got)
    }
}
```

`start` создаёт одновременную конкуренцию без `time.Sleep`. Timeout ограничивает
зависание теста, но не используется для упорядочивания goroutines. Fixture
должна поднимать изолированный PostgreSQL, применять показанную схему и
закрывать pool через `test.Cleanup`.

---

## Что проверить тестами

- Happy path сохраняет два новых баланса, один transfer и два outbox events.
- Ошибка credit или вставки outbox откатывает debit.
- Недостаток средств возвращает `ErrInsufficientFunds` и ничего не меняет.
- Отсутствие одного account откатывает всю операцию.
- Self-transfer, нулевая и отрицательная суммы отклоняются до SQL.
- Accounts разных валют не изменяются.
- Повтор того же `transferID` и payload возвращает успех без нового списания.
- Тот же `transferID` с другим payload возвращает
  `ErrIdempotencyConflict`.
- Много конкурентных списаний не уводят баланс в минус и не теряют updates.
- Встречные переводы завершаются без взаимной блокировки либо целиком
  повторяются после retryable DB error.
- Parent context отменяет ожидание pool, lock и запросы.
- Outbox worker допускает повторную доставку, а consumer дедуплицирует
  `event_id`.
- Точная аналитика совпадает с числом committed transfer rows.
- Каждый terminal path увеличивает `transfer_attempts_total` ровно один раз.
- Идемпотентный replay не увеличивает `transfers_committed_total`.
- Метрики регистрируются в изолированном `prometheus.NewRegistry` без
  конфликтов с другими тестами.
- Локальные конкурентные тесты дополнительно проходят с `go test -race`.

---

## Interview-ready answer

**1. Какие проблемы назвать первыми?**

- Атомарность — debit, credit и события сейчас могут завершиться частично.
- Конкуренция — `read → calculate → write` теряет обновления без row locks.
- Повтор — без `transferID` неизвестный результат нельзя безопасно повторить.
- Деньги — нужны integer minor units, положительная сумма и проверка валюты.

**2. Достаточно ли `READ COMMITTED`?**

- Нет — уровень изоляции сам по себе не защищает текущую read-modify-write
  последовательность.
- Защита — оба счёта блокируются через `FOR UPDATE` в стабильном порядке либо
  используются согласованные атомарные updates.
- Граница — locks удерживаются одной транзакцией до commit или rollback.

**3. Почему нужен transactional outbox?**

- Согласованность — balances, transfer и event records фиксируются одним commit.
- Доставка — worker публикует только committed events.
- Ограничение — возможны дубли после crash, поэтому consumer идемпотентен по
  `event_id`.

**4. Как считать переводы?**

- Observability — process-local counter увеличивается после commit.
- Точная аналитика — таблица `transfers`, CDC или durable event.
- Масштабирование — одна общая строка-счётчик станет hot row.

**5. Что проверять интеграционно?**

- PostgreSQL — конкурентные списания, unique idempotency key, row locks и
  rollback.
- Инвариант — сумма двух балансов сохраняется, а один transfer создаёт ровно
  два balance events.
- Повторы — одинаковый ID применяется один раз, конфликтующий payload
  отклоняется.

---

## Связанные материалы

- [Списание баланса и ledger](./03-balance-withdrawal-and-ledger.md)
- [PostgreSQL: транзакции и блокировки](../../../06-databases/database-systems-catalog/postgresql/04-transactions-and-locking.md)
- [PostgreSQL: outbox и идемпотентность](../../../06-databases/database-systems-catalog/postgresql/14-outbox-and-idempotency.md)
- [Saga и transactional outbox](../../../04-architecture-and-patterns/patterns/09-saga-and-outbox.md)
- [Тестирование баз данных](../../../09-testing-and-quality/07-database-testing.md)
- [Prometheus: metric types](https://prometheus.io/docs/concepts/metric_types/)
- [Prometheus: metric and label naming](https://prometheus.io/docs/practices/naming/)
- [`client_golang`](https://github.com/prometheus/client_golang)
