package ledger_test

import (
	"testing"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestNewMoney(t *testing.T) {
	t.Run("creates valid money", func(t *testing.T) {
		m, err := ledger.NewMoney(1000, ledger.USD)
		require.NoError(t, err)
		assert.Equal(t, int64(1000), m.Amount())
		assert.Equal(t, ledger.USD, m.Currency())
	})

	t.Run("allows zero amount", func(t *testing.T) {
		m, err := ledger.NewMoney(0, ledger.USD)
		require.NoError(t, err)
		assert.Equal(t, int64(0), m.Amount())
	})

	t.Run("allows negative amount for debits", func(t *testing.T) {
		m, err := ledger.NewMoney(-500, ledger.USD)
		require.NoError(t, err)
		assert.Equal(t, int64(-500), m.Amount())
	})

	t.Run("rejects invalid currency", func(t *testing.T) {
		_, err := ledger.NewMoney(100, ledger.Currency(""))
		assert.Error(t, err)
	})
}

func TestMoneyAdd(t *testing.T) {
	t.Run("adds same currency", func(t *testing.T) {
		a, _ := ledger.NewMoney(1000, ledger.USD)
		b, _ := ledger.NewMoney(500, ledger.USD)

		result, err := a.Add(b)
		require.NoError(t, err)
		assert.Equal(t, int64(1500), result.Amount())
	})

	t.Run("rejects different currencies", func(t *testing.T) {
		a, _ := ledger.NewMoney(1000, ledger.USD)
		b, _ := ledger.NewMoney(500, ledger.MYR)

		_, err := a.Add(b)
		assert.Error(t, err)
	})
}

func TestMoneySubtract(t *testing.T) {
	t.Run("subtracts same currency", func(t *testing.T) {
		a, _ := ledger.NewMoney(1000, ledger.USD)
		b, _ := ledger.NewMoney(300, ledger.USD)

		result, err := a.Subtract(b)
		require.NoError(t, err)
		assert.Equal(t, int64(700), result.Amount())
	})

	t.Run("allows negative result", func(t *testing.T) {
		a, _ := ledger.NewMoney(100, ledger.USD)
		b, _ := ledger.NewMoney(500, ledger.USD)

		result, err := a.Subtract(b)
		require.NoError(t, err)
		assert.Equal(t, int64(-400), result.Amount())
	})
}

func TestMoneyNegate(t *testing.T) {
	m, _ := ledger.NewMoney(1000, ledger.USD)
	neg := m.Negate()
	assert.Equal(t, int64(-1000), neg.Amount())
	assert.Equal(t, ledger.USD, neg.Currency())
}

func TestMoneyIsZero(t *testing.T) {
	zero, _ := ledger.NewMoney(0, ledger.USD)
	nonZero, _ := ledger.NewMoney(100, ledger.USD)

	assert.True(t, zero.IsZero())
	assert.False(t, nonZero.IsZero())
}

func TestMoneyEquals(t *testing.T) {
	a, _ := ledger.NewMoney(1000, ledger.USD)
	b, _ := ledger.NewMoney(1000, ledger.USD)
	c, _ := ledger.NewMoney(500, ledger.USD)
	d, _ := ledger.NewMoney(1000, ledger.MYR)

	assert.True(t, a.Equals(b))
	assert.False(t, a.Equals(c))
	assert.False(t, a.Equals(d))
}

// === Property-Based Tests ===

func TestMoneyPBT_AdditionIsCommutative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		amount1 := rapid.Int64Range(-1_000_000_00, 1_000_000_00).Draw(t, "amount1")
		amount2 := rapid.Int64Range(-1_000_000_00, 1_000_000_00).Draw(t, "amount2")

		a, _ := ledger.NewMoney(amount1, ledger.USD)
		b, _ := ledger.NewMoney(amount2, ledger.USD)

		ab, err1 := a.Add(b)
		ba, err2 := b.Add(a)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.True(t, ab.Equals(ba), "a+b should equal b+a")
	})
}

func TestMoneyPBT_AddThenSubtractIsIdentity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		amount1 := rapid.Int64Range(-1_000_000_00, 1_000_000_00).Draw(t, "amount1")
		amount2 := rapid.Int64Range(-1_000_000_00, 1_000_000_00).Draw(t, "amount2")

		a, _ := ledger.NewMoney(amount1, ledger.USD)
		b, _ := ledger.NewMoney(amount2, ledger.USD)

		result, _ := a.Add(b)
		back, _ := result.Subtract(b)

		assert.True(t, a.Equals(back), "a + b - b should equal a")
	})
}

func TestMoneyPBT_NegateIsInvolution(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		amount := rapid.Int64Range(-1_000_000_00, 1_000_000_00).Draw(t, "amount")
		m, _ := ledger.NewMoney(amount, ledger.USD)

		doubleNeg := m.Negate().Negate()
		assert.True(t, m.Equals(doubleNeg), "negate(negate(m)) should equal m")
	})
}

func TestMoneyPBT_AddNegateIsZero(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		amount := rapid.Int64Range(-1_000_000_00, 1_000_000_00).Draw(t, "amount")
		m, _ := ledger.NewMoney(amount, ledger.USD)

		result, _ := m.Add(m.Negate())
		assert.True(t, result.IsZero(), "m + (-m) should be zero")
	})
}
