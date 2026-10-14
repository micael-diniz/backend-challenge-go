package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewExternalWagerTransaction(t *testing.T) {
	createdAt := time.Date(2026, time.October, 13, 18, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		kind        WagerTransactionKind
		money       Money
		referenceID ExternalTransactionID
		wantRef     bool
	}{
		{
			name:  "bet with positive amount",
			kind:  WagerTransactionKindBet,
			money: mustNewMoney(t, "25.00", "BRL"),
		},
		{
			name:  "win without optional bet reference",
			kind:  WagerTransactionKindWin,
			money: mustNewMoney(t, "40.00", "BRL"),
		},
		{
			name:        "win with optional bet reference",
			kind:        WagerTransactionKindWin,
			money:       mustNewMoney(t, "40.00", "BRL"),
			referenceID: ExternalTransactionID("external-bet-1"),
			wantRef:     true,
		},
		{
			name:  "loss with zero amount",
			kind:  WagerTransactionKindLoss,
			money: mustNewMoney(t, "0.00", "BRL"),
		},
		{
			name:        "refund with required bet reference",
			kind:        WagerTransactionKindRefund,
			money:       mustNewMoney(t, "25.00", "BRL"),
			referenceID: ExternalTransactionID("external-bet-1"),
			wantRef:     true,
		},
		{
			name:        "rollback with required transaction reference",
			kind:        WagerTransactionKindRollback,
			money:       mustNewMoney(t, "25.00", "BRL"),
			referenceID: ExternalTransactionID("external-transaction-1"),
			wantRef:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validExternalWagerTransactionInput(t)
			input.Kind = tt.kind
			input.Money = tt.money
			input.ReferenceExternalTransactionID = tt.referenceID
			input.CreatedAt = createdAt

			got, err := NewExternalWagerTransaction(input)
			if err != nil {
				t.Fatalf("NewExternalWagerTransaction() returned an unexpected error: %v", err)
			}

			if got.ID() != input.ID {
				t.Errorf("ID() = %q; want %q", got.ID(), input.ID)
			}
			if got.ProviderID() != input.ProviderID {
				t.Errorf("ProviderID() = %q; want %q", got.ProviderID(), input.ProviderID)
			}
			if got.ExternalTransactionID() != input.ExternalTransactionID {
				t.Errorf("ExternalTransactionID() = %q; want %q", got.ExternalTransactionID(), input.ExternalTransactionID)
			}
			if got.IdempotencyKey() != input.IdempotencyKey {
				t.Errorf("IdempotencyKey() = %q; want %q", got.IdempotencyKey(), input.IdempotencyKey)
			}
			if got.PayloadHash() != input.PayloadHash {
				t.Errorf("PayloadHash() = %q; want %q", got.PayloadHash(), input.PayloadHash)
			}
			if got.WalletID() != input.WalletID {
				t.Errorf("WalletID() = %q; want %q", got.WalletID(), input.WalletID)
			}
			if got.PlayerID() != input.PlayerID {
				t.Errorf("PlayerID() = %q; want %q", got.PlayerID(), input.PlayerID)
			}
			if got.RoundID() != input.RoundID {
				t.Errorf("RoundID() = %q; want %q", got.RoundID(), input.RoundID)
			}
			if got.GameID() != input.GameID {
				t.Errorf("GameID() = %q; want %q", got.GameID(), input.GameID)
			}
			if got.Kind() != input.Kind {
				t.Errorf("Kind() = %q; want %q", got.Kind(), input.Kind)
			}
			if got.Kind().String() != string(input.Kind) {
				t.Errorf("Kind().String() = %q; want %q", got.Kind().String(), input.Kind)
			}
			assertMoneyState(t, "Money()", got.Money(), input.Money)

			gotReferenceID, gotReference := got.ReferenceExternalTransactionID()
			if gotReference != tt.wantRef {
				t.Errorf("ReferenceExternalTransactionID() present = %t; want %t", gotReference, tt.wantRef)
			}
			if gotReferenceID != tt.referenceID {
				t.Errorf("ReferenceExternalTransactionID() = %q; want %q", gotReferenceID, tt.referenceID)
			}

			if got.Status() != WagerTransactionStatusPending {
				t.Errorf("Status() = %q; want %q", got.Status(), WagerTransactionStatusPending)
			}
			if got.Status().String() != string(WagerTransactionStatusPending) {
				t.Errorf("Status().String() = %q; want %q", got.Status().String(), WagerTransactionStatusPending)
			}
			if !got.CreatedAt().Equal(createdAt) {
				t.Errorf("CreatedAt() = %s; want %s", got.CreatedAt(), createdAt)
			}
			if !got.UpdatedAt().Equal(createdAt) {
				t.Errorf("UpdatedAt() = %s; want %s", got.UpdatedAt(), createdAt)
			}
		})
	}
}

func TestNewExternalWagerTransactionRejectsInvalidRequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*ExternalWagerTransactionInput)
		wantError error
	}{
		{name: "empty transaction ID", change: func(input *ExternalWagerTransactionInput) { input.ID = "" }, wantError: ErrInvalidTransactionID},
		{name: "blank transaction ID", change: func(input *ExternalWagerTransactionInput) { input.ID = " \t" }, wantError: ErrInvalidTransactionID},
		{name: "empty provider ID", change: func(input *ExternalWagerTransactionInput) { input.ProviderID = "" }, wantError: ErrInvalidProviderID},
		{name: "blank provider ID", change: func(input *ExternalWagerTransactionInput) { input.ProviderID = "\n" }, wantError: ErrInvalidProviderID},
		{name: "empty external transaction ID", change: func(input *ExternalWagerTransactionInput) { input.ExternalTransactionID = "" }, wantError: ErrInvalidExternalTransactionID},
		{name: "blank external transaction ID", change: func(input *ExternalWagerTransactionInput) { input.ExternalTransactionID = "  " }, wantError: ErrInvalidExternalTransactionID},
		{name: "empty idempotency key", change: func(input *ExternalWagerTransactionInput) { input.IdempotencyKey = "" }, wantError: ErrInvalidIdempotencyKey},
		{name: "blank idempotency key", change: func(input *ExternalWagerTransactionInput) { input.IdempotencyKey = " \n" }, wantError: ErrInvalidIdempotencyKey},
		{name: "empty payload hash", change: func(input *ExternalWagerTransactionInput) { input.PayloadHash = "" }, wantError: ErrInvalidPayloadHash},
		{name: "blank payload hash", change: func(input *ExternalWagerTransactionInput) { input.PayloadHash = "\t" }, wantError: ErrInvalidPayloadHash},
		{name: "empty wallet ID", change: func(input *ExternalWagerTransactionInput) { input.WalletID = "" }, wantError: ErrInvalidWalletID},
		{name: "blank wallet ID", change: func(input *ExternalWagerTransactionInput) { input.WalletID = " " }, wantError: ErrInvalidWalletID},
		{name: "empty player ID", change: func(input *ExternalWagerTransactionInput) { input.PlayerID = "" }, wantError: ErrInvalidPlayerID},
		{name: "blank player ID", change: func(input *ExternalWagerTransactionInput) { input.PlayerID = "\t" }, wantError: ErrInvalidPlayerID},
		{name: "empty round ID", change: func(input *ExternalWagerTransactionInput) { input.RoundID = "" }, wantError: ErrInvalidRoundID},
		{name: "blank round ID", change: func(input *ExternalWagerTransactionInput) { input.RoundID = "\n" }, wantError: ErrInvalidRoundID},
		{name: "empty game ID", change: func(input *ExternalWagerTransactionInput) { input.GameID = "" }, wantError: ErrInvalidGameID},
		{name: "blank game ID", change: func(input *ExternalWagerTransactionInput) { input.GameID = "  " }, wantError: ErrInvalidGameID},
		{name: "uninitialized money", change: func(input *ExternalWagerTransactionInput) { input.Money = Money{} }, wantError: ErrInvalidMoney},
		{name: "zero creation time", change: func(input *ExternalWagerTransactionInput) { input.CreatedAt = time.Time{} }, wantError: ErrInvalidWagerTransactionTimestamp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validExternalWagerTransactionInput(t)
			tt.change(&input)

			_, err := NewExternalWagerTransaction(input)
			assertWagerTransactionError(t, err, tt.wantError)
		})
	}
}

func TestNewExternalWagerTransactionRejectsInvalidKind(t *testing.T) {
	tests := []struct {
		name string
		kind WagerTransactionKind
	}{
		{name: "empty kind", kind: WagerTransactionKind("")},
		{name: "unknown kind", kind: WagerTransactionKind("BONUS")},
		{name: "lowercase kind", kind: WagerTransactionKind("bet")},
		{name: "internal opening kind", kind: WagerTransactionKindOpening},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validExternalWagerTransactionInput(t)
			input.Kind = tt.kind

			_, err := NewExternalWagerTransaction(input)
			assertWagerTransactionError(t, err, ErrInvalidWagerTransactionKind)
		})
	}
}

func TestNewExternalWagerTransactionRejectsInvalidAmountForKind(t *testing.T) {
	zero := mustNewMoney(t, "0.00", "BRL")
	negative := mustNewMoney(t, "-0.01", "BRL")
	positive := mustNewMoney(t, "0.01", "BRL")

	tests := []struct {
		name  string
		kind  WagerTransactionKind
		money Money
	}{
		{name: "zero bet", kind: WagerTransactionKindBet, money: zero},
		{name: "negative bet", kind: WagerTransactionKindBet, money: negative},
		{name: "zero win", kind: WagerTransactionKindWin, money: zero},
		{name: "negative win", kind: WagerTransactionKindWin, money: negative},
		{name: "positive loss", kind: WagerTransactionKindLoss, money: positive},
		{name: "negative loss", kind: WagerTransactionKindLoss, money: negative},
		{name: "zero refund", kind: WagerTransactionKindRefund, money: zero},
		{name: "negative refund", kind: WagerTransactionKindRefund, money: negative},
		{name: "zero rollback", kind: WagerTransactionKindRollback, money: zero},
		{name: "negative rollback", kind: WagerTransactionKindRollback, money: negative},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validExternalWagerTransactionInput(t)
			input.Kind = tt.kind
			input.Money = tt.money
			if tt.kind == WagerTransactionKindRefund || tt.kind == WagerTransactionKindRollback {
				input.ReferenceExternalTransactionID = "external-bet-1"
			}

			_, err := NewExternalWagerTransaction(input)
			assertWagerTransactionError(t, err, ErrInvalidWagerTransactionAmount)
			assertErrorContains(t, err, "kind="+tt.kind.String(), "amount="+tt.money.formatForError())
		})
	}
}

func TestNewExternalWagerTransactionRejectsInvalidReference(t *testing.T) {
	tests := []struct {
		name        string
		kind        WagerTransactionKind
		referenceID ExternalTransactionID
	}{
		{name: "win with blank optional reference", kind: WagerTransactionKindWin, referenceID: " \t"},
		{name: "refund without reference", kind: WagerTransactionKindRefund},
		{name: "refund with blank reference", kind: WagerTransactionKindRefund, referenceID: " \t"},
		{name: "rollback without reference", kind: WagerTransactionKindRollback},
		{name: "rollback with blank reference", kind: WagerTransactionKindRollback, referenceID: "\n"},
		{name: "bet with unsupported reference", kind: WagerTransactionKindBet, referenceID: "external-bet-1"},
		{name: "loss with unsupported reference", kind: WagerTransactionKindLoss, referenceID: "external-bet-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validExternalWagerTransactionInput(t)
			input.Kind = tt.kind
			input.ReferenceExternalTransactionID = tt.referenceID
			if tt.kind == WagerTransactionKindLoss {
				input.Money = mustNewMoney(t, "0.00", "BRL")
			}

			_, err := NewExternalWagerTransaction(input)
			assertWagerTransactionError(t, err, ErrInvalidWagerTransactionReference)
		})
	}
}

func TestNewOpeningWagerTransaction(t *testing.T) {
	createdAt := time.Date(2026, time.October, 14, 10, 0, 0, 0, time.UTC)
	input := OpeningWagerTransactionInput{
		ID:        TransactionID("opening-transaction-1"),
		WalletID:  WalletID("wallet-1"),
		PlayerID:  PlayerID("player-1"),
		Money:     mustNewMoney(t, "1000.00", "BRL"),
		CreatedAt: createdAt,
	}

	got, err := NewOpeningWagerTransaction(input)
	if err != nil {
		t.Fatalf("NewOpeningWagerTransaction() returned an unexpected error: %v", err)
	}

	if got.ID() != input.ID {
		t.Errorf("ID() = %q; want %q", got.ID(), input.ID)
	}
	if got.WalletID() != input.WalletID {
		t.Errorf("WalletID() = %q; want %q", got.WalletID(), input.WalletID)
	}
	if got.PlayerID() != input.PlayerID {
		t.Errorf("PlayerID() = %q; want %q", got.PlayerID(), input.PlayerID)
	}
	if got.Kind() != WagerTransactionKindOpening {
		t.Errorf("Kind() = %q; want %q", got.Kind(), WagerTransactionKindOpening)
	}
	assertMoneyState(t, "Money()", got.Money(), input.Money)
	if got.Status() != WagerTransactionStatusProcessed {
		t.Errorf("Status() = %q; want %q", got.Status(), WagerTransactionStatusProcessed)
	}
	if !got.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt() = %s; want %s", got.CreatedAt(), createdAt)
	}
	if !got.UpdatedAt().Equal(createdAt) {
		t.Errorf("UpdatedAt() = %s; want %s", got.UpdatedAt(), createdAt)
	}

	if got.ProviderID() != "" {
		t.Errorf("ProviderID() = %q; want empty for internal OPENING", got.ProviderID())
	}
	if got.ExternalTransactionID() != "" {
		t.Errorf("ExternalTransactionID() = %q; want empty for internal OPENING", got.ExternalTransactionID())
	}
	if got.IdempotencyKey() != "" {
		t.Errorf("IdempotencyKey() = %q; want empty for internal OPENING", got.IdempotencyKey())
	}
	if got.PayloadHash() != "" {
		t.Errorf("PayloadHash() = %q; want empty for internal OPENING", got.PayloadHash())
	}
	if got.RoundID() != "" {
		t.Errorf("RoundID() = %q; want empty for internal OPENING", got.RoundID())
	}
	if got.GameID() != "" {
		t.Errorf("GameID() = %q; want empty for internal OPENING", got.GameID())
	}
	if referenceID, present := got.ReferenceExternalTransactionID(); present || referenceID != "" {
		t.Errorf("ReferenceExternalTransactionID() = (%q, %t); want empty and absent for internal OPENING", referenceID, present)
	}
}

func TestNewOpeningWagerTransactionRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*OpeningWagerTransactionInput)
		wantError error
	}{
		{name: "empty transaction ID", change: func(input *OpeningWagerTransactionInput) { input.ID = "" }, wantError: ErrInvalidTransactionID},
		{name: "blank transaction ID", change: func(input *OpeningWagerTransactionInput) { input.ID = " \t" }, wantError: ErrInvalidTransactionID},
		{name: "empty wallet ID", change: func(input *OpeningWagerTransactionInput) { input.WalletID = "" }, wantError: ErrInvalidWalletID},
		{name: "blank wallet ID", change: func(input *OpeningWagerTransactionInput) { input.WalletID = "\n" }, wantError: ErrInvalidWalletID},
		{name: "empty player ID", change: func(input *OpeningWagerTransactionInput) { input.PlayerID = "" }, wantError: ErrInvalidPlayerID},
		{name: "blank player ID", change: func(input *OpeningWagerTransactionInput) { input.PlayerID = "  " }, wantError: ErrInvalidPlayerID},
		{name: "uninitialized money", change: func(input *OpeningWagerTransactionInput) { input.Money = Money{} }, wantError: ErrInvalidMoney},
		{name: "zero money", change: func(input *OpeningWagerTransactionInput) { input.Money = mustNewMoney(t, "0.00", "BRL") }, wantError: ErrInvalidWagerTransactionAmount},
		{name: "negative money", change: func(input *OpeningWagerTransactionInput) { input.Money = mustNewMoney(t, "-0.01", "BRL") }, wantError: ErrInvalidWagerTransactionAmount},
		{name: "zero creation time", change: func(input *OpeningWagerTransactionInput) { input.CreatedAt = time.Time{} }, wantError: ErrInvalidWagerTransactionTimestamp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validOpeningWagerTransactionInput(t)
			tt.change(&input)

			_, err := NewOpeningWagerTransaction(input)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("NewOpeningWagerTransaction() error = %v; want %v", err, tt.wantError)
			}
			assertErrorContains(t, err, "create opening wager transaction", tt.wantError.Error())
		})
	}
}

func validOpeningWagerTransactionInput(t *testing.T) OpeningWagerTransactionInput {
	t.Helper()

	return OpeningWagerTransactionInput{
		ID:        TransactionID("opening-transaction-1"),
		WalletID:  WalletID("wallet-1"),
		PlayerID:  PlayerID("player-1"),
		Money:     mustNewMoney(t, "1000.00", "BRL"),
		CreatedAt: time.Date(2026, time.October, 14, 10, 0, 0, 0, time.UTC),
	}
}

func validExternalWagerTransactionInput(t *testing.T) ExternalWagerTransactionInput {
	t.Helper()

	return ExternalWagerTransactionInput{
		ID:                    TransactionID("transaction-1"),
		ProviderID:            ProviderID("provider-1"),
		ExternalTransactionID: ExternalTransactionID("external-transaction-1"),
		IdempotencyKey:        IdempotencyKey("idempotency-key-1"),
		PayloadHash:           PayloadHash("sha256:payload-hash-1"),
		WalletID:              WalletID("wallet-1"),
		PlayerID:              PlayerID("player-1"),
		RoundID:               RoundID("round-1"),
		GameID:                GameID("game-1"),
		Kind:                  WagerTransactionKindBet,
		Money:                 mustNewMoney(t, "25.00", "BRL"),
		CreatedAt:             time.Date(2026, time.October, 13, 18, 0, 0, 0, time.UTC),
	}
}

func assertWagerTransactionError(t *testing.T, err error, want error) {
	t.Helper()

	if !errors.Is(err, want) {
		t.Fatalf("NewExternalWagerTransaction() error = %v; want %v", err, want)
	}
	assertErrorContains(t, err, "create external wager transaction", want.Error())
}
