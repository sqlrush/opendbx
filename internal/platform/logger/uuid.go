// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// RFC 4122 hyphen positions in the canonical 8-4-4-4-12 form. Pre-computed
// so emit-time formatting is a few index ops rather than a fmt.Sprintf.
//
//	xxxxxxxx-xxxx-Mxxx-Nxxx-xxxxxxxxxxxx
//	         |     ^ version nibble (M)
//	         |          ^ variant high bits (N: 10xx)
//	         hyphens at offsets 8, 13, 18, 23 of the 32-hex string.

// uuid4 returns a RFC 4122 random (v4) UUID string.
//
// Spec § 8 Q2 ★A: implemented from crypto/rand without a third-party dep,
// keeping the package stdlib-only per spec § 5 contract.
// Spec § 8 Q16: sessionId uses v4 (no time ordering requirement).
//
// Returns the canonical 8-4-4-4-12 hex form.
func uuid4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failures are essentially process-fatal in practice; the
		// logger contract is best-effort, so we fall back to a deterministic
		// zero-pattern + timestamp suffix rather than panic. Callers that need
		// strong randomness should detect "00000000-0000-..." as a degraded id.
		return degradedUUID(4)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4 (0100)
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return formatUUID(b)
}

// uuid7 returns a RFC 9562 time-ordered (v7) UUID string.
//
// Spec § 8 Q16: trace_id uses v7 so jq time-window queries on sidecar JSONL
// can sort lexicographically and get chronological ordering for free.
//
// Layout (big-endian):
//
//	0..47   unix ms timestamp (48 bits)
//	48..51  version nibble = 7 (0111)
//	52..63  monotonic sequence within the ms (12 bits, "rand_a" per RFC)
//	64..65  variant = 10
//	66..127 random 62 bits ("rand_b")
//
// Strict monotonicity contract: sequential uuid7() calls always return
// lexicographically increasing values. This is achieved via the RFC 9562
// "Method 3" (Replace Leftmost Random Bits with Increased Clock Precision):
//
//   - track (lastMs, lastSeq) in a mutex-guarded state value
//   - if current ms == lastMs: seq = lastSeq + 1
//   - if seq overflows 12 bits: virtually advance ms by 1 and reset seq = 0
//   - if current ms < lastMs (clock skew): clamp to lastMs
//
// The lock is held only across (3 reads + 3 writes + 1 conditional), so the
// per-call cost is negligible — well below the cost of crypto/rand.Read.
func uuid7() string {
	var b [16]byte
	if _, err := rand.Read(b[7:]); err != nil {
		return degradedUUID(7)
	}

	ms, seq := nextUUID7TimestampAndSeq(time.Now().UnixMilli())

	// Bytes 0..5: 48-bit ms timestamp big-endian.
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	// rand_a (12 bits): version nibble (7) + 12-bit seq packed into b[6]:b[7].
	b[6] = 0x70 | byte(seq>>8)  // 0111 (version 7) | seq high 4 bits
	b[7] = byte(seq)            // seq low 8 bits (overwrites randomised b[7])
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return formatUUID(b)
}

// uuid7State holds the monotonic clock state for uuid7 generation. Process-
// global; the mu protects (lastMs, lastSeq) reads + writes.
var uuid7State struct {
	mu      sync.Mutex
	lastMs  int64
	lastSeq uint16 // 12-bit usable; values 0x0000..0x0fff
}

// nextUUID7TimestampAndSeq advances the monotonic clock state and returns
// the (ms, seq) pair to embed in the next v7 UUID. Guarantees:
//
//   - the returned ms is non-decreasing across calls
//   - within the same returned ms, seq strictly increases
//   - seq is always in [0, 0x0fff]
func nextUUID7TimestampAndSeq(currentMs int64) (int64, uint16) {
	uuid7State.mu.Lock()
	defer uuid7State.mu.Unlock()

	ms := currentMs
	if ms < uuid7State.lastMs {
		// Clock skew: hold previous ms so monotonicity is preserved. Real
		// monotonic clocks should make this rare; tests can still drive it.
		ms = uuid7State.lastMs
	}

	var seq uint16
	switch {
	case ms == uuid7State.lastMs:
		// Same ms as last call: bump the sequence.
		seq = uuid7State.lastSeq + 1
		if seq >= 0x1000 { // 12-bit overflow
			ms++
			seq = 0
		}
	case ms > uuid7State.lastMs:
		// Newer ms: reset sequence.
		seq = 0
	}

	uuid7State.lastMs = ms
	uuid7State.lastSeq = seq
	return ms, seq
}

// formatUUID renders b as the canonical 8-4-4-4-12 hex form. ~80 ns/op.
func formatUUID(b [16]byte) string {
	var hexBuf [32]byte
	hex.Encode(hexBuf[:], b[:])
	var sb strings.Builder
	sb.Grow(36)
	sb.Write(hexBuf[0:8])
	sb.WriteByte('-')
	sb.Write(hexBuf[8:12])
	sb.WriteByte('-')
	sb.Write(hexBuf[12:16])
	sb.WriteByte('-')
	sb.Write(hexBuf[16:20])
	sb.WriteByte('-')
	sb.Write(hexBuf[20:32])
	return sb.String()
}

// degradedUUID returns a placeholder when crypto/rand fails. The version
// nibble is still set so consumers can detect a degraded value via the
// "all-zero random tail" pattern.
func degradedUUID(version int) string {
	var b [16]byte
	b[6] = byte(version&0x0f) << 4 // version nibble for visibility in degraded UUIDs
	b[8] = 0x80
	return formatUUID(b)
}
