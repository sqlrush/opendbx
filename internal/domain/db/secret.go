// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File secret.go — SecretDSN, a non-rendering DSN secret (spec-1.19 D-2).
//
// A composed DSN embeds the password (and host/user). spec-1.19's three-route
// review CRIT: a bare string "never to be logged" is unenforceable —
// redact:"true" only masks struct fields, and fmt/log/errcode.Wrap will render
// a raw string. SecretDSN makes leakage structurally hard, mirroring
// spec-1.18's safeErr: every rendering path (String / GoString / Format / text
// marshal) emits the constant "<REDACTED-DSN>" — never the host/user/password.
// The one sanctioned way to obtain the raw DSN is Expose(), whose call sites
// are confined to the single bootstrap db.Open boundary (enforced by a grep
// lint in spec-1.19 D-8).

package db

import (
	"fmt"
	"io"
)

// redactedDSN is what every rendering path emits.
const redactedDSN = "<REDACTED-DSN>"

// SecretDSN wraps a raw DSN string so that it cannot be accidentally logged.
// The zero value is a valid empty secret (IsZero reports true).
type SecretDSN struct {
	raw string
}

// NewSecretDSN wraps a raw DSN. The raw string is never rendered; use Expose
// only at the db.Open boundary.
func NewSecretDSN(raw string) SecretDSN { return SecretDSN{raw: raw} }

// String renders the redaction placeholder (never the raw DSN). Implements
// fmt.Stringer so %v/%s on a SecretDSN value are safe.
func (SecretDSN) String() string { return redactedDSN }

// GoString renders the placeholder for %#v.
func (SecretDSN) GoString() string { return "db.SecretDSN(" + redactedDSN + ")" }

// Format covers every fmt verb (%v %s %q %x ...) so no verb can coax the raw
// DSN out via fmt. fmt prefers Format over String when both exist.
func (SecretDSN) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redactedDSN) }

// MarshalText keeps the raw DSN out of JSON/YAML/text encoders.
func (SecretDSN) MarshalText() ([]byte, error) { return []byte(redactedDSN), nil }

// IsZero reports whether the secret is empty.
func (s SecretDSN) IsZero() bool { return s.raw == "" }

// Expose returns the raw DSN. This is the ONLY accessor for the underlying
// secret; call it solely at the db.Open boundary (spec-1.19 D-8 grep lint
// confines call sites to internal/bootstrap). Never pass the result to a
// logger, error, or any sink other than the driver.
func (s SecretDSN) Expose() string { return s.raw }
