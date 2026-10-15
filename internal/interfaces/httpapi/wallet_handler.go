package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
)

type WalletHandler struct {
	openWallet *application.OpenWalletService
	newID      func() string
}

func NewWalletHandler(openWallet *application.OpenWalletService, newID func() string) *WalletHandler {
	return &WalletHandler{openWallet: openWallet, newID: newID}
}

func (handler *WalletHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.Path != "/wallets" {
		http.NotFound(response, request)
		return
	}

	var body struct {
		PlayerID       string       `json:"playerId"`
		InitialBalance domain.Money `json:"initialBalance"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_body")
		return
	}

	wallet, err := handler.openWallet.OpenWallet(request.Context(), application.OpenWalletCommand{
		PlayerID:       domain.PlayerID(body.PlayerID),
		InitialBalance: body.InitialBalance,
		CorrelationID:  domain.CorrelationID(handler.newID()),
	})
	if err != nil {
		writeOpenWalletError(response, err)
		return
	}

	writeJSON(response, http.StatusOK, struct {
		ID       string       `json:"id"`
		PlayerID string       `json:"playerId"`
		Balance  domain.Money `json:"balance"`
		Version  int64        `json:"version"`
	}{
		ID:       string(wallet.ID()),
		PlayerID: string(wallet.PlayerID()),
		Balance:  wallet.Balance(),
		Version:  wallet.Version(),
	})
}

func writeOpenWalletError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrWalletAlreadyExists):
		writeError(response, http.StatusConflict, "wallet_already_exists")
	case errors.Is(err, domain.ErrInvalidPlayerID),
		errors.Is(err, domain.ErrInvalidMoney),
		errors.Is(err, domain.ErrNegativeWalletBalance):
		writeError(response, http.StatusBadRequest, "invalid_wallet")
	default:
		writeError(response, http.StatusInternalServerError, "open_wallet_failed")
	}
}
