package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
)

func TestHealthLiveAndReadyArePublic(t *testing.T) {
	inner := httpapi.NewAuthenticator(stubTokens{clientID: "wallet-service"}, "wallet-service").Wrap(newBetHandler(t, "100.00"))

	live := getNoAuth(t, inner, "/health/live")
	if live.Code != http.StatusOK || decodeHealth(t, live).Status != "live" {
		t.Fatalf("live = %d %s; want 200 live", live.Code, live.Body.String())
	}
	ready := getNoAuth(t, inner, "/health/ready")
	if ready.Code != http.StatusOK || decodeHealth(t, ready).Status != "ready" {
		t.Fatalf("ready = %d %s; want 200 ready", ready.Code, ready.Body.String())
	}
	wallet := getNoAuth(t, inner, "/wallets/0192f291-27dd-7d3f-8071-5f8685deef37")
	if wallet.Code != http.StatusUnauthorized {
		t.Fatalf("wallet status = %d; want 401", wallet.Code)
	}
}

func TestHealthReadyReportsWhenADependencyIsDown(t *testing.T) {
	mux := http.NewServeMux()
	health := httpapi.NewHealthHandler(func(context.Context) error {
		return errors.New("down")
	})
	mux.HandleFunc("GET /health/ready", health.Ready)
	handler := httpapi.NewAuthenticator(stubTokens{clientID: "wallet-service"}, "wallet-service").Wrap(mux)

	ready := getNoAuth(t, handler, "/health/ready")
	if ready.Code != http.StatusServiceUnavailable || decodeHealth(t, ready).Status != "not_ready" {
		t.Fatalf("ready = %d %s; want 503 not_ready", ready.Code, ready.Body.String())
	}
}

func getNoAuth(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

type healthHTTPResponse struct {
	Status string `json:"status"`
}

func decodeHealth(t *testing.T, response *httptest.ResponseRecorder) healthHTTPResponse {
	t.Helper()

	var got healthHTTPResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode health: %v body %s", err, response.Body.String())
	}
	return got
}
