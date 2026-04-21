# shopspring/decimal

`github.com/shopspring/decimal` — точная десятичная арифметика. Используется везде, где нельзя допустить ошибки округления.

## Проблема float64

```go
fmt.Println(0.1 + 0.2)          // 0.30000000000000004
fmt.Println(1.005 * 100)        // 100.50000000000001

price := 19.99
discount := 0.1
fmt.Println(price * discount)   // 1.9990000000000001
```

`float64` хранит числа в двоичной системе счисления, поэтому десятичные дроби представляются приближённо. Для денег и финансовых расчётов это неприемлемо.

## Основные операции

```go
import "github.com/shopspring/decimal"

// Конструкторы
price, err := decimal.NewFromString("19.99")
d1 := decimal.NewFromFloat(19.99)
d2 := decimal.New(1999, -2)     // 1999 * 10^-2 = 19.99
d3 := decimal.NewFromInt(20)

// Арифметика — все методы возвращают новый Decimal (immutable)
sum  := d1.Add(d2)
diff := d1.Sub(d2)
prod := d1.Mul(d3)
quot := d1.Div(d3)

fmt.Println(d1.Add(decimal.NewFromFloat(0.1).
    Add(decimal.NewFromFloat(0.2))))  // 20.29 (точно)

// Сравнение
d1.Equal(d2)
d1.GreaterThan(d2)
d1.LessThanOrEqual(d2)
d1.IsZero()
d1.IsNegative()

// Округление
d1.Round(2)      // banker's rounding (half to even)
d1.RoundBank(2)  // то же самое
d1.RoundCeil(2)  // всегда вверх
d1.RoundFloor(2) // всегда вниз
d1.Truncate(2)   // обрезать без округления

// Строковое представление
d1.String()           // "19.99"
d1.StringFixed(2)     // "19.99" (всегда 2 знака после запятой)
d1.StringFixed(0)     // "20" (с округлением)
```

## JSON

`decimal.Decimal` реализует `json.Marshaler` / `json.Unmarshaler`. Сериализуется как строка:

```go
type Order struct {
    Total    decimal.Decimal `json:"total"`
    Discount decimal.Decimal `json:"discount"`
}

// JSON: {"total": "19.99", "discount": "2.00"}
// НЕ: {"total": 19.99} — float в JSON тоже теряет точность
```

## PostgreSQL

Хранить в типе `NUMERIC` или `DECIMAL`, не в `FLOAT`:

```go
// Сканирование — pgx не умеет напрямую в decimal.Decimal
var priceStr string
row.Scan(&priceStr)
price, err := decimal.NewFromString(priceStr)

// INSERT — отправляем как string
_, err = pool.Exec(ctx,
    "INSERT INTO products (price) VALUES ($1)",
    price.String(),
)

// Или через pgtype.Numeric
import "github.com/jackc/pgx/v5/pgtype"
var num pgtype.Numeric
row.Scan(&num)
// num.Int — *big.Int, num.Exp — int32; конвертировать вручную
```

## Типичные ошибки

```go
// Плохо: NewFromFloat теряет точность float64
d := decimal.NewFromFloat(0.1 + 0.2)  // уже неточно до создания

// Хорошо: строить из строки или целых чисел
d1 := decimal.NewFromString("0.1")
d2 := decimal.NewFromString("0.2")
fmt.Println(d1.Add(d2))  // "0.3" — точно
```

## Когда использовать

- любые денежные суммы, цены, баланс
- финансовые расчёты: проценты, налоги, скидки
- crypto/fiat amounts
- медицинские измерения с точностью

**Никогда не использовать `float64` для денег.**

## Interview-ready answer

`float64` хранит числа в двоичной системе, поэтому десятичные дроби вроде `0.1` и `0.2` имеют ошибки представления. `shopspring/decimal` хранит число как integer + exponent без потерь точности. Сериализуется в JSON как строка, в PostgreSQL хранится как `NUMERIC`. Для сложения, умножения и сравнения используются методы объекта — не арифметические операторы Go.
