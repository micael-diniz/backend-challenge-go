package httpapi

import "net/http"

func NewHandler(wallets *WalletHandler, bets *WagerTransactionHandler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /wallets", wallets)
	mux.Handle("POST /wagering/transactions", bets)
	return mux
}
