# Comparison Table

Таблица ниже не говорит "всегда бери X". Она помогает понять trade-offs.

## Быстрое сравнение

| Подход | Что это | Плюсы | Минусы | Когда выбирать |
| --- | --- | --- | --- | --- |
| `database/sql` | стандартный SQL abstraction layer | стандартная библиотека, predictable API, много драйверов | много boilerplate, ручной scan, нет type-safe queries | простой сервис, полный контроль SQL, минимум зависимостей |
| `pgx` | PostgreSQL driver/toolkit | PostgreSQL-specific features, хороший контроль, быстрый драйвер | привязка к Postgres API, больше явного кода | PostgreSQL-first сервисы |
| `pgxpool` | pool поверх `pgx` | удобный pooling, нативный Postgres workflow | надо понимать pool и lifecycle | production Go + PostgreSQL |
| `sqlx` | расширение `database/sql` | меньше boilerplate, struct scan, остается SQL-first | SQL все еще строками, runtime ошибки возможны | хочешь raw SQL, но меньше ручного scan |
| `sqlc` | генератор Go-кода из SQL | type-safe Go methods, SQL остается SQL, compile-time checking | нужен generation step, queries живут отдельно | строгий SQL-first подход, большие команды |
| `GORM` | ORM | быстро стартовать, associations, hooks, migrations | магия, сложнее контролировать SQL, риск N+1 | CRUD-heavy app, admin/internal systems |
| `Ent` | schema-first ORM/entity framework | type-safe API, code generation, graph-like model | сложнее setup, framework lock-in | сложный domain model, нужен typed query API |
| `Bun` | SQL-first ORM/query builder | ближе к SQL, меньше магии чем классический ORM | еще один abstraction layer, надо изучать API | нужен query builder + struct mapping без полного GORM-style ORM |

## Как думать про выбор

Если команда хорошо пишет SQL:
- `database/sql`, `pgx`, `sqlx`, `sqlc`.

Если нужен максимальный контроль PostgreSQL:
- `pgx` или `pgxpool`.

Если хочется SQL-first и type safety:
- `sqlc`.

Если продукт CRUD-heavy и важна скорость:
- `GORM`, `Ent`, `Bun`.

Если важнее debugging slow queries:
- меньше магии обычно лучше.

## Practical warning

Любая библиотека не отменяет:
- миграции;
- индексы;
- транзакции;
- connection pool tuning;
- observability;
- понимание SQL и query plans.

Самая частая ошибка:
- выбрать ORM, чтобы "не думать про SQL";
- а потом не понимать, почему production query стал медленным.
