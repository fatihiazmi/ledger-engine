package transfer

import (
	"errors"
	"time"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/google/uuid"
)

var (
	ErrSameAccount       = errors.New("cannot transfer to the same account")
	ErrInvalidTransition = errors.New("invalid saga state transition")
)

// SagaState represents the lifecycle of a transfer saga.
// Entity state object pattern (Secure by Design — Ch. 7).
type SagaState string

const (
	Initiated  SagaState = "INITIATED"
	Debited    SagaState = "DEBITED"
	Completed  SagaState = "COMPLETED"
	Failed     SagaState = "FAILED"
	Compensated SagaState = "COMPENSATED"
)

// Transfer is the saga aggregate tracking a multi-step money transfer.
type Transfer struct {
	id          string
	fromAccount ledger.AccountID
	toAccount   ledger.AccountID
	amount      ledger.Money
	description string
	state       SagaState
	failReason  string
	steps       []SagaStep
	createdAt   time.Time
	updatedAt   time.Time
}

// SagaStep records each step in the saga lifecycle for auditability.
type SagaStep struct {
	Action    string    `json:"action"`
	State     SagaState `json:"state"`
	Detail    string    `json:"detail"`
	Timestamp time.Time `json:"timestamp"`
}

// NewTransfer creates a new transfer saga in INITIATED state.
func NewTransfer(from, to ledger.AccountID, amount ledger.Money, description string) (Transfer, error) {
	if from.Equals(to) {
		return Transfer{}, ErrSameAccount
	}

	now := time.Now().UTC()
	t := Transfer{
		id:          uuid.Must(uuid.NewV7()).String(),
		fromAccount: from,
		toAccount:   to,
		amount:      amount,
		description: description,
		state:       Initiated,
		createdAt:   now,
		updatedAt:   now,
	}
	t.addStep("transfer.initiated", Initiated, "Transfer created")
	return t, nil
}

// MarkDebited transitions from INITIATED → DEBITED after source account is debited.
func (t *Transfer) MarkDebited() error {
	if t.state != Initiated {
		return ErrInvalidTransition
	}
	t.state = Debited
	t.updatedAt = time.Now().UTC()
	t.addStep("source.debited", Debited, "Source account debited")
	return nil
}

// MarkCompleted transitions from DEBITED → COMPLETED after destination is credited.
func (t *Transfer) MarkCompleted() error {
	if t.state != Debited {
		return ErrInvalidTransition
	}
	t.state = Completed
	t.updatedAt = time.Now().UTC()
	t.addStep("destination.credited", Completed, "Transfer completed successfully")
	return nil
}

// MarkFailed transitions to FAILED state with a reason.
func (t *Transfer) MarkFailed(reason string) {
	t.state = Failed
	t.failReason = reason
	t.updatedAt = time.Now().UTC()
	t.addStep("transfer.failed", Failed, reason)
}

// MarkCompensated transitions from FAILED → COMPENSATED after debit is reversed.
func (t *Transfer) MarkCompensated() {
	t.state = Compensated
	t.updatedAt = time.Now().UTC()
	t.addStep("debit.reversed", Compensated, "Compensating transaction applied")
}

func (t *Transfer) addStep(action string, state SagaState, detail string) {
	t.steps = append(t.steps, SagaStep{
		Action:    action,
		State:     state,
		Detail:    detail,
		Timestamp: time.Now().UTC(),
	})
}

func (t Transfer) ID() string               { return t.id }
func (t Transfer) FromAccount() ledger.AccountID { return t.fromAccount }
func (t Transfer) ToAccount() ledger.AccountID   { return t.toAccount }
func (t Transfer) Amount() ledger.Money      { return t.amount }
func (t Transfer) Description() string       { return t.description }
func (t Transfer) State() SagaState          { return t.state }
func (t Transfer) FailReason() string        { return t.failReason }
func (t Transfer) Steps() []SagaStep         { return t.steps }
func (t Transfer) CreatedAt() time.Time      { return t.createdAt }
func (t Transfer) UpdatedAt() time.Time      { return t.updatedAt }
