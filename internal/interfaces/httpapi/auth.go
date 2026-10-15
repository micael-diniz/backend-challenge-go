package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type TokenVerifier interface {
	ClientID(ctx context.Context, rawToken string) (string, error)
}

type Authenticator struct {
	tokens         TokenVerifier
	walletClientID string
}

func NewAuthenticator(tokens TokenVerifier, walletClientID string) *Authenticator {
	return &Authenticator{tokens: tokens, walletClientID: walletClientID}
}

func (authenticator *Authenticator) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		rawToken, ok := bearerToken(request.Header.Get("Authorization"))
		if !ok {
			writeError(response, http.StatusUnauthorized, "unauthorized")
			return
		}
		clientID, err := authenticator.tokens.ClientID(request.Context(), rawToken)
		if err != nil || clientID == "" {
			writeError(response, http.StatusUnauthorized, "unauthorized")
			return
		}

		switch request.URL.Path {
		case "/wallets":
			if clientID != authenticator.walletClientID {
				writeError(response, http.StatusForbidden, "forbidden")
				return
			}
		case "/wagering/transactions":
			if clientID == authenticator.walletClientID {
				writeError(response, http.StatusForbidden, "forbidden")
				return
			}
			providerID, err := peekProviderID(request)
			if err != nil {
				writeError(response, http.StatusBadRequest, "invalid_body")
				return
			}
			if providerID != clientID {
				writeError(response, http.StatusForbidden, "forbidden")
				return
			}
		}

		next.ServeHTTP(response, request)
	})
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func peekProviderID(request *http.Request) (string, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return "", err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	var payload struct {
		ProviderID string `json:"providerId"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	return payload.ProviderID, nil
}
