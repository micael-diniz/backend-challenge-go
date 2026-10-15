package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
)

func TestGetWalletReturnsTheCurrentBalance(t *testing.T) {
	inner := newBetHandler(t, "100.00")
	if response := postBet(t, inner, "provider-a:transaction-123", betBody("25.00")); response.Code != http.StatusOK {
		t.Fatalf("bet status = %d body %s", response.Code, response.Body.String())
	}

	forbidden := getAs(t, inner, "/wallets/0192f291-27dd-7d3f-8071-5f8685deef37", "provider-a")
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("provider status = %d body %s; want 403", forbidden.Code, forbidden.Body.String())
	}
	response := getAs(t, inner, "/wallets/0192f291-27dd-7d3f-8071-5f8685deef37", "wallet-service")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body %s; want 200", response.Code, response.Body.String())
	}
	got := decodeWalletResponse(t, response)
	if got.Balance.Amount != "75.00" || got.Version != 2 {
		t.Fatalf("wallet = %+v; want the balance after the bet", got)
	}
	missing := getAs(t, inner, "/wallets/missing-wallet", "wallet-service")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d body %s; want 404", missing.Code, missing.Body.String())
	}
}

func TestGetLedgerPagesInStableOrder(t *testing.T) {
	inner := newBetHandler(t, "100.00")
	if response := postBet(t, inner, "provider-a:transaction-123", betBody("25.00")); response.Code != http.StatusOK {
		t.Fatalf("bet status = %d body %s", response.Code, response.Body.String())
	}

	first := getAs(t, inner, "/wallets/0192f291-27dd-7d3f-8071-5f8685deef37/ledger?limit=1", "wallet-service")
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d body %s; want 200", first.Code, first.Body.String())
	}
	page := decodeLedgerResponse(t, first)
	if len(page.Entries) != 1 || page.Entries[0].Direction != "CREDIT" || page.Entries[0].Amount.Amount != "100.00" || page.NextCursor == "" {
		t.Fatalf("first page = %+v; want the opening credit and a cursor", page)
	}

	second := getAs(t, inner, "/wallets/0192f291-27dd-7d3f-8071-5f8685deef37/ledger?limit=1&cursor="+page.NextCursor, "wallet-service")
	next := decodeLedgerResponse(t, second)
	if second.Code != http.StatusOK || len(next.Entries) != 1 || next.Entries[0].Direction != "DEBIT" || next.Entries[0].BalanceAfter.Amount != "75.00" || next.NextCursor != "" {
		t.Fatalf("second page = %+v body %s; want the bet debit", next, second.Body.String())
	}
	invalid := getAs(t, inner, "/wallets/0192f291-27dd-7d3f-8071-5f8685deef37/ledger?cursor=not-a-cursor", "wallet-service")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("cursor status = %d body %s; want 400", invalid.Code, invalid.Body.String())
	}
}

func TestGetTransactionShowsPendingAndRejection(t *testing.T) {
	pendingHandler := newBetHandler(t, "100.00")
	body := replaceJSONString(t, betBody("40.00"), "kind", "WIN")
	body = replaceJSONString(t, body, "externalTransactionId", "win-123")
	body = replaceJSONString(t, body, "referenceExternalTransactionId", "missing-bet")
	if response := postBet(t, pendingHandler, "provider-a:win-123", body); response.Code != http.StatusAccepted {
		t.Fatalf("win status = %d body %s", response.Code, response.Body.String())
	}
	response := getAs(t, pendingHandler, "/wagering/transactions/0192f298-345e-7e38-af88-e43f851a819d", "provider-a")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body %s; want 200", response.Code, response.Body.String())
	}
	got := decodeTransactionResponse(t, response)
	if got.Status != "PENDING_REFERENCE" || got.FailureCode != "" || got.Balance != nil {
		t.Fatalf("transaction = %+v; want a pending reference without a balance", got)
	}
	foreign := getAs(t, pendingHandler, "/wagering/transactions/0192f298-345e-7e38-af88-e43f851a819d", "provider-b")
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign status = %d body %s; want 404", foreign.Code, foreign.Body.String())
	}
	byExternal := getAs(t, pendingHandler, "/providers/provider-a/wagering/transactions/win-123", "provider-a")
	external := decodeTransactionResponse(t, byExternal)
	if byExternal.Code != http.StatusOK || external.TransactionID != got.TransactionID {
		t.Fatalf("external = %+v status %d body %s", external, byExternal.Code, byExternal.Body.String())
	}

	rejectedHandler := newBetHandler(t, "10.00")
	if response := postBet(t, rejectedHandler, "provider-a:transaction-123", betBody("25.00")); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bet status = %d body %s", response.Code, response.Body.String())
	}
	rejected := getAs(t, rejectedHandler, "/providers/provider-a/wagering/transactions/transaction-123", "provider-a")
	rejection := decodeTransactionResponse(t, rejected)
	if rejected.Code != http.StatusOK || rejection.Status != "REJECTED" || rejection.FailureCode != "INSUFFICIENT_FUNDS" {
		t.Fatalf("rejected = %+v status %d body %s", rejection, rejected.Code, rejected.Body.String())
	}
}

func getAs(t *testing.T, handler http.Handler, path string, clientID string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	httpapi.NewAuthenticator(stubTokens{clientID: clientID}, "wallet-service").Wrap(handler).ServeHTTP(recorder, request)
	return recorder
}

type ledgerHTTPResponse struct {
	Entries    []ledgerEntryHTTP `json:"entries"`
	NextCursor string            `json:"nextCursor"`
}

type ledgerEntryHTTP struct {
	Direction    string        `json:"direction"`
	Amount       moneyResponse `json:"amount"`
	BalanceAfter moneyResponse `json:"balanceAfter"`
}

type transactionHTTPResponse struct {
	TransactionID string         `json:"transactionId"`
	Status        string         `json:"status"`
	FailureCode   string         `json:"failureCode"`
	Balance       *moneyResponse `json:"balance"`
}

func decodeLedgerResponse(t *testing.T, response *httptest.ResponseRecorder) ledgerHTTPResponse {
	t.Helper()

	var got ledgerHTTPResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode ledger: %v body %s", err, response.Body.String())
	}
	return got
}

func decodeTransactionResponse(t *testing.T, response *httptest.ResponseRecorder) transactionHTTPResponse {
	t.Helper()

	var got transactionHTTPResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode transaction: %v body %s", err, response.Body.String())
	}
	return got
}
