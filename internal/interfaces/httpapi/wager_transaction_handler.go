package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

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

	var body struct {
		ProviderID                     string       `json:"providerId"`
		ExternalTransactionID          string       `json:"externalTransactionId"`
		PlayerID                       string       `json:"playerId"`
		WalletID                       string       `json:"walletId"`
		RoundID                        string       `json:"roundId"`
		GameID                         string       `json:"gameId"`
		Kind                           string       `json:"kind"`
		Money                          domain.Money `json:"money"`
		ReferenceExternalTransactionID string       `json:"referenceExternalTransactionId"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_body")
		return
	}
	command := application.PlaceBetCommand{
		WalletID:                       domain.WalletID(body.WalletID),
		PlayerID:                       domain.PlayerID(body.PlayerID),
		ProviderID:                     domain.ProviderID(body.ProviderID),
		ExternalTransactionID:          domain.ExternalTransactionID(body.ExternalTransactionID),
		IdempotencyKey:                 domain.IdempotencyKey(idempotencyKey),
		RoundID:                        domain.RoundID(body.RoundID),
		GameID:                         domain.GameID(body.GameID),
		Money:                          body.Money,
		ReferenceExternalTransactionID: domain.ExternalTransactionID(body.ReferenceExternalTransactionID),
		CorrelationID:                  domain.CorrelationID(idempotencyKey),
	}
	var bet domain.WagerTransaction
	var replay bool
	var err error
	switch domain.WagerTransactionKind(body.Kind) {
	case domain.WagerTransactionKindBet:
		_, bet, replay, err = handler.placeBet.PlaceBet(request.Context(), command)
	case domain.WagerTransactionKindLoss:
		_, bet, replay, err = handler.placeBet.RecordLoss(request.Context(), command)
	case domain.WagerTransactionKindWin:
		_, bet, replay, err = handler.placeBet.RecordWin(request.Context(), command)
	case domain.WagerTransactionKindRefund:
		_, bet, replay, err = handler.placeBet.RecordRefund(request.Context(), command)
	case domain.WagerTransactionKindRollback:
		_, bet, replay, err = handler.placeBet.RecordRollback(request.Context(), command)
	default:
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
		}
	}
	result, present := bet.ResultBalance()
	if !present {
		writeError(response, http.StatusInternalServerError, "missing_result_balance")
		return
	}
	writeJSON(response, http.StatusOK, struct {
		TransactionID    string       `json:"transactionId"`
		Status           string       `json:"status"`
		Balance          domain.Money `json:"balance"`
		IdempotentReplay bool         `json:"idempotentReplay"`
	}{
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

func writeError(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, map[string]string{"error": code})
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}
