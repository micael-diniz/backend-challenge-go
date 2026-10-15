package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/memory"
	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
)

func TestPostWagerTransactionProcessesBet(t *testing.T) {
	handler := newBetHandler(t, "100.00")

	response := postBet(t, handler, "provider-a:transaction-123", betBody("25.00"))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body %s; want 200", response.Code, response.Body.String())
	}
	got := decodeBetResponse(t, response)
	if got.TransactionID == "" || got.Status != "PROCESSED" || got.IdempotentReplay {
		t.Errorf("response = %+v; want processed bet without replay", got)
	}
	if got.Balance.Amount != "75.00" || got.Balance.Currency != "BRL" {
		t.Errorf("balance = %+v; want 75.00 BRL", got.Balance)
	}
}

func TestPostWagerTransactionReplaysOriginalBalance(t *testing.T) {
	handler := newBetHandler(t, "100.00")
	body := betBody("25.00")

	first := postBet(t, handler, "provider-a:transaction-123", body)
	second := postBet(t, handler, "provider-a:transaction-123", body)
	if second.Code != http.StatusOK {
		t.Fatalf("replay status = %d body %s; want 200", second.Code, second.Body.String())
	}
	got := decodeBetResponse(t, second)
	firstBody := decodeBetResponse(t, first)
	if !got.IdempotentReplay || got.TransactionID != firstBody.TransactionID || got.Balance.Amount != "75.00" {
		t.Errorf("replay = %+v; want original transaction and 75.00", got)
	}
}

func TestPostWagerTransactionRejectsChangedContent(t *testing.T) {
	handler := newBetHandler(t, "100.00")

	if response := postBet(t, handler, "provider-a:transaction-123", betBody("25.00")); response.Code != http.StatusOK {
		t.Fatalf("first status = %d; want 200", response.Code)
	}
	response := postBet(t, handler, "provider-a:transaction-123", betBody("10.00"))
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d body %s; want 409", response.Code, response.Body.String())
	}
}

func TestPostWagerTransactionRejectsInvalidBody(t *testing.T) {
	handler := newBetHandler(t, "100.00")

	response := postBet(t, handler, "provider-a:transaction-123", "{")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body %s; want 400", response.Code, response.Body.String())
	}
}

func TestPostWagerTransactionRequiresIdempotencyKey(t *testing.T) {
	handler := newBetHandler(t, "100.00")

	response := postBet(t, handler, "", betBody("25.00"))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body %s; want 400", response.Code, response.Body.String())
	}
}

func TestPostWagerTransactionRecordsLossWithoutChangingBalance(t *testing.T) {
	handler := newBetHandler(t, "100.00")
	body := strings.Replace(betBody("0.00"), `"kind": "BET"`, `"kind": "LOSS"`, 1)

	response := postBet(t, handler, "provider-a:transaction-123", body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body %s; want 200", response.Code, response.Body.String())
	}
	got := decodeBetResponse(t, response)
	if got.Status != "PROCESSED" || got.IdempotentReplay || got.Balance.Amount != "100.00" {
		t.Errorf("response = %+v; want processed loss at 100.00", got)
	}

	replay := postBet(t, handler, "provider-a:transaction-123", body)
	replayed := decodeBetResponse(t, replay)
	if replay.Code != http.StatusOK || !replayed.IdempotentReplay || replayed.Balance.Amount != "100.00" || replayed.TransactionID != got.TransactionID {
		t.Errorf("replay = %d %+v; want the original 100.00 loss", replay.Code, replayed)
	}
}

func TestPostWagerTransactionCreditsWin(t *testing.T) {
	handler := newBetHandler(t, "100.00")
	body := strings.Replace(betBody("40.00"), `"kind": "BET"`, `"kind": "WIN"`, 1)
	body = strings.Replace(body, `"externalTransactionId": "transaction-123"`, `"externalTransactionId": "win-123"`, 1)

	response := postBet(t, handler, "provider-a:win-123", body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body %s; want 200", response.Code, response.Body.String())
	}
	got := decodeBetResponse(t, response)
	if got.Status != "PROCESSED" || got.IdempotentReplay || got.Balance.Amount != "140.00" {
		t.Errorf("response = %+v; want processed win at 140.00", got)
	}
}

func TestPostWagerTransactionCreditsWinForProcessedBet(t *testing.T) {
	handler := newBetHandler(t, "100.00")
	if response := postBet(t, handler, "provider-a:transaction-123", betBody("25.00")); response.Code != http.StatusOK {
		t.Fatalf("bet status = %d body %s; want 200", response.Code, response.Body.String())
	}
	body := strings.Replace(betBody("40.00"), `"kind": "BET"`, `"kind": "WIN"`, 1)
	body = strings.Replace(body, `"externalTransactionId": "transaction-123"`, `"externalTransactionId": "win-123"`, 1)
	body = strings.TrimSuffix(strings.TrimSpace(body), "}")
	body += `,
		"referenceExternalTransactionId": "transaction-123"
	}`

	response := postBet(t, handler, "provider-a:win-123", body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body %s; want 200", response.Code, response.Body.String())
	}
	got := decodeBetResponse(t, response)
	if got.Status != "PROCESSED" || got.Balance.Amount != "115.00" {
		t.Errorf("response = %+v; want 115.00 after the 25.00 bet and 40.00 win", got)
	}
}

func TestPostWagerTransactionRejectsMissingWinReference(t *testing.T) {
	handler := newBetHandler(t, "100.00")
	body := strings.Replace(betBody("40.00"), `"kind": "BET"`, `"kind": "WIN"`, 1)
	body = strings.TrimSuffix(strings.TrimSpace(body), "}")
	body += `,
		"referenceExternalTransactionId": "missing-bet"
	}`

	response := postBet(t, handler, "provider-a:win-123", body)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body %s; want 422", response.Code, response.Body.String())
	}
}

func TestPostWagerTransactionRejectsUnsupportedKind(t *testing.T) {
	handler := newBetHandler(t, "100.00")
	body := strings.Replace(betBody("25.00"), `"kind": "BET"`, `"kind": "REFUND"`, 1)

	response := postBet(t, handler, "provider-a:transaction-123", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body %s; want 400", response.Code, response.Body.String())
	}
}

func TestPostWagerTransactionRejectsUnknownWallet(t *testing.T) {
	handler := newBetHandler(t, "100.00")
	body := strings.Replace(betBody("25.00"), `"walletId": "0192f291-27dd-7d3f-8071-5f8685deef37"`, `"walletId": "missing-wallet"`, 1)

	response := postBet(t, handler, "provider-a:transaction-123", body)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body %s; want 404", response.Code, response.Body.String())
	}
}

func TestPostWagerTransactionRejectsPlayerMismatch(t *testing.T) {
	handler := newBetHandler(t, "100.00")
	body := strings.Replace(betBody("25.00"), `"playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1"`, `"playerId": "other-player"`, 1)

	response := postBet(t, handler, "provider-a:transaction-123", body)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body %s; want 422", response.Code, response.Body.String())
	}
}

func TestPostWagerTransactionRejectsInsufficientFunds(t *testing.T) {
	handler := newBetHandler(t, "10.00")

	response := postBet(t, handler, "provider-a:transaction-123", betBody("25.00"))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body %s; want 422", response.Code, response.Body.String())
	}
	replay := postBet(t, handler, "provider-a:transaction-123", betBody("25.00"))
	if replay.Code != http.StatusUnprocessableEntity {
		t.Fatalf("replay status = %d body %s; want 422", replay.Code, replay.Body.String())
	}
}

func newBetHandler(t *testing.T, openingAmount string) http.Handler {
	t.Helper()

	openedAt := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	store := memory.NewWalletOpeningStore()
	var sequence int
	ids := []string{
		"0192f291-27dd-7d3f-8071-5f8685deef37",
		"opening-1", "ledger-open-1", "event-open-1", "event-open-2",
		"0192f298-345e-7e38-af88-e43f851a819d",
		"ledger-bet-1", "event-bet-1", "event-bet-2",
		"0192f299-345e-7e38-af88-e43f851a819d",
		"ledger-win-1", "event-win-1", "event-win-2",
	}
	newID := func() string {
		if sequence >= len(ids) {
			t.Fatalf("ID generator exhausted")
		}
		id := ids[sequence]
		sequence++
		return id
	}
	openService, err := application.NewOpenWalletService(store, newID, func() time.Time { return openedAt })
	if err != nil {
		t.Fatalf("NewOpenWalletService() returned an unexpected error: %v", err)
	}
	amount, err := domain.NewMoney(openingAmount, "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	_, err = openService.OpenWallet(context.Background(), application.OpenWalletCommand{
		PlayerID:       "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
		InitialBalance: amount,
		CorrelationID:  "correlation-open",
	})
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	placeService, err := application.NewPlaceBetService(store, newID, func() time.Time { return openedAt.Add(time.Second) })
	if err != nil {
		t.Fatalf("NewPlaceBetService() returned an unexpected error: %v", err)
	}
	return httpapi.NewWagerTransactionHandler(placeService)
}

func postBet(t *testing.T, handler http.Handler, idempotencyKey string, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/wagering/transactions", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func betBody(amount string) string {
	return `{
		"providerId": "provider-a",
		"externalTransactionId": "transaction-123",
		"playerId": "0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1",
		"walletId": "0192f291-27dd-7d3f-8071-5f8685deef37",
		"roundId": "round-987",
		"gameId": "fortune-chimp",
		"kind": "BET",
		"money": { "amount": "` + amount + `", "currency": "BRL" }
	}`
}

type betHTTPResponse struct {
	TransactionID    string `json:"transactionId"`
	Status           string `json:"status"`
	Balance          moneyResponse
	IdempotentReplay bool `json:"idempotentReplay"`
}

type moneyResponse struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func decodeBetResponse(t *testing.T, response *httptest.ResponseRecorder) betHTTPResponse {
	t.Helper()

	var got betHTTPResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v body %s", err, response.Body.String())
	}
	return got
}
