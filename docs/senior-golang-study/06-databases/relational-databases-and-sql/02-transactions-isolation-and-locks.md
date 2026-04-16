# Transactions Isolation And Locks

Транзакция нужна, чтобы группа операций применялась как одно логическое изменение.

## Содержание

- [ACID простыми словами](#acid-простыми-словами)
- [Пример транзакции](#пример-транзакции)
- [Где должны быть границы транзакции](#где-должны-быть-границы-транзакции)
- [Isolation levels](#isolation-levels)
- [Locks](#locks)
- [Double-write example](#double-write-example)
- [Deadlock](#deadlock)
- [Transaction в Go](#transaction-в-go)
- [Interview-ready summary](#interview-ready-summary)

## ACID простыми словами

`Atomicity`:
- либо все изменения применились;
- либо ничего не применилось.

`Consistency`:
- после транзакции данные остаются в валидном состоянии.

`Isolation`:
- параллельные транзакции не должны ломать друг друга.

`Durability`:
- после commit данные не должны пропасть при обычном сбое.

## Пример транзакции

```sql
BEGIN;

UPDATE accounts
SET balance = balance - 100
WHERE id = 1;

UPDATE accounts
SET balance = balance + 100
WHERE id = 2;

COMMIT;
```

Если посередине ошибка:

```sql
ROLLBACK;
```

## Где должны быть границы транзакции

Хорошее правило:
- транзакция должна включать минимальный набор SQL-операций, которые должны быть атомарны.

Плохая идея:
- открыть транзакцию;
- сходить во внешний HTTP API;
- потом сделать `COMMIT`.

Почему плохо:
- держатся locks;
- занято connection из pool;
- растет шанс deadlock;
- external call может зависнуть.

## Isolation levels

Уровень изоляции определяет, какие эффекты параллельных транзакций видны друг другу.

Частые уровни:
- `READ COMMITTED`
- `REPEATABLE READ`
- `SERIALIZABLE`

Практически:
- `READ COMMITTED` часто default;
- `SERIALIZABLE` сильнее, но дороже и может давать serialization failures;
- выбор зависит от инвариантов и конкуренции.

## Locks

Locks нужны, чтобы защитить данные при конкурирующих изменениях.

Пример:

```sql
SELECT *
FROM orders
WHERE id = 42
FOR UPDATE;
```

Что это значит:
- строка блокируется для конкурирующих updates;
- другая транзакция не сможет одновременно изменить этот order так же свободно.

## Double-write example

Плохой сценарий:
- два запроса одновременно читают `order.status = 'new'`;
- оба решают, что можно оплатить;
- оба списывают деньги или создают payment.

Что помогает:
- транзакция;
- `SELECT ... FOR UPDATE`;
- unique constraints;
- idempotency key;
- state machine.

## Deadlock

Deadlock возникает, когда транзакции ждут друг друга по кругу.

Пример:
- T1 залочила order 1 и ждет order 2;
- T2 залочила order 2 и ждет order 1.

Что помогает:
- брать locks в одном порядке;
- держать транзакции короткими;
- retry transaction при deadlock;
- не делать внешние вызовы внутри транзакции.

## Transaction в Go

Типичный flow:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
    return err
}
defer tx.Rollback()

// queries via tx

if err := tx.Commit(); err != nil {
    return err
}
```

Важно:
- после успешного `Commit` deferred `Rollback` уже ничего не испортит;
- все запросы внутри транзакции должны идти через `tx`, а не через `db`.

## Interview-ready summary

Транзакция нужна, когда несколько изменений должны быть атомарны. Но транзакцию нельзя растягивать: чем дольше она живет, тем больше locks, wait, deadlocks и pressure на connection pool.
