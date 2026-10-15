package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

type contextKey int

const clientIDContextKey contextKey = 1

func contextWithClientID(ctx context.Context, clientID string) context.Context {
	return context.WithValue(ctx, clientIDContextKey, clientID)
}

func clientIDFromContext(ctx context.Context) (string, bool) {
	clientID, ok := ctx.Value(clientIDContextKey).(string)
	return clientID, ok && clientID != ""
}

type ReadHandler struct {
	reads          *application.ReadService
	walletClientID string
}

func NewReadHandler(reads *application.ReadService, walletClientID string) *ReadHandler {
	return &ReadHandler{reads: reads, walletClientID: walletClientID}
}

func (handler *ReadHandler) Wallet(response http.ResponseWriter, request *http.Request) {
	wallet, err := handler.reads.Wallet(request.Context(), domain.WalletID(request.PathValue("walletId")))
	if err != nil {
		writeReadError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, walletJSON(wallet))
}

func (handler *ReadHandler) Ledger(response http.ResponseWriter, request *http.Request) {
	limit, err := ledgerLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_limit")
		return
	}
	page, err := handler.reads.Ledger(
		request.Context(),
		domain.WalletID(request.PathValue("walletId")),
		request.URL.Query().Get("cursor"),
		limit,
	)
	if err != nil {
		writeReadError(response, err)
		return
	}
	entries := make([]ledgerEntryResponse, 0, len(page.Entries))
	for _, entry := range page.Entries {
		entries = append(entries, ledgerEntryResponse{
			ID:            string(entry.ID()),
			TransactionID: string(entry.TransactionID()),
			Direction:     string(entry.Direction()),
			Amount:        entry.Money(),
			BalanceBefore: entry.BalanceBefore(),
			BalanceAfter:  entry.BalanceAfter(),
			CreatedAt:     entry.CreatedAt().UTC(),
		})
	}
	writeJSON(response, http.StatusOK, ledgerPageResponse{
		Entries:    entries,
		NextCursor: page.NextCursor,
	})
}

func (handler *ReadHandler) Transaction(response http.ResponseWriter, request *http.Request) {
	clientID, ok := clientIDFromContext(request.Context())
	if !ok {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}
	transaction, found, err := handler.reads.Transaction(request.Context(), domain.TransactionID(request.PathValue("transactionId")))
	if err != nil {
		writeReadError(response, err)
		return
	}
	if !found || !handler.canReadTransaction(clientID, transaction) {
		writeError(response, http.StatusNotFound, "transaction_not_found")
		return
	}
	writeJSON(response, http.StatusOK, transactionResponse(transaction))
}

func (handler *ReadHandler) ProviderTransaction(response http.ResponseWriter, request *http.Request) {
	clientID, ok := clientIDFromContext(request.Context())
	if !ok {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}
	if clientID != request.PathValue("providerId") {
		writeError(response, http.StatusForbidden, "forbidden")
		return
	}
	transaction, found, err := handler.reads.ProviderTransaction(
		request.Context(),
		domain.ProviderID(request.PathValue("providerId")),
		domain.ExternalTransactionID(request.PathValue("externalTransactionId")),
	)
	if err != nil {
		writeReadError(response, err)
		return
	}
	if !found {
		writeError(response, http.StatusNotFound, "transaction_not_found")
		return
	}
	writeJSON(response, http.StatusOK, transactionResponse(transaction))
}

func (handler *ReadHandler) canReadTransaction(clientID string, transaction domain.WagerTransaction) bool {
	if clientID == handler.walletClientID {
		return true
	}
	return string(transaction.ProviderID()) == clientID
}

func ledgerLimit(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return limit, nil
}

func writeReadError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrWalletNotFound):
		writeError(response, http.StatusNotFound, "wallet_not_found")
	case errors.Is(err, application.ErrInvalidLedgerLimit):
		writeError(response, http.StatusBadRequest, "invalid_limit")
	case errors.Is(err, application.ErrInvalidLedgerCursor):
		writeError(response, http.StatusBadRequest, "invalid_cursor")
	default:
		writeError(response, http.StatusInternalServerError, "read_failed")
	}
}

type ledgerPageResponse struct {
	Entries    []ledgerEntryResponse `json:"entries"`
	NextCursor string                `json:"nextCursor,omitempty"`
}

type walletResponse struct {
	ID       string       `json:"id"`
	PlayerID string       `json:"playerId"`
	Balance  domain.Money `json:"balance"`
	Version  int64        `json:"version"`
}

func walletJSON(wallet domain.Wallet) walletResponse {
	return walletResponse{
		ID:       string(wallet.ID()),
		PlayerID: string(wallet.PlayerID()),
		Balance:  wallet.Balance(),
		Version:  wallet.Version(),
	}
}

type ledgerEntryResponse struct {
	ID            string       `json:"id"`
	TransactionID string       `json:"transactionId"`
	Direction     string       `json:"direction"`
	Amount        domain.Money `json:"amount"`
	BalanceBefore domain.Money `json:"balanceBefore"`
	BalanceAfter  domain.Money `json:"balanceAfter"`
	CreatedAt     time.Time    `json:"createdAt"`
}

type transactionResponseBody struct {
	TransactionID                  string        `json:"transactionId"`
	ProviderID                     string        `json:"providerId,omitempty"`
	ExternalTransactionID          string        `json:"externalTransactionId,omitempty"`
	PlayerID                       string        `json:"playerId"`
	WalletID                       string        `json:"walletId"`
	RoundID                        string        `json:"roundId,omitempty"`
	GameID                         string        `json:"gameId,omitempty"`
	Kind                           string        `json:"kind"`
	Status                         string        `json:"status"`
	Money                          domain.Money  `json:"money"`
	ReferenceExternalTransactionID string        `json:"referenceExternalTransactionId,omitempty"`
	FailureCode                    string        `json:"failureCode,omitempty"`
	Balance                        *domain.Money `json:"balance,omitempty"`
}

func transactionResponse(transaction domain.WagerTransaction) transactionResponseBody {
	body := transactionResponseBody{
		TransactionID:         string(transaction.ID()),
		ProviderID:            string(transaction.ProviderID()),
		ExternalTransactionID: string(transaction.ExternalTransactionID()),
		PlayerID:              string(transaction.PlayerID()),
		WalletID:              string(transaction.WalletID()),
		RoundID:               string(transaction.RoundID()),
		GameID:                string(transaction.GameID()),
		Kind:                  string(transaction.Kind()),
		Status:                string(transaction.Status()),
		Money:                 transaction.Money(),
	}
	if reference, present := transaction.ReferenceExternalTransactionID(); present {
		body.ReferenceExternalTransactionID = string(reference)
	}
	if code, present := transaction.FailureCode(); present {
		body.FailureCode = string(code)
	}
	if balance, present := transaction.ResultBalance(); present {
		body.Balance = &balance
	}
	return body
}
