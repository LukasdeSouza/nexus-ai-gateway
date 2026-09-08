// Package handlers contains all HTTP request handlers for the gateway.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/domain"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
)

// ErrorResponse is the stable JSON error envelope.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail holds the structured error fields.
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// writeJSON serialises v as JSON and sets Content-Type.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a structured JSON error response mapped from a domain error.
func writeError(w http.ResponseWriter, requestID string, err error) {
	var (
		code      domain.Code
		message   string
		retryable bool
		status    int
	)

	if de, ok := err.(*domain.Error); ok {
		code = de.Code
		message = de.Message
		retryable = de.Retryable
	} else if pe, ok := err.(*provider.ProviderError); ok {
		code = domain.CodeProviderError
		message = pe.Message
		retryable = pe.Retryable
		status = pe.StatusCode
	} else {
		code = domain.CodeInternal
		message = err.Error()
	}

	if status == 0 {
		switch code {
	case domain.CodeNotFound:
		status = http.StatusNotFound
	case domain.CodeUnauthorized:
		status = http.StatusUnauthorized
	case domain.CodeForbidden:
		status = http.StatusForbidden
	case domain.CodeBadRequest:
		status = http.StatusBadRequest
	case domain.CodeConflict:
		status = http.StatusConflict
	case domain.CodeRateLimitExceeded:
		status = http.StatusTooManyRequests
	case domain.CodeBudgetExceeded:
		status = http.StatusPaymentRequired
	case domain.CodeProviderTimeout:
		status = http.StatusGatewayTimeout
	case domain.CodeProviderUnavailable:
		status = http.StatusBadGateway
	case domain.CodeProviderError:
		status = http.StatusBadGateway
	default:
		status = http.StatusInternalServerError
	}
	}

	writeJSON(w, status, ErrorResponse{
		Error: ErrorDetail{
			Code:      string(code),
			Message:   message,
			Retryable: retryable,
			RequestID: requestID,
		},
	})
}

// decodeJSON decodes the request body into v.
func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return domain.New(domain.CodeBadRequest, "invalid JSON body: "+err.Error())
	}
	return nil
}
