package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/memory"
	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
)

func TestServerRoutesWalletThenBet(t *testing.T) {
	handler := newAPIHandler(t)

	opened := postWallet(t, handler, walletBody("100.00"))
	if opened.Code != http.StatusOK {
		t.Fatalf("open status = %d body %s; want 200", opened.Code, opened.Body.String())
	}
	wallet := decodeWalletResponse(t, opened)

	body := betBody("25.00")
	body = replaceJSONString(t, body, "walletId", wallet.ID)
	played := postBet(t, handler, "provider-a:transaction-123", body)
	if played.Code != http.StatusOK {
		t.Fatalf("bet status = %d body %s; want 200", played.Code, played.Body.String())
	}
	got := decodeBetResponse(t, played)
	if got.Status != "PROCESSED" || got.Balance.Amount != "75.00" || got.IdempotentReplay {
		t.Errorf("bet = %+v; want processed 75.00 without replay", got)
	}
}

func newAPIHandler(t *testing.T) http.Handler {
	t.Helper()

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	store := memory.NewWalletOpeningStore()
	var sequence int
	newID := func() string {
		sequence++
		return fmt.Sprintf("id-%d", sequence)
	}
	openService, err := application.NewOpenWalletService(store, newID, func() time.Time { return openedAt })
	if err != nil {
		t.Fatalf("NewOpenWalletService() returned an unexpected error: %v", err)
	}
	placeService, err := application.NewPlaceBetService(store, newID, func() time.Time { return openedAt })
	if err != nil {
		t.Fatalf("NewPlaceBetService() returned an unexpected error: %v", err)
	}
	return httpapi.NewHandler(
		httpapi.NewWalletHandler(openService, newID),
		httpapi.NewWagerTransactionHandler(placeService),
	)
}

func replaceJSONString(t *testing.T, body string, field string, value string) string {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	payload[field] = value
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}
	return string(encoded)
}
