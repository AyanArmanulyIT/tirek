package money

import "testing"

func TestMoneyRejectsNegativeAndBadCurrency(t *testing.T) {
	if _, err := New(-1, KZT); err == nil {
		t.Fatal("expected error for negative amount")
	}
	if _, err := New(100, "RUB"); err == nil {
		t.Fatal("expected error for unsupported currency")
	}
}

func TestPercentageRoundsHalfUpOnce(t *testing.T) {
	cases := []struct {
		amount int64
		bps    int64
		want   int64
	}{
		{10000, 1200, 1200},   // 12% of 100.00 KZT
		{10000, 500, 500},     // 5%
		{10001, 500, 500},     // 100.01 * 5% = 5.0005 -> 500 (half-up)
		{10001, 100, 100},     // 100.01 * 1% = 1.0001 -> 100
		{33333, 10000, 33333}, // 100%
		{33333, 15000, 50000}, // 150% -> 49999.5 -> 50000 (half-up)
	}
	for _, c := range cases {
		m := MustNew(c.amount, KZT)
		if got := m.Percentage(c.bps).AmountMinor; got != c.want {
			t.Errorf("Percentage(%d, %d) = %d, want %d", c.amount, c.bps, got, c.want)
		}
	}
}

func TestAddSubCurrencyMismatch(t *testing.T) {
	a := MustNew(100, KZT)
	b := MustNew(100, USD)
	if _, err := a.Add(b); err == nil {
		t.Fatal("expected currency mismatch error on Add")
	}
	if _, err := a.Sub(b); err == nil {
		t.Fatal("expected currency mismatch error on Sub")
	}
}