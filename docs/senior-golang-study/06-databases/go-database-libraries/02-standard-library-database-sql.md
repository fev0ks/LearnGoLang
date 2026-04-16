# Standard Library database/sql

`database/sql` это стандартный Go abstraction layer для работы с SQL databases.

## Содержание

- [Как это выглядит](#как-это-выглядит)
- [Что дает `database/sql`](#что-дает-databasesql)
- [Плюсы](#плюсы)
- [Минусы](#минусы)
- [На что обращать внимание](#на-что-обращать-внимание)
- [Когда выбирать](#когда-выбирать)
- [Interview-ready answer](#interview-ready-answer)

## Как это выглядит

```go
db, err := sql.Open("postgres", dsn)
if err != nil {
    return err
}
defer db.Close()

ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

row := db.QueryRowContext(ctx, `
    SELECT id, email
    FROM users
    WHERE id = $1
`, userID)

var user User
if err := row.Scan(&user.ID, &user.Email); err != nil {
    return err
}
```

## Что дает `database/sql`

- общий API для SQL drivers;
- connection pool;
- `QueryContext`, `ExecContext`, `QueryRowContext`;
- transactions через `BeginTx`;
- prepared statements;
- explicit scan.

## Плюсы

- часть стандартной библиотеки;
- мало магии;
- легко понять, какой SQL реально выполняется;
- удобно для production debugging;
- хорошо подходит для explicit repository layer.

## Минусы

- много boilerplate;
- ручной `Scan`;
- nullable values требуют аккуратности;
- SQL хранится строками;
- ошибки в SQL часто runtime-only.

## На что обращать внимание

Всегда:
- передавай `context.Context`;
- задавай timeout;
- закрывай `rows`;
- проверяй `rows.Err()`;
- настрой pool limits;
- не держи transaction дольше нужного.

## Когда выбирать

`database/sql` хорошо подходит, если:
- хочешь minimum dependencies;
- команда уверенно пишет SQL;
- запросов не очень много;
- важен контроль над каждым query.

Если запросов становится много и много ручного scan:
- можно смотреть в сторону `sqlx` или `sqlc`.

## Interview-ready answer

`database/sql` это стандартный abstraction layer, который дает общий API и connection pooling, но не избавляет от SQL и ручного mapping. Его выбирают, когда нужен контроль и минимум магии.
