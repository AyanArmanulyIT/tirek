package money

// Money is the single money representation in Tirek.
//
// Rule: all monetary values are integer minor units (1 KZT = 100 tiyn,
// 1 USD = 100 cents) plus an ISO 4217 currency code. Floating point is
// never used for money anywhere in the codebase.

type Money struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"` // ISO 4217, e.g. "KZT"
}

const (
	KZT = "KZT"
	USD = "USD"
	EUR = "EUR"
)

var supported = map[string]struct{}{
	KZT: {}, USD: {}, EUR: {},
}

// New validates and constructs a Money value.
func New(amountMinor int64, currency string) (Money, error) {
	if _, ok := supported[currency]; !ok {
		return Money{}, &Error{Code: "UNSUPPORTED_CURRENCY", Msg: "unsupported currency: " + currency}
	}
	if amountMinor < 0 {
		return Money{}, &Error{Code: "NEGATIVE_AMOUNT", Msg: "amount must be non-negative"}
	}
	return Money{AmountMinor: amountMinor, Currency: currency}, nil
}

// MustNew panics on invalid input; use only for constants in tests/seeds.
func MustNew(amountMinor int64, currency string) Money {
	m, err := New(amountMinor, currency)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Money) IsZero() bool { return m.AmountMinor == 0 }

func (m Money) Add(o Money) (Money, error) {
	if m.Currency != o.Currency {
		return Money{}, &Error{Code: "CURRENCY_MISMATCH", Msg: "cannot add different currencies"}
	}
	return Money{AmountMinor: m.AmountMinor + o.AmountMinor, Currency: m.Currency}, nil
}

func (m Money) Sub(o Money) (Money, error) {
	if m.Currency != o.Currency {
		return Money{}, &Error{Code: "CURRENCY_MISMATCH", Msg: "cannot subtract different currencies"}
	}
	return Money{AmountMinor: m.AmountMinor - o.AmountMinor, Currency: m.Currency}, nil
}

// Percentage applies a percent rate (basis points) with half-up rounding at
// the smallest unit. Rounding happens exactly once, here.
func (m Money) Percentage(basisPoints int64) Money {
	if basisPoints <= 0 {
		return Money{AmountMinor: 0, Currency: m.Currency}
	}
	// amount * bps / 10000, with half-up rounding
	n := m.AmountMinor * basisPoints
	q := n / 10000
	r := n % 10000
	if 2*r >= 10000 {
		q++
	}
	return Money{AmountMinor: q, Currency: m.Currency}
}

// Error is a domain error with a stable machine code.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }