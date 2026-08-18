// Package http provides shared HTTP helpers for the Tirek API: RFC 7807
// problem+json errors, JSON encoding/decoding, and validation.
package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

// Problem is an RFC 7807 application/problem+json error body.
type Problem struct {
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Status    int            `json:"status"`
	Detail    string         `json:"detail,omitempty"`
	Instance  string         `json:"instance,omitempty"`
	Code      string         `json:"code"`
	RequestID string         `json:"request_id,omitempty"`
	Fields    []FieldProblem `json:"fields,omitempty"`
}

// FieldProblem describes a single field validation failure.
type FieldProblem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ErrType is the base URL used to build machine-readable error types.
const ErrType = "https://api.tirek.kz/errors/"

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Nothing sensible left to do; headers already sent.
		return
	}
}

// WriteProblem writes an RFC 7807 error body.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	p := Problem{
		Type:      ErrType + strings.ToLower(strings.ReplaceAll(code, "_", "-")),
		Title:     title,
		Status:    status,
		Detail:    detail,
		Instance:  r.URL.Path,
		Code:      code,
		RequestID: middleware.GetReqID(r.Context()),
	}
	WriteJSON(w, status, p)
}

// WriteValidationErrors writes a 400 problem with field-level validation
// failures.
func WriteValidationErrors(w http.ResponseWriter, r *http.Request, errors ValidationErrors) {
	fields := make([]FieldProblem, 0, len(errors))
	for _, e := range errors {
		fields = append(fields, FieldProblem(e))
	}
	p := Problem{
		Type:      ErrType + "validation-error",
		Title:     "Validation failed",
		Status:    http.StatusBadRequest,
		Detail:    errors.Error(),
		Instance:  r.URL.Path,
		Code:      "VALIDATION_ERROR",
		RequestID: middleware.GetReqID(r.Context()),
		Fields:    fields,
	}
	WriteJSON(w, http.StatusBadRequest, p)
}

// maxBodyBytes caps request bodies at 1 MiB (see docs/architecture/api.md §10).
const maxBodyBytes = 1 << 20

// DecodeJSON decodes a single JSON object from the request body, enforcing a
// 1 MiB limit and rejecting trailing garbage or unknown fields.
func DecodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Reject additional JSON values after the first object.
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}
