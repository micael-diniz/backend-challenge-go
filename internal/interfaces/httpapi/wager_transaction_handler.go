package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

type acceptedWagerResponse struct {
	TransactionID    string `json:"transactionId"`
	Status           string `json:"status"`
	IdempotentReplay bool   `json:"idempotentReplay"`
}

type processedWagerResponse struct {
	TransactionID    string       `json:"transactionId"`
	Status           string       `json:"status"`
	Balance          domain.Money `json:"balance"`
	IdempotentReplay bool         `json:"idempotentReplay"`
}

type WagerTransactionHandler struct {
	placeBet *application.PlaceBetService
}

func NewWagerTransactionHandler(placeBet *application.PlaceBetService) *WagerTransactionHandler {
	return &WagerTransactionHandler{placeBet: placeBet}
}

func (handler *WagerTransactionHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.Path != "/wagering/transactions" {
		http.NotFound(response, request)
		return
	}

	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeError(response, http.StatusBadRequest, "idempotency_key_required")
		return
	}

	var body application.WagerMessageData
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_body")
		return
	}
	body.IdempotencyKey = idempotencyKey
	bet, replay, err := handler.placeBet.Apply(
		request.Context(),
		body.Command(domain.CorrelationID(idempotencyKey)),
		domain.WagerTransactionKind(body.Kind),
	)
	if errors.Is(err, application.ErrInvalidWagerMessage) {
		writeError(response, http.StatusBadRequest, "unsupported_kind")
		return
	}
	if err != nil {
		writePlaceBetError(response, err)
		return
	}
	code, hasFailureCode := bet.FailureCode()
	if bet.Status() == domain.WagerTransactionStatusRejected && hasFailureCode {
		switch code {
		case domain.WagerTransactionFailureCodeInsufficientFunds:
			writeError(response, http.StatusUnprocessableEntity, "insufficient_funds")
			return
		case domain.WagerTransactionFailureCodeReversalInsufficientFunds:
			writeError(response, http.StatusUnprocessableEntity, "reversal_insufficient_funds")
			return
		case domain.WagerTransactionFailureCodeDuplicateReversal:
			writeError(response, http.StatusUnprocessableEntity, "duplicate_reversal")
			return
		case domain.WagerTransactionFailureCodeReferenceNotFound:
			writeError(response, http.StatusUnprocessableEntity, "reference_not_found")
			return
		case domain.WagerTransactionFailureCodeInvalidReference:
			writeError(response, http.StatusUnprocessableEntity, "invalid_reference")
			return
		}
	}
	if bet.Status() == domain.WagerTransactionStatusPendingReference {
		writeJSON(response, http.StatusAccepted, acceptedWagerResponse{
			TransactionID:    string(bet.ID()),
			Status:           string(bet.Status()),
			IdempotentReplay: replay,
		})
		return
	}
	result, present := bet.ResultBalance()
	if !present {
		writeError(response, http.StatusInternalServerError, "missing_result_balance")
		return
	}
	writeJSON(response, http.StatusOK, processedWagerResponse{
		TransactionID:    string(bet.ID()),
		Status:           string(bet.Status()),
		Balance:          result,
		IdempotentReplay: replay,
	})
}

func writePlaceBetError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrIdempotencyConflict):
		writeError(response, http.StatusConflict, "idempotency_conflict")
	case errors.Is(err, application.ErrWalletNotFound):
		writeError(response, http.StatusNotFound, "wallet_not_found")
	case errors.Is(err, application.ErrBetPlayerMismatch):
		writeError(response, http.StatusUnprocessableEntity, "bet_player_mismatch")
	case errors.Is(err, domain.ErrInsufficientFunds):
		writeError(response, http.StatusUnprocessableEntity, "insufficient_funds")
	case errors.Is(err, application.ErrReferenceNotFound):
		writeError(response, http.StatusUnprocessableEntity, "reference_not_found")
	case errors.Is(err, application.ErrInvalidReference):
		writeError(response, http.StatusUnprocessableEntity, "invalid_reference")
	case errors.Is(err, application.ErrDuplicateReversal):
		writeError(response, http.StatusUnprocessableEntity, "duplicate_reversal")
	case errors.Is(err, application.ErrReversalInsufficientFunds):
		writeError(response, http.StatusUnprocessableEntity, "reversal_insufficient_funds")
	default:
		writeError(response, http.StatusInternalServerError, "place_bet_failed")
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, errorResponse{Error: code})
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}
