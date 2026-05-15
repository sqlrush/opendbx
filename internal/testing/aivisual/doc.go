// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package aivisual implements Layer 4 of the spec-0.11.5 UI Review
// Pipeline: AI visual review against a local Qwen2.5-VL-72B endpoint
// (OpenAI-compatible chat completions API on :8082).
//
// Workflow:
//   1. Construct a Reviewer; defaults to http://127.0.0.1:8082/v1.
//   2. Call Review(ctx, png, focus) with the snapshot PNG.
//   3. The reviewer ships the PNG + prompt template (frozen at
//      testdata/prompt.txt; SHA-256 verified by TestPromptFrozen) to
//      the endpoint with response_format JSON.
//   4. Returns a Report (Verdict / Issues / Tokens).
//
// Failure modes:
//   - Endpoint unreachable (dial/timeout/reset) → returns a sentinel
//     mapped to errcode AIVISUAL.ENDPOINT_DOWN; tests should Skip(),
//     not Fail (Layer 4 is best-effort non-blocking).
//   - Invalid JSON response → Report.Verdict = "uncertain" fallback.
//
// CI behavior: Layer 4 runs only when LOCAL_VL_ENDPOINT secret is set
// (spec § D-5). Dev local runs require llama-server on :8082.
//
// Design: spec-0.11.5-ui-review-pipeline § D-3.
package aivisual
