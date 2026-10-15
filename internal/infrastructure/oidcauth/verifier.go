package oidcauth

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
)

var errTokenClientAbsent = errors.New("token client is absent")

type tokenClaims struct {
	AuthorizedParty string `json:"azp"`
}

type Verifier struct {
	issuer   string
	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
}

func NewVerifier(issuer string) *Verifier {
	return &Verifier{issuer: issuer}
}

func (verifier *Verifier) ClientID(ctx context.Context, rawToken string) (string, error) {
	tokenVerifier, err := verifier.tokenVerifier(ctx)
	if err != nil {
		return "", err
	}
	token, err := tokenVerifier.Verify(ctx, rawToken)
	if err != nil {
		return "", fmt.Errorf("verify token: %w", err)
	}
	var claims tokenClaims
	if err := token.Claims(&claims); err != nil {
		return "", fmt.Errorf("read token client: %w", err)
	}
	if claims.AuthorizedParty == "" {
		return "", fmt.Errorf("verify token: %w", errTokenClientAbsent)
	}
	return claims.AuthorizedParty, nil
}

func (verifier *Verifier) tokenVerifier(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	verifier.mu.Lock()
	defer verifier.mu.Unlock()
	if verifier.verifier != nil {
		return verifier.verifier, nil
	}

	provider, err := oidc.NewProvider(ctx, verifier.issuer)
	if err != nil {
		return nil, fmt.Errorf("discover issuer: %w", err)
	}
	verifier.verifier = provider.Verifier(&oidc.Config{SkipClientIDCheck: true})
	return verifier.verifier, nil
}
