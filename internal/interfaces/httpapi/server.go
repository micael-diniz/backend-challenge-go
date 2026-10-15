package httpapi

import (
	"expvar"
	"net/http"
)

func NewHandler(wallets *WalletHandler, bets *WagerTransactionHandler, reads *ReadHandler, health *HealthHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", health.Live)
	mux.HandleFunc("GET /health/ready", health.Ready)
	mux.Handle("GET /metrics", expvar.Handler())
	mux.Handle("POST /wallets", wallets)
	mux.Handle("POST /wagering/transactions", bets)
	mux.HandleFunc("GET /wallets/{walletId}", reads.Wallet)
	mux.HandleFunc("GET /wallets/{walletId}/ledger", reads.Ledger)
	mux.HandleFunc("POST /wallets/{walletId}/reconciliation", reads.Reconciliation)
	mux.HandleFunc("GET /wagering/transactions/{transactionId}", reads.Transaction)
	mux.HandleFunc("GET /providers/{providerId}/wagering/transactions/{externalTransactionId}", reads.ProviderTransaction)
	return mux
}
