//go:build integration

package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/oidcauth"
	"github.com/micael-diniz/backend-challenge-go/internal/interfaces/httpapi"
)

func TestKeycloakTokenAuthorizesItsProvider(t *testing.T) {
	issuer := "http://127.0.0.1:8081/realms/jungle"
	token := keycloakToken(t, issuer, "provider-a", "provider-a-secret")
	verifier := oidcauth.NewVerifier(issuer)
	clientID, err := verifier.ClientID(context.Background(), token)
	if err != nil {
		t.Fatalf("ClientID() error = %v", err)
	}
	if clientID != "provider-a" {
		t.Fatalf("client = %q; want provider-a", clientID)
	}

	next := &recordingHandler{}
	handler := httpapi.NewAuthenticator(verifier, "wallet-service").Wrap(next)
	request := httptest.NewRequest(http.MethodPost, "/wagering/transactions", strings.NewReader(betBody("25.00")))
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || next.calls != 1 {
		t.Fatalf("status = %d calls = %d body %s; want 200 and one call", recorder.Code, next.calls, recorder.Body.String())
	}
}

func keycloakToken(t *testing.T, issuer string, clientID string, secret string) string {
	t.Helper()

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", secret)
	deadline := time.Now().Add(90 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		response, err := http.Post(
			issuer+"/protocol/openid-connect/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		if err != nil {
			last = err
			time.Sleep(2 * time.Second)
			continue
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			last = fmt.Errorf("status %d body %s", response.StatusCode, body)
			time.Sleep(2 * time.Second)
			continue
		}
		var payload struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.AccessToken == "" {
			t.Fatalf("token response = %s", body)
		}
		return payload.AccessToken
	}
	t.Fatalf("keycloak token: %v", last)
	return ""
}
