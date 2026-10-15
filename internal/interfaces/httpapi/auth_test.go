package httpapi_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
)

func TestProtectedRouteRejectsMissingToken(t *testing.T) {
	next := &recordingHandler{}
	handler := httpapi.NewAuthenticator(stubTokens{clientID: "provider-a"}, "wallet-service").Wrap(next)

	response := postWallet(t, handler, walletBody("1000.00"))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body %s; want 401", response.Code, response.Body.String())
	}
	if next.calls != 0 {
		t.Fatalf("handler calls = %d; want 0", next.calls)
	}
}

func TestProtectedBetRequiresTheAuthenticatedProvider(t *testing.T) {
	next := &recordingHandler{}
	handler := httpapi.NewAuthenticator(stubTokens{clientID: "provider-a"}, "wallet-service").Wrap(next)

	foreign := withToken(t, handler, "/wagering/transactions", replaceJSONString(t, betBody("25.00"), "providerId", "provider-b"))
	if foreign.Code != http.StatusForbidden || next.calls != 0 {
		t.Fatalf("foreign status = %d calls = %d; want 403 and no call", foreign.Code, next.calls)
	}

	matched := withToken(t, handler, "/wagering/transactions", betBody("25.00"))
	if matched.Code != http.StatusOK || next.calls != 1 {
		t.Fatalf("status = %d calls = %d; want 200 and one call", matched.Code, next.calls)
	}

	internal := httpapi.NewAuthenticator(stubTokens{clientID: "wallet-service"}, "wallet-service").Wrap(next)
	rejected := withToken(t, internal, "/wagering/transactions", replaceJSONString(t, betBody("25.00"), "providerId", "wallet-service"))
	if rejected.Code != http.StatusForbidden || next.calls != 1 {
		t.Fatalf("internal client status = %d calls = %d; want 403 and no new call", rejected.Code, next.calls)
	}
}

func TestProtectedWalletRequiresTheInternalClient(t *testing.T) {
	next := &recordingHandler{}
	provider := httpapi.NewAuthenticator(stubTokens{clientID: "provider-a"}, "wallet-service").Wrap(next)
	rejected := withToken(t, provider, "/wallets", walletBody("1000.00"))
	if rejected.Code != http.StatusForbidden || next.calls != 0 {
		t.Fatalf("provider status = %d calls = %d; want 403 and no call", rejected.Code, next.calls)
	}

	internal := httpapi.NewAuthenticator(stubTokens{clientID: "wallet-service"}, "wallet-service").Wrap(next)
	accepted := withToken(t, internal, "/wallets", walletBody("1000.00"))
	if accepted.Code != http.StatusOK || next.calls != 1 {
		t.Fatalf("status = %d calls = %d; want 200 and one call", accepted.Code, next.calls)
	}
}

type stubTokens struct {
	clientID string
}

func (tokens stubTokens) ClientID(context.Context, string) (string, error) {
	return tokens.clientID, nil
}

type recordingHandler struct {
	calls int
}

func (handler *recordingHandler) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	handler.calls++
	response.WriteHeader(http.StatusOK)
}

func withToken(t *testing.T, handler http.Handler, path string, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
