// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Registered error codes for the config package. spec-0.6 D-3+D-4 migration.
// Public boundary errors returned through config.Load(), config.Validate(),
// or admin config verbs flow through these codes; internal helpers may
// continue to use fmt.Errorf as long as the outer boundary wraps with
// errcode.Wrap (user R2 B+ scope per spec-0.6 § 1.3.1).

package config

import (
	"errors"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

//nolint:gochecknoglobals // file-scope var = Register/sentinel per spec-0.6 § 2.2.1.
var (
	errENVParse = errors.New("config env parse failed")

	// ErrEnvParseFailed — OPENDBX_* env var value did not match the
	// expected shape (e.g. non-integer for an int field).
	ErrEnvParseFailed errcode.Sentinel = errcode.Register(
		"CONFIG.ENV_PARSE_FAILED",
		"OPENDBX_* env var value parse failed",
		"check the env var matches the field type (int / bool / duration / oneof)",
	)

	// ErrValidationFailed — schema-level rule violation (required / min /
	// max / oneof) detected by config.Validate.
	ErrValidationFailed errcode.Sentinel = errcode.Register(
		"CONFIG.VALIDATION_FAILED",
		"config validation rule failed",
		"run `opendbx admin config validate <file>` for the exact rule + path; fix the value and retry",
	)

	// ErrLoadFailed — yaml decode / source resolution failed during
	// config.Load. Typically wraps a stdlib / yaml root error.
	ErrLoadFailed errcode.Sentinel = errcode.Register(
		"CONFIG.LOAD_FAILED",
		"config file load failed",
		"check the file path + yaml syntax; --settings <path> to override the default chain",
	)

	// ErrAdminFieldNotFound — `admin config sources NoSuch.Field` referenced
	// a path that does not exist in the config tree.
	ErrAdminFieldNotFound errcode.Sentinel = errcode.Register(
		"CONFIG.ADMIN_FIELD_NOT_FOUND",
		"config field path not found",
		"use `opendbx admin config dump-defaults` to see the valid field names",
	)
)

func wrapLoadError(err error) error {
	if err == nil {
		return nil
	}
	if alreadyErrcode(err) {
		return err
	}
	if errors.Is(err, errENVParse) {
		return errcode.Wrap(ErrEnvParseFailed.Code(), err, "", "")
	}
	var validationErrs ValidationErrors
	if errors.As(err, &validationErrs) {
		return errcode.Wrap(ErrValidationFailed.Code(), err, "config validation failed", "")
	}
	return errcode.Wrap(ErrLoadFailed.Code(), err, "", "")
}

func wrapValidationError(err error) error {
	if err == nil {
		return nil
	}
	if alreadyErrcode(err) {
		return err
	}
	return errcode.Wrap(ErrValidationFailed.Code(), err, "", "")
}

func alreadyErrcode(err error) bool {
	var ec errcode.Error
	return errors.As(err, &ec)
}
