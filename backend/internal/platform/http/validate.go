package http

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
)

// ValidationError aggregates field-level validation failures.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is the set of field errors returned on a 400.
type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
	parts := make([]string, 0, len(v))
	for _, e := range v {
		parts = append(parts, e.Field+": "+e.Message)
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Code implements the api error interface.
func (v ValidationErrors) Code() string { return "VALIDATION_ERROR" }

// Validator accumulates field errors.
type Validator struct {
	errors ValidationErrors
}

func (v *Validator) Error() error {
	if len(v.errors) == 0 {
		return nil
	}
	return v.errors
}

// Email validates a non-empty, well-formed email address (bounded length).
func (v *Validator) Email(field, value string) {
	if value == "" {
		v.Add(field, "is required")
		return
	}
	if len(value) > 254 {
		v.Add(field, "is too long")
		return
	}
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value {
		v.Add(field, "is not a valid email address")
		return
	}
	if len(value) != len(addr.Address) || strings.ContainsAny(value, " \t\r\n") {
		v.Add(field, "is not a valid email address")
	}
}

// Required validates a non-empty, length-bounded string.
func (v *Validator) Required(field, value string, maxLen int) {
	if strings.TrimSpace(value) == "" {
		v.Add(field, "is required")
		return
	}
	v.MaxLen(field, value, maxLen)
}

// MaxLen bounds a string's length.
func (v *Validator) MaxLen(field, value string, maxLen int) {
	if len(value) > maxLen {
		v.Add(field, fmt.Sprintf("must be at most %d characters", maxLen))
	}
}

// OneOf validates membership in an allowed set.
func (v *Validator) OneOf(field, value string, allowed ...string) {
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	v.Add(field, fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", ")))
}

var upperAlphaRe = regexp.MustCompile(`^[A-Z]+$`)

// UpperAlpha validates that a value is exactly n uppercase ASCII letters
// (used for ISO country codes and currency codes).
func (v *Validator) UpperAlpha(field, value string, n int) {
	if value == "" {
		v.Add(field, "is required")
		return
	}
	if len(value) != n || !upperAlphaRe.MatchString(value) {
		v.Add(field, fmt.Sprintf("must be %d uppercase letters", n))
	}
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// UUID validates a canonical UUID string.
func (v *Validator) UUID(field, value string) {
	if !uuidRe.MatchString(value) {
		v.Add(field, "must be a valid UUID")
	}
}

func (v *Validator) Add(field, message string) {
	v.errors = append(v.errors, ValidationError{Field: field, Message: message})
}
