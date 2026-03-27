package account

import (
	"context"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
)

// Repository defines the port for account persistence (Hexagonal Architecture).
// Domain owns the interface; infrastructure implements it.
type Repository interface {
	Save(ctx context.Context, account Account) error
	FindByID(ctx context.Context, id ledger.AccountID) (Account, error)
	FindAll(ctx context.Context) ([]Account, error)
}
