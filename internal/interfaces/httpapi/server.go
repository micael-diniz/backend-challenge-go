package httpapi

import "net/http"

func NewHandler(wallets *WalletHandler, bets *WagerTransactionHandler, reads *ReadHandler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /wallets", wallets)
	mux.Handle("POST /wagering/transactions", bets)
	mux.HandleFunc("GET /wallets/{walletId}", reads.Wallet)
	mux.HandleFunc("GET /wallets/{walletId}/ledger", reads.Ledger)
	mux.HandleFunc("GET /wagering/transactions/{transactionId}", reads.Transaction)
	mux.HandleFunc("GET /providers/{providerId}/wagering/transactions/{externalTransactionId}", reads.ProviderTransaction)
	return mux
}
