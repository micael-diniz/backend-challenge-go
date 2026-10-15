package httpapi_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"testing"

	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
)

func TestWagerLogKeepsTheAmountOutOfTheLog(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	handler := newBetHandler(t, "100.00")
	response := postBet(t, handler, "provider-a:transaction-123", betBody("25.00"))
	if response.Code != http.StatusOK {
		t.Fatalf("bet status = %d body %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(logs.Bytes(), []byte(`"walletId":"0192f291-27dd-7d3f-8071-5f8685deef37"`)) ||
		!bytes.Contains(logs.Bytes(), []byte(`"status":"PROCESSED"`)) ||
		bytes.Contains(logs.Bytes(), []byte("25.00")) || bytes.Contains(logs.Bytes(), []byte("75.00")) {
		t.Fatalf("log = %s; want the wallet id and status without the amount", logs.String())
	}
	missing := getNoAuth(t, httpapi.NewAuthenticator(stubTokens{clientID: "wallet-service"}, "wallet-service").Wrap(handler), "/metrics")
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("metrics without a token = %d; want 401", missing.Code)
	}
	forbidden := getAs(t, handler, "/metrics", "provider-a")
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("provider metrics = %d; want 403", forbidden.Code)
	}
	metrics := getAs(t, handler, "/metrics", "wallet-service")
	if metrics.Code != http.StatusOK || !bytes.Contains(metrics.Body.Bytes(), []byte("wager_results")) {
		t.Fatalf("metrics = %d %s; want the wager result counter", metrics.Code, metrics.Body.String())
	}
}
