//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/micael-diniz/backend-challenge-go/internal/application"
	"github.com/micael-diniz/backend-challenge-go/internal/domain"
	"github.com/micael-diniz/backend-challenge-go/internal/infrastructure/postgres"
)

func TestThreeProcessesKeepOneDebitWhenTwoBetsRace(t *testing.T) {
	if os.Getenv("PLACE_BET_PROCESS") == "1" {
		runBetProcess(t)
		return
	}

	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)
	openedAt := time.Now().Add(-time.Minute).UTC()
	store := postgres.NewWalletOpeningStore(connection)
	playerID := domain.PlayerID(uuid.NewString())
	wallet, err := mustPostgresOpenWalletService(t, store, uuid.NewString, func() time.Time { return openedAt }).OpenWallet(
		ctx,
		postgresOpenWalletCommand(t, playerID, "100.00"),
	)
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}

	startAt := time.Now().Add(200 * time.Millisecond).UTC().Format(time.RFC3339Nano)
	first := startBetProcess(t, wallet.ID(), playerID, "external-80-a", startAt)
	second := startBetProcess(t, wallet.ID(), playerID, "external-80-b", startAt)
	processed, rejected := classifyBetResults(t, waitBetProcess(t, first), waitBetProcess(t, second))
	if processed == "" || rejected == "" {
		t.Fatalf("want one processed bet and one insufficient-funds rejection")
	}

	externalID := strings.TrimPrefix(processed, "provider-a:")
	_, stored, replayed, err := mustPostgresPlaceBetService(t, store, uuid.NewString, time.Now).PlaceBet(ctx, application.PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              playerID,
		ProviderID:            "provider-a",
		ExternalTransactionID: domain.ExternalTransactionID(externalID),
		IdempotencyKey:        domain.IdempotencyKey(processed),
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 mustTestMoney(t, "80.00"),
		CorrelationID:         domain.CorrelationID(processed),
	})
	if err != nil {
		t.Fatalf("replay PlaceBet() returned an unexpected error: %v", err)
	}
	if !replayed {
		t.Fatal("resend was not a replay")
	}
	result, present := stored.ResultBalance()
	if !present || result.Amount() != "20.00" {
		t.Fatalf("replay balance = (%s, %t); want 20.00", result.Amount(), present)
	}

	rejectedExternalID := strings.TrimPrefix(rejected, "provider-a:")
	_, rejectedBet, rejectedReplay, err := mustPostgresPlaceBetService(t, store, uuid.NewString, time.Now).PlaceBet(ctx, application.PlaceBetCommand{
		WalletID:              wallet.ID(),
		PlayerID:              playerID,
		ProviderID:            "provider-a",
		ExternalTransactionID: domain.ExternalTransactionID(rejectedExternalID),
		IdempotencyKey:        domain.IdempotencyKey(rejected),
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 mustTestMoney(t, "80.00"),
		CorrelationID:         domain.CorrelationID(rejected),
	})
	if err != nil {
		t.Fatalf("rejected resend error = %v", err)
	}
	if !rejectedReplay || rejectedBet.Status() != domain.WagerTransactionStatusRejected {
		t.Fatalf("rejected resend = replay %t status %s", rejectedReplay, rejectedBet.Status())
	}
	assertOneDebit(t, ctx, connection)
}

func TestSeparateWalletsAcceptConcurrentBets(t *testing.T) {
	if os.Getenv("PLACE_BET_PROCESS") == "1" {
		runBetProcess(t)
		return
	}

	ctx, connection := openTestDatabase(t)
	prepareBetPersistenceSchema(t, ctx, connection)
	openedAt := time.Now().Add(-time.Minute).UTC()
	store := postgres.NewWalletOpeningStore(connection)
	openWallet := mustPostgresOpenWalletService(t, store, uuid.NewString, func() time.Time { return openedAt })
	firstWallet, err := openWallet.OpenWallet(ctx, postgresOpenWalletCommand(t, domain.PlayerID(uuid.NewString()), "100.00"))
	if err != nil {
		t.Fatalf("OpenWallet() returned an unexpected error: %v", err)
	}
	secondWallet, err := openWallet.OpenWallet(ctx, postgresOpenWalletCommand(t, domain.PlayerID(uuid.NewString()), "100.00"))
	if err != nil {
		t.Fatalf("second OpenWallet() returned an unexpected error: %v", err)
	}

	startAt := time.Now().Add(200 * time.Millisecond).UTC().Format(time.RFC3339Nano)
	first := startBetProcess(t, firstWallet.ID(), firstWallet.PlayerID(), "external-wallet-a", startAt)
	second := startBetProcess(t, secondWallet.ID(), secondWallet.PlayerID(), "external-wallet-b", startAt)
	firstProcessed, firstRejected := classifyBetResults(t, waitBetProcess(t, first))
	secondProcessed, secondRejected := classifyBetResults(t, waitBetProcess(t, second))
	if firstProcessed == "" || secondProcessed == "" || firstRejected != "" || secondRejected != "" {
		t.Fatalf("separate wallets = (%q, %q) and (%q, %q); want both processed", firstProcessed, firstRejected, secondProcessed, secondRejected)
	}

	var balances int64
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wallets WHERE balance_minor = 2000 AND version = 2").Scan(&balances); err != nil {
		t.Fatalf("count wallets: %v", err)
	}
	var debits int64
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wallet_ledger_entries WHERE direction = 'DEBIT'").Scan(&debits); err != nil {
		t.Fatalf("count debits: %v", err)
	}
	if balances != 2 || debits != 2 {
		t.Fatalf("wallets at 20.00 = %d debits = %d; want 2 and 2", balances, debits)
	}
}

func runBetProcess(t *testing.T) {
	t.Helper()
	if start := os.Getenv("BET_START_AT"); start != "" {
		deadline, err := time.Parse(time.RFC3339Nano, start)
		if err != nil {
			t.Fatalf("BET_START_AT = %q: %v", start, err)
		}
		time.Sleep(time.Until(deadline))
	}

	ctx, connection := openTestDatabase(t)
	externalID := os.Getenv("BET_EXTERNAL_ID")
	command := application.PlaceBetCommand{
		WalletID:              domain.WalletID(os.Getenv("BET_WALLET_ID")),
		PlayerID:              domain.PlayerID(os.Getenv("BET_PLAYER_ID")),
		ProviderID:            "provider-a",
		ExternalTransactionID: domain.ExternalTransactionID(externalID),
		IdempotencyKey:        domain.IdempotencyKey("provider-a:" + externalID),
		RoundID:               "round-1",
		GameID:                "game-1",
		Money:                 mustTestMoney(t, "80.00"),
		CorrelationID:         domain.CorrelationID("provider-a:" + externalID),
	}
	_, _, _, err := mustPostgresPlaceBetService(
		t,
		postgres.NewWalletOpeningStore(connection),
		uuid.NewString,
		time.Now,
	).PlaceBet(ctx, command)
	switch {
	case err == nil:
		fmt.Printf("RESULT processed provider-a:%s\n", externalID)
	case errors.Is(err, domain.ErrInsufficientFunds):
		fmt.Printf("RESULT insufficient_funds provider-a:%s\n", externalID)
	default:
		t.Fatalf("PlaceBet() error = %v", err)
	}
}

func startBetProcess(
	t *testing.T,
	walletID domain.WalletID,
	playerID domain.PlayerID,
	externalID string,
	startAt string,
) *exec.Cmd {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=TestThreeProcessesKeepOneDebitWhenTwoBetsRace")
	command.Env = append(os.Environ(),
		"PLACE_BET_PROCESS=1",
		"BET_WALLET_ID="+string(walletID),
		"BET_PLAYER_ID="+string(playerID),
		"BET_EXTERNAL_ID="+externalID,
		"BET_START_AT="+startAt,
	)
	command.Stdout = &bytes.Buffer{}
	command.Stderr = command.Stdout
	if err := command.Start(); err != nil {
		t.Fatalf("start bet process: %v", err)
	}
	return command
}

func waitBetProcess(t *testing.T, command *exec.Cmd) string {
	t.Helper()
	err := command.Wait()
	output := command.Stdout.(*bytes.Buffer).String()
	if err != nil {
		t.Fatalf("bet process failed: %v\n%s", err, output)
	}
	return output
}

func classifyBetResults(t *testing.T, outputs ...string) (processed string, rejected string) {
	t.Helper()
	for _, output := range outputs {
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) != 3 || fields[0] != "RESULT" {
				continue
			}
			switch fields[1] {
			case "processed":
				processed = fields[2]
			case "insufficient_funds":
				rejected = fields[2]
			default:
				t.Fatalf("unknown bet result %q", line)
			}
		}
	}
	return processed, rejected
}

func mustTestMoney(t *testing.T, amount string) domain.Money {
	t.Helper()
	money, err := domain.NewMoney(amount, "BRL")
	if err != nil {
		t.Fatalf("NewMoney() returned an unexpected error: %v", err)
	}
	return money
}

func assertOneDebit(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	var balanceMinor, version, processed, rejected, debits int64
	if err := connection.QueryRow(ctx, "SELECT balance_minor, version FROM wallets").Scan(&balanceMinor, &version); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wager_transactions WHERE kind = 'BET' AND status = 'PROCESSED'").Scan(&processed); err != nil {
		t.Fatalf("count processed bets: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wager_transactions WHERE kind = 'BET' AND status = 'REJECTED' AND failure_code = 'INSUFFICIENT_FUNDS'").Scan(&rejected); err != nil {
		t.Fatalf("count rejected bets: %v", err)
	}
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM wallet_ledger_entries WHERE direction = 'DEBIT'").Scan(&debits); err != nil {
		t.Fatalf("count debits: %v", err)
	}
	var rejectionEvents int64
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE event_type = 'WagerTransactionRejected'").Scan(&rejectionEvents); err != nil {
		t.Fatalf("count rejection events: %v", err)
	}
	if balanceMinor != 2000 || version != 2 || processed != 1 || rejected != 1 || debits != 1 || rejectionEvents != 1 {
		t.Fatalf("state = balance %d version %d processed %d rejected %d debits %d events %d; want 2000, 2, 1, 1, 1, 1", balanceMinor, version, processed, rejected, debits, rejectionEvents)
	}
}
