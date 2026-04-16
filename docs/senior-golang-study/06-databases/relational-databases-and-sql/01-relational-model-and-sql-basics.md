# Relational Model And SQL Basics

Реляционная база данных хранит данные в таблицах и позволяет связывать их через ключи.

## Основные сущности

`table`:
- набор строк одного типа;
- например `users`, `orders`, `payments`.

`row`:
- одна запись в таблице.

`column`:
- одно поле записи;
- например `id`, `email`, `created_at`.

`primary key`:
- уникальный идентификатор строки.

`foreign key`:
- ссылка на строку в другой таблице.

## Пример

```sql
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## CRUD простыми словами

`INSERT`:
- создать строку.

```sql
INSERT INTO users (email) VALUES ('user@example.com');
```

`SELECT`:
- прочитать строки.

```sql
SELECT id, email FROM users WHERE email = 'user@example.com';
```

`UPDATE`:
- изменить строки.

```sql
UPDATE orders SET status = 'paid' WHERE id = 42;
```

`DELETE`:
- удалить строки.

```sql
DELETE FROM users WHERE id = 42;
```

## Constraints

Constraints защищают данные от некорректного состояния.

Частые:
- `PRIMARY KEY`
- `FOREIGN KEY`
- `UNIQUE`
- `NOT NULL`
- `CHECK`

Пример:

```sql
ALTER TABLE orders
ADD CONSTRAINT orders_status_check
CHECK (status IN ('new', 'paid', 'cancelled'));
```

## Почему constraints важны

Application validation недостаточно.

Причины:
- несколько сервисов могут писать в одну БД;
- есть миграции, скрипты, manual fixes;
- concurrency может обойти naive проверки в коде.

Хорошее правило:
- бизнес-инварианты, которые нельзя нарушать, стоит защищать на уровне БД тоже.

## Нормализация простыми словами

Нормализация помогает не дублировать данные без необходимости.

Пример плохой идеи:
- хранить `user_email` в каждой строке `orders`, если email уже есть в `users`.

Но есть trade-off:
- нормализация уменьшает дублирование;
- денормализация иногда ускоряет read path.

## Что важно для Go backend

В Go-коде обычно важно:
- не собирать SQL через string concatenation;
- использовать placeholders;
- явно обрабатывать `sql.ErrNoRows`;
- передавать `context.Context` в запросы;
- задавать timeout;
- не держать transaction дольше нужного.

## Interview-ready summary

Реляционная БД хранит данные в таблицах, а корректность поддерживается ключами, constraints и транзакциями. Хороший backend-разработчик думает не только о запросе, но и о data invariants, concurrency и том, как этот запрос будет жить под нагрузкой.
