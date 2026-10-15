package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/memory"
	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
)

func TestPostWalletOpensPositiveBalance(t *testing.T) {
	handler := newWalletHandler(t)

	response := postWallet(t, handler, walletBody("1000.00"))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body %s; want 200", response.Code, response.Body.String())
	}
	got := decodeWalletResponse(t, response)
	if got.ID != "0192f291-27dd-7d3f-8071-5f8685deef37" || got.PlayerID != "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1" || got.Version != 1 {
		t.Errorf("response = %+v; want the opened wallet at version 1", got)
	}
	if got.Balance.Amount != "1000.00" || got.Balance.Currency != "BRL" {
		t.Errorf("balance = %+v; want 1000.00 BRL", got.Balance)
	}
}

func TestPostWalletOpensZeroBalance(t *testing.T) {
	handler := newWalletHandler(t)

	response := postWallet(t, handler, walletBody("0.00"))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body %s; want 200", response.Code, response.Body.String())
	}
	got := decodeWalletResponse(t, response)
	if got.Balance.Amount != "0.00" || got.Version != 1 {
		t.Errorf("response = %+v; want zero balance at version 1", got)
	}
}

func TestPostWalletRejectsDuplicatePlayerCurrency(t *testing.T) {
	handler := newWalletHandler(t)

	if response := postWallet(t, handler, walletBody("1000.00")); response.Code != http.StatusOK {
		t.Fatalf("first status = %d body %s; want 200", response.Code, response.Body.String())
	}
	response := postWallet(t, handler, walletBody("1000.00"))
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d body %s; want 409", response.Code, response.Body.String())
	}
}

func TestPostWalletRejectsBlankPlayer(t *testing.T) {
	handler := newWalletHandler(t)
	body := strings.Replace(walletBody("1000.00"), `"playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1"`, `"playerId": " "`, 1)

	response := postWallet(t, handler, body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body %s; want 400", response.Code, response.Body.String())
	}
}

func TestPostWalletRejectsInvalidBody(t *testing.T) {
	handler := newWalletHandler(t)

	response := postWallet(t, handler, "{")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body %s; want 400", response.Code, response.Body.String())
	}
}

func newWalletHandler(t *testing.T) http.Handler {
	t.Helper()

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	store := memory.NewWalletOpeningStore()
	ids := []string{
		"correlation-1",
		"0192f291-27dd-7d3f-8071-5f8685deef37",
		"opening-1", "ledger-1", "event-1", "event-2",
	}
	var sequence int
	newID := func() string {
		sequence++
		if sequence <= len(ids) {
			return ids[sequence-1]
		}
		return fmt.Sprintf("id-%d", sequence)
	}
	service, err := application.NewOpenWalletService(store, newID, func() time.Time { return openedAt })
	if err != nil {
		t.Fatalf("NewOpenWalletService() returned an unexpected error: %v", err)
	}
	return httpapi.NewWalletHandler(service, newID)
}

func postWallet(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func walletBody(amount string) string {
	return `{
		"playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
		"initialBalance": { "amount": "` + amount + `", "currency": "BRL" }
	}`
}

type walletHTTPResponse struct {
	ID       string        `json:"id"`
	PlayerID string        `json:"playerId"`
	Balance  moneyResponse `json:"balance"`
	Version  int64         `json:"version"`
}

func decodeWalletResponse(t *testing.T, response *httptest.ResponseRecorder) walletHTTPResponse {
	t.Helper()

	var got walletHTTPResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v body %s", err, response.Body.String())
	}
	return got
}
