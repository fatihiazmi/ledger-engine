package ledger

import "time"

// EventType identifies the kind of domain event.
type EventType string

const (
	EventAccountOpened      EventType = "account.opened"
	EventAccountSuspended   EventType = "account.suspended"
	EventAccountReactivated EventType = "account.reactivated"
	EventAccountClosed      EventType = "account.closed"
	EventTransactionRecorded EventType = "transaction.recorded"
)

// DomainEvent is the base interface for all events in the system.
// Events are modeled as CHANGES, not absolute states (Cloud Native Patterns).
type DomainEvent interface {
	EventType() EventType
	AggregateID() string
	OccurredAt() time.Time
}

// AccountOpenedEvent records that a new account was created.
type AccountOpenedEvent struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	Name        string    `json:"name"`
	AccountType string    `json:"account_type"`
	Currency    string    `json:"currency"`
	Timestamp   time.Time `json:"timestamp"`
}

func (e AccountOpenedEvent) EventType() EventType  { return EventAccountOpened }
func (e AccountOpenedEvent) AggregateID() string    { return e.AccountID }
func (e AccountOpenedEvent) OccurredAt() time.Time  { return e.Timestamp }

// TransactionRecordedEvent records a complete balanced transaction.
type TransactionRecordedEvent struct {
	ID            string              `json:"id"`
	TransactionID string              `json:"transaction_id"`
	Entries       []EntryEventData    `json:"entries"`
	Description   string              `json:"description"`
	Timestamp     time.Time           `json:"timestamp"`
}

func (e TransactionRecordedEvent) EventType() EventType  { return EventTransactionRecorded }
func (e TransactionRecordedEvent) AggregateID() string    { return e.TransactionID }
func (e TransactionRecordedEvent) OccurredAt() time.Time  { return e.Timestamp }

// EntryEventData is the serializable representation of a ledger entry within an event.
type EntryEventData struct {
	EntryID   string `json:"entry_id"`
	AccountID string `json:"account_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Type      string `json:"type"`
}
