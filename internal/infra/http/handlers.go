package http

import (
	"encoding/json"
	"net/http"

	"github.com/fatihiazmi/ledger-engine/internal/app"
	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/fatihiazmi/ledger-engine/internal/projection"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc     *app.LedgerService
	queries *projection.PostgresQueryService
}

func NewHandler(svc *app.LedgerService, queries *projection.PostgresQueryService) *Handler {
	return &Handler{svc: svc, queries: queries}
}

// --- Request/Response DTOs ---

type openAccountRequest struct {
	Name        string `json:"name"`
	AccountType string `json:"account_type"` // ASSET, EQUITY, LIABILITY
	Currency    string `json:"currency"`
}

type entryRequest struct {
	AccountID string `json:"account_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Type      string `json:"type"` // DEBIT, CREDIT
}

type recordTransactionRequest struct {
	Entries     []entryRequest `json:"entries"`
	Description string         `json:"description"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// --- Handlers ---

func (h *Handler) OpenAccount(w http.ResponseWriter, r *http.Request) {
	var req openAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	acc, err := h.svc.OpenAccount(r.Context(), app.OpenAccountCmd{
		Name:        req.Name,
		AccountType: req.AccountType,
		Currency:    ledger.Currency(req.Currency),
	})
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"account_id":   acc.ID().String(),
		"name":         acc.Name(),
		"account_type": string(acc.Type()),
		"currency":     string(acc.Currency()),
		"balance":      acc.Balance().Amount(),
		"status":       string(acc.Status()),
	})
}

func (h *Handler) RecordTransaction(w http.ResponseWriter, r *http.Request) {
	var req recordTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	entries := make([]app.EntryCmd, len(req.Entries))
	for i, e := range req.Entries {
		accID, err := ledger.NewAccountID(e.AccountID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid account_id in entry"})
			return
		}
		entries[i] = app.EntryCmd{
			AccountID: accID,
			Amount:    e.Amount,
			Currency:  ledger.Currency(e.Currency),
			Type:      ledger.EntryType(e.Type),
		}
	}

	tx, err := h.svc.RecordTransaction(r.Context(), app.RecordTransactionCmd{
		Entries:     entries,
		Description: req.Description,
	})
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"transaction_id": tx.ID().String(),
		"description":    tx.Description(),
	})
}

func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID   string `json:"account_id"`
		Amount      float64 `json:"amount"` // dollars
		Currency    string `json:"currency"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	accID, err := ledger.NewAccountID(req.AccountID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid account_id"})
		return
	}

	cents := int64(req.Amount * 100)
	if cents <= 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "amount must be positive"})
		return
	}

	desc := req.Description
	if desc == "" {
		desc = "Deposit"
	}

	currency := ledger.Currency(req.Currency)
	if currency == "" {
		currency = ledger.USD
	}

	if err := h.svc.Deposit(r.Context(), accID, cents, currency, desc); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deposited"})
}

func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")

	view, err := h.queries.GetAccountBalance(r.Context(), accountID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.queries.ListAccounts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	if accounts == nil {
		accounts = []projection.AccountView{}
	}
	writeJSON(w, http.StatusOK, accounts)
}

func (h *Handler) GetTransactionHistory(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")

	history, err := h.queries.GetTransactionHistory(r.Context(), accountID, 100, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	if history == nil {
		history = []projection.TransactionHistoryEntry{}
	}
	writeJSON(w, http.StatusOK, history)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
