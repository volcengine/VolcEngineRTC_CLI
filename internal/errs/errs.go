// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package errs defines the typed error envelope used across the CLI.
//
// CLI-owned failures are expressed with a structured envelope so an
// Agent can branch on the stable `error.code` before ever reading the human
// message. This is a distinct system from the SDK runtime error codes queried
// by `explain-error` (see internal/errorcodes).
package errs

import (
	"errors"
	"fmt"
)

// Type is the coarse error category an Agent branches on first.
type Type string

const (
	TypeValidation   Type = "validation"   // bad flag/arg/config value
	TypeNotFound     Type = "not_found"    // missing resource (template, code, file)
	TypeAuth         Type = "auth"         // not authenticated / missing credential
	TypePrecondition Type = "precondition" // required prior step not done
	TypeIO           Type = "io"           // filesystem / process failure
	TypeInternal     Type = "internal"     // unexpected CLI bug
)

// Error is the typed, Agent-parseable CLI error.
type Error struct {
	// Code is the stable, catalog-registered identifier (e.g.
	// "vertc.config.missing_field"). Agents branch on this.
	Code string `json:"code"`
	// Type is the coarse category.
	Type Type `json:"type"`
	// Subtype narrows the category (free-form, e.g. "missing_field").
	Subtype string `json:"subtype,omitempty"`
	// Param points at the offending flag/field/argument when applicable.
	Param string `json:"param,omitempty"`
	// Message is the human-readable explanation.
	Message string `json:"message"`
	// Hint is an actionable next step (also mirrored to stderr).
	Hint string `json:"hint,omitempty"`
	// Details carries safe, command-specific structured recovery data. Callers
	// must never place credentials or other secrets here.
	Details map[string]any `json:"details,omitempty"`
	// exit is the process exit code (defaults to 1).
	exit int
	// wrapped is an optional underlying cause (not serialized).
	wrapped error
	// reported marks that the command already emitted a data envelope to
	// stdout (e.g. doctor/validate). The output layer then only sets the exit
	// code and writes the hint to stderr, avoiding a second stdout envelope.
	reported bool
}

func (e *Error) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Hint)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.wrapped }

// ExitCode returns the process exit code (never 0 for an error).
func (e *Error) ExitCode() int {
	if e.exit <= 0 {
		return 1
	}
	return e.exit
}

// WithParam attaches the offending flag/field name.
func (e *Error) WithParam(p string) *Error { e.Param = p; return e }

// WithHint sets the actionable hint.
func (e *Error) WithHint(format string, a ...any) *Error {
	e.Hint = fmt.Sprintf(format, a...)
	return e
}

// WithDetails attaches safe structured recovery data for Agent callers.
func (e *Error) WithDetails(details map[string]any) *Error { e.Details = details; return e }

// WithCause attaches an underlying error for logging (not serialized).
func (e *Error) WithCause(err error) *Error { e.wrapped = err; return e }

// WithExit overrides the exit code.
func (e *Error) WithExit(code int) *Error { e.exit = code; return e }

// Reported marks that a data envelope was already written to stdout, so the
// output layer must not print a second (error) envelope.
func (e *Error) Reported() *Error { e.reported = true; return e }

// IsReported reports whether the command already emitted stdout output.
func (e *Error) IsReported() bool { return e.reported }

// New builds an Error and asserts its code is registered in the catalog.
// Using an unregistered code panics in tests (see catalog CI check) so codes
// stay converged in one place.
func New(code string, t Type, format string, a ...any) *Error {
	return &Error{
		Code:    code,
		Type:    t,
		Subtype: subtypeOf(code),
		Message: fmt.Sprintf(format, a...),
		exit:    1,
	}
}

// As unwraps err into a *Error if it is one.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Wrap converts an arbitrary error into an internal Error envelope.
func Wrap(err error, code string, format string, a ...any) *Error {
	return New(code, TypeInternal, format, a...).WithCause(err)
}
