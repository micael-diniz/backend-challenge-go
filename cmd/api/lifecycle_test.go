package main

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"go.uber.org/fx"
)

func TestApplicationStartStopsWorkers(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" || os.Getenv("SQS_ENDPOINT") == "" || os.Getenv("MIGRATIONS_DIR") == "" {
		t.Skip("DATABASE_URL, SQS_ENDPOINT, and MIGRATIONS_DIR are required")
	}
	t.Setenv("HTTP_ADDR", "127.0.0.1:18080")
	t.Setenv("OIDC_ISSUER", "http://127.0.0.1:8081/realms/jungle")
	t.Setenv("WALLET_CLIENT_ID", "wallet-service")

	app := newApp(fx.NopLogger)
	startCtx, cancelStart := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStart()
	if err := app.Start(startCtx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	conn, err := net.DialTimeout("tcp", "127.0.0.1:18080", time.Second)
	if err != nil {
		t.Fatalf("dial started API: %v", err)
	}
	conn.Close()

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStop()
	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:18080", 200*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("listener still open after Stop")
	}
}
