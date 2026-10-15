package httpapi

import (
	"context"
	"log/slog"
	"net/http"
)

type HealthHandler struct {
	ready func(context.Context) error
}

func NewHealthHandler(ready func(context.Context) error) *HealthHandler {
	return &HealthHandler{ready: ready}
}

func (handler *HealthHandler) Live(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, healthResponse{Status: "live"})
}

func (handler *HealthHandler) Ready(response http.ResponseWriter, request *http.Request) {
	if handler.ready == nil {
		writeJSON(response, http.StatusServiceUnavailable, healthResponse{Status: "not_ready"})
		return
	}
	if err := handler.ready(request.Context()); err != nil {
		slog.Warn("health ready failed", "error", err.Error())
		writeJSON(response, http.StatusServiceUnavailable, healthResponse{Status: "not_ready"})
		return
	}
	writeJSON(response, http.StatusOK, healthResponse{Status: "ready"})
}

type healthResponse struct {
	Status string `json:"status"`
}
