# Primitive Types And Zero Values

Эта заметка нужна для простых, но частых interview вопросов: какие в Go есть базовые типы и какие у них zero values.

## Содержание

- [Основные группы типов](#основные-группы-типов)
- [`bool`](#bool)
- [`string`](#string)
- [Integer types](#integer-types)
- [Floating-point types](#floating-point-types)
- [Complex types](#complex-types)
- [Zero values у часто спрашиваемых reference-like типов](#zero-values-у-часто-спрашиваемых-reference-like-типов)
- [Мини-таблица](#мини-таблица)
- [Что обычно спрашивают на интервью](#что-обычно-спрашивают-на-интервью)

## Основные группы типов

В Go обычно выделяют:
- boolean;
- numeric types;
- string;
- pointers;
- functions;
- arrays;
- slices;
- maps;
- structs;
- interfaces;
- channels.

Если говорить именно про базовые built-in scalar types, то это:
- `bool`
- `string`
- все целочисленные типы
- все floating-point типы
- все complex типы

## `bool`

Тип:

```go
bool
```

Значения:
- `true`
- `false`

Zero value:

```go
false
```

## `string`

Тип:

```go
string
```

Zero value:

```go
""
```

Важно:
- `string` в Go immutable;
- это не `[]byte`, хотя между ними можно конвертировать.

## Integer types

Знаковые:
- `int`
- `int8`
- `int16`
- `int32`
- `int64`

Беззнаковые:
- `uint`
- `uint8`
- `uint16`
- `uint32`
- `uint64`
- `uintptr`

Алиасы:
- `byte` это alias для `uint8`
- `rune` это alias для `int32`

Zero value для всех integer types:

```go
0
```

## Floating-point types

Типы:
- `float32`
- `float64`

Zero value:

```go
0
```

Практически чаще используется:
- `float64`

## Complex types

Типы:
- `complex64`
- `complex128`

Zero value:

```go
0 + 0i
```

## Zero values у часто спрашиваемых reference-like типов

`pointer`:

```go
nil
```

`slice`:

```go
nil
```

`map`:

```go
nil
```

`chan`:

```go
nil
```

`func`:

```go
nil
```

`interface`:

```go
nil
```

Важно:
- `nil` slice можно `append`-ить;
- в `nil` map писать нельзя;
- `nil` channel блокирует send и receive;
- `nil` interface и interface с `nil` внутри это не одно и то же.

## Мини-таблица

| Type | Example | Zero value |
| --- | --- | --- |
| bool | `bool` | `false` |
| string | `string` | `""` |
| integers | `int64`, `uint32` | `0` |
| floats | `float64` | `0` |
| complex | `complex128` | `0+0i` |
| pointer | `*T` | `nil` |
| slice | `[]T` | `nil` |
| map | `map[K]V` | `nil` |
| chan | `chan T` | `nil` |
| func | `func()` | `nil` |
| interface | `any` | `nil` |

## Что обычно спрашивают на интервью

- чем `byte` отличается от `rune`;
- какой zero value у `map`, `slice`, `chan`;
- почему `nil` map и `nil` slice ведут себя по-разному;
- почему `string` не равен `[]byte`.
