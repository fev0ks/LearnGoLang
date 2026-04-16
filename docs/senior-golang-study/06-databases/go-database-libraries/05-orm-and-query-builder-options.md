# ORM And Query Builder Options

В Go часто спорят про ORM. Важно не занимать религиозную позицию, а понимать trade-offs.

## Содержание

- [GORM](#gorm)
- [Ent](#ent)
- [Bun](#bun)
- [Главный риск ORM](#главный-риск-orm)
- [Interview-ready answer](#interview-ready-answer)

## GORM

`GORM` это популярный ORM для Go.

Что дает:
- model mapping;
- associations;
- hooks;
- transactions;
- eager loading;
- migrations-like tooling;
- CRUD API.

Плюсы:
- быстро стартовать;
- удобно для CRUD-heavy приложений;
- много готовых возможностей;
- низкий порог входа для простых cases.

Минусы:
- много магии;
- сложнее контролировать SQL;
- легче получить N+1;
- сложнее debugging slow queries, если команда не смотрит generated SQL.

Когда уместен:
- internal admin;
- CRUD-heavy сервис;
- быстрый MVP;
- команда понимает ORM trade-offs.

## Ent

`Ent` это entity framework для Go с schema-first и code generation подходом.

Что дает:
- typed schema;
- generated query API;
- relationships;
- graph-like modeling;
- compile-time friendly API.

Плюсы:
- сильная type-safety;
- понятная schema-as-code модель;
- удобно для сложных domain entities.

Минусы:
- framework lock-in;
- code generation;
- нужно учить conventions;
- raw SQL path может быть менее прямым, чем в SQL-first подходах.

Когда уместен:
- сложная модель данных;
- много relationships;
- хочется typed API поверх DB.

## Bun

`Bun` позиционируется как SQL-first ORM/query builder.

Что дает:
- struct mapping;
- query builder;
- relations;
- multi-database support;
- SQL-first style.

Плюсы:
- ближе к SQL, чем классический ORM;
- удобнее ручного `Scan`;
- меньше boilerplate.

Минусы:
- еще один abstraction layer;
- надо понимать generated SQL;
- не заменяет знание индексов и query plans.

Когда уместен:
- хочется query builder и mapping;
- но не хочется полностью уходить от SQL.

## Главный риск ORM

ORM удобен, пока:
- запросы простые;
- нагрузка умеренная;
- команда смотрит, какой SQL генерируется.

ORM начинает вредить, когда:
- никто не понимает generated SQL;
- появляются сложные joins;
- N+1 проходит в production;
- query plan никто не смотрит.

## Interview-ready answer

ORM не плох сам по себе. Он ускоряет CRUD и уменьшает boilerplate, но добавляет abstraction cost. Для production backend важно уметь посмотреть generated SQL, explain plan и понять, где ORM делает не то, что ты ожидал.
