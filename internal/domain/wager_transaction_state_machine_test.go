package domain

import "testing"

func TestWagerTransactionStatusCanTransitionTo(t *testing.T) {
	statuses := []WagerTransactionStatus{
		WagerTransactionStatusPending,
		WagerTransactionStatusPendingReference,
		WagerTransactionStatusProcessed,
		WagerTransactionStatusRejected,
		WagerTransactionStatusFailed,
	}
	allowed := map[WagerTransactionStatus]map[WagerTransactionStatus]bool{
		WagerTransactionStatusPending: {
			WagerTransactionStatusPendingReference: true,
			WagerTransactionStatusProcessed:        true,
			WagerTransactionStatusRejected:         true,
			WagerTransactionStatusFailed:           true,
		},
		WagerTransactionStatusPendingReference: {
			WagerTransactionStatusProcessed: true,
			WagerTransactionStatusRejected:  true,
			WagerTransactionStatusFailed:    true,
		},
	}

	for _, from := range statuses {
		for _, to := range statuses {
			want := allowed[from][to]
			if got := from.canTransitionTo(to); got != want {
				t.Errorf("%s.canTransitionTo(%s) = %t; want %t", from, to, got, want)
			}
		}
	}
}

func TestUnsupportedWagerTransactionStatusCannotTransition(t *testing.T) {
	unsupported := WagerTransactionStatus("UNKNOWN")

	if unsupported.canTransitionTo(WagerTransactionStatusProcessed) {
		t.Error("unsupported status can transition to PROCESSED; want false")
	}
	if WagerTransactionStatusPending.canTransitionTo(unsupported) {
		t.Error("PENDING can transition to unsupported status; want false")
	}
}
