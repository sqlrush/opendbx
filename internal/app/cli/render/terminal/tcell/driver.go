// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tcell

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	tcellv2 "github.com/gdamore/tcell/v2"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// errDriverBackpressure is returned by PostEvent when the tcell event
// queue is full (spec-0.13 T-13 code-reviewer MED-2 PostEvent contract).
// Unexported (spec-1.17 R-fix MED-7): an internal flow-control signal,
// not a user-facing error — no cross-package caller errors.Is it.
var errDriverBackpressure = errors.New("tcell driver: PostEvent queue full")

// errScreenClosed is returned by PollEvent when the underlying tcell
// screen has been finalized (tcell.Screen.PollEvent returned nil). This
// is the normal shutdown path — the PollEvent goroutine treats it as a
// signal to exit. A sentinel error (rather than a bare (nil, nil)) keeps
// the contract nilnil-clean. Unexported (spec-1.17 R-fix MED-7): internal
// signal only; the poll goroutine returns on any non-nil err.
var errScreenClosed = errors.New("tcell driver: screen closed")

// Driver implements terminal.Driver over a tcell.Screen. Constructed by
// NewDriver; lifecycle owned by scheduler.Run (Init/Fini via sync.Once).
//
// Concurrency (spec-1.17 R-fix HIGH-2): cols/rows are accessed from two
// goroutines — the scheduler goroutine (Size() every frame, Resize()) and
// the PollEvent goroutine (EventResize). They are stored as atomic.Int32
// so concurrent read/write is race-free. A torn one-frame-stale size read
// is harmless for rendering (the next frame self-corrects).
type Driver struct {
	screen   tcellv2.Screen
	cols     atomic.Int32
	rows     atomic.Int32
	initOnce sync.Once
	initErr  error
	finiOnce sync.Once
}

// Compile-time check that *Driver satisfies the terminal.Driver interface.
var _ terminal.Driver = (*Driver)(nil)

// NewDriver wraps a tcell.Screen as a terminal.Driver. The screen MUST
// NOT be initialized — Driver.Init owns screen.Init(). Pass a screen
// constructed by tui.NewScreen-style factories that return uninitialized
// screens; for tests use a SimulationScreen that the caller has not yet
// Init'd (driver.Init will run Init).
func NewDriver(screen tcellv2.Screen) *Driver {
	if screen == nil {
		panic("tcell.NewDriver: screen must be non-nil")
	}
	return &Driver{screen: screen}
}

// Init initializes the underlying tcell.Screen and caches its initial
// size. Idempotent via sync.Once — scheduler.Run may call Init at most
// once, but tests that re-invoke don't double-Init.
func (d *Driver) Init() error {
	d.initOnce.Do(func() {
		if err := d.screen.Init(); err != nil {
			d.initErr = errcode.Wrap("TERMINAL.INIT_FAILED", err,
				"tcell.Screen.Init failed (render/terminal/tcell adapter)",
				"verify $TERM is set and the terminal supports ANSI escape sequences")
			return
		}
		cols, rows := d.screen.Size()
		d.cols.Store(dim(cols))
		d.rows.Store(dim(rows))
	})
	// errcode-lint:exempt -- spec-1.17 D-6a: d.initErr is errcode.Wrap'd (TERMINAL.INIT_FAILED) at the assignment above; nil on success.
	return d.initErr
}

// Fini cleans up the underlying screen. Idempotent via sync.Once so
// scheduler shutdown + any leftover defer paths can both call without
// double-Fini panics.
func (d *Driver) Fini() {
	d.finiOnce.Do(func() {
		d.screen.Fini()
	})
}

// Show flushes the current frame to the terminal.
func (d *Driver) Show() { d.screen.Show() }

// Sync re-emits the full frame; used after resize / suspend.
func (d *Driver) Sync() { d.screen.Sync() }

// Clear wipes the back buffer; next Show paints a blank frame.
func (d *Driver) Clear() { d.screen.Clear() }

// Size returns the cached cols/rows (atomic reads; spec-1.17 R-fix
// HIGH-2). Resize / the PollEvent resize branch update the cache; the
// underlying tcell.Screen.Size is consulted only at Init.
func (d *Driver) Size() (cols, rows int) {
	return int(d.cols.Load()), int(d.rows.Load())
}

// SetCell writes a single styled cell. Coordinates outside Size()
// are silently dropped by tcell.
func (d *Driver) SetCell(x, y int, ch rune, st style.Style) {
	d.screen.SetContent(x, y, ch, nil, toTcellStyle(st))
}

// PollEvent translates a tcell event to a terminal.Event.
//
// ctx contract (spec-1.17 R-fix MED-5): PollEvent returns ctx.Err() if
// ctx is cancelled either before the call or *while blocked* in
// tcell.screen.PollEvent. The cancel-while-blocked case is handled by a
// short-lived bridge goroutine that posts a tcell interrupt on ctx.Done,
// waking the blocked PollEvent; on return we re-check ctx and surface
// ctx.Err() ahead of the woken event. The bridge is torn down before
// PollEvent returns (no goroutine leak). Keystrokes are human-paced, so
// the per-call goroutine cost is negligible.
//
// Unknown/uninteresting tcell events return an EventInterrupt the caller
// loop drops, so polling continues (spec-1.17 D-6a: future event types
// fall through safely without exiting the input loop).
func (d *Driver) PollEvent(ctx context.Context) (terminal.Event, error) {
	// Fast path: ctx already cancelled — don't block in tcell.
	select {
	case <-ctx.Done():
		// errcode-lint:exempt -- spec-1.17 D-6a: ctx.Err is a stdlib sentinel (context.Canceled / DeadlineExceeded); caller maps it.
		return nil, ctx.Err()
	default:
	}

	// Bridge ctx cancellation to wake a blocked screen.PollEvent.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			// PostEvent is thread-safe; ignore err (screen may be Fini'd).
			_ = d.screen.PostEvent(tcellv2.NewEventInterrupt(ctxWakeup{}))
		case <-stop:
		}
	}()

	ev := d.screen.PollEvent()

	// If ctx was cancelled while we were blocked, surface it ahead of the
	// (possibly bridge-injected) event — honors the Driver ctx contract.
	// errcode-lint:exempt -- spec-1.17 MED-5: ctx.Err is a stdlib sentinel (context.Canceled / DeadlineExceeded); caller maps it.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if ev == nil {
		// Screen post-Fini; signal the caller loop to exit (normal shutdown).
		// errcode-lint:exempt -- spec-1.17 D-6a: errScreenClosed is a package sentinel; the PollEvent goroutine returns on any non-nil err.
		return nil, errScreenClosed
	}
	switch e := ev.(type) {
	case *tcellv2.EventKey:
		return fromTcellEventKey(e), nil
	case *tcellv2.EventResize:
		cols, rows := e.Size()
		d.cols.Store(dim(cols))
		d.rows.Store(dim(rows))
		return terminal.EventResize{Cols: cols, Rows: rows}, nil
	case *tcellv2.EventInterrupt:
		return terminal.EventInterrupt{Data: e.Data()}, nil
	}
	// Unknown/uninteresting tcell event (mouse / paste / focus / time).
	// Return an EventInterrupt the caller loop drops so polling continues —
	// returning (nil, nil) here would make the caller exit the loop.
	return terminal.EventInterrupt{Data: nil}, nil
}

// ctxWakeup is the payload for the bridge interrupt PollEvent posts on
// ctx cancellation. Distinct type so a reader can tell it apart from a
// real EventInterrupt payload in traces.
type ctxWakeup struct{}

// dim narrows a terminal dimension (cols/rows) to int32 for atomic
// storage. Terminal dimensions are small non-negative values (tcell caps
// well under int32), so the conversion cannot overflow; gosec G115
// cannot prove the bound.
//
//nolint:gosec // spec-1.17 R-fix HIGH-2: terminal cols/rows are small non-negative ints, never overflow int32.
func dim(v int) int32 { return int32(v) }

// PostEvent enqueues a terminal.Event onto the tcell event queue so the
// next PollEvent observes it. Returns errDriverBackpressure when the
// queue is full (spec-0.13 T-13 MED-2 contract).
//
// Currently only EventInterrupt is round-tripped (used by program for
// ctx-cancel signaling). EventKey / EventResize are not posted by
// production callers; if needed, extend toTcellEvent.
func (d *Driver) PostEvent(ev terminal.Event) error {
	tev := toTcellEvent(ev)
	if tev == nil {
		return nil // unsupported event type — silent no-op (caller can extend)
	}
	if err := d.screen.PostEvent(tev); err != nil {
		// errcode-lint:exempt -- spec-1.17 D-6a: errDriverBackpressure is a package sentinel (spec-0.13 T-13 MED-2 PostEvent backpressure contract).
		return errDriverBackpressure
	}
	return nil
}

// Resize updates the cached cols/rows (atomic store; spec-1.17 R-fix
// HIGH-2). Called by the scheduler goroutine; tcell's own resize path
// runs through PollEvent → EventResize.
func (d *Driver) Resize(cols, rows int) {
	d.cols.Store(dim(cols))
	d.rows.Store(dim(rows))
	d.screen.Sync()
}

// ── translation helpers ────────────────────────────────────────────────

// fromTcellEventKey maps a tcell.EventKey to a terminal.EventKey,
// normalizing tcell.KeyBackspace2 (DEL/127) to terminal.KeyBackspace
// (BS/8) so callers see a single backward-delete code (spec-1.17 R-10).
func fromTcellEventKey(e *tcellv2.EventKey) terminal.EventKey {
	k := e.Key()
	r := e.Rune()

	var code int
	switch k {
	case tcellv2.KeyRune:
		code = terminal.KeyRune
	case tcellv2.KeyBackspace, tcellv2.KeyBackspace2:
		code = terminal.KeyBackspace // normalize BS + DEL → KeyBackspace
	default:
		code = int(k)
	}

	return terminal.EventKey{
		Code:         code,
		Rune:         r,
		ShiftCtrlAlt: fromTcellMod(e.Modifiers()),
	}
}

// fromTcellMod converts tcell.ModMask bits to terminal.Mod* bits.
// tcell.ModMeta is dropped — opendbx does not surface Meta in keybindings.
func fromTcellMod(m tcellv2.ModMask) uint8 {
	var out uint8
	if m&tcellv2.ModShift != 0 {
		out |= terminal.ModShift
	}
	if m&tcellv2.ModCtrl != 0 {
		out |= terminal.ModCtrl
	}
	if m&tcellv2.ModAlt != 0 {
		out |= terminal.ModAlt
	}
	return out
}

// toTcellEvent reverses the mapping for PostEvent. Only EventInterrupt
// is currently round-tripped (ctx-cancel signal). Returning nil means
// "unsupported event type — caller should not see this back through
// PollEvent" (silent drop by PostEvent).
func toTcellEvent(ev terminal.Event) tcellv2.Event {
	switch e := ev.(type) {
	case terminal.EventInterrupt:
		return tcellv2.NewEventInterrupt(e.Data)
	}
	return nil
}

// toTcellStyle converts a render/style.Style to tcell.Style.
//
// Color encoding (render/style):
//   - ColorDefault (0)        → tcell.ColorDefault
//   - 1..256 palette          → tcell.PaletteColor(idx-1)
//   - 0x1RRGGBB truecolor bit → tcell.NewRGBColor(r, g, b)
//
// Bold/Italic/Underline/Reverse map 1:1 to tcell attributes.
func toTcellStyle(st style.Style) tcellv2.Style {
	out := tcellv2.StyleDefault
	if c := toTcellColor(st.FG); c != tcellv2.ColorDefault {
		out = out.Foreground(c)
	}
	if c := toTcellColor(st.BG); c != tcellv2.ColorDefault {
		out = out.Background(c)
	}
	if st.Bold {
		out = out.Bold(true)
	}
	if st.Italic {
		out = out.Italic(true)
	}
	if st.Underline {
		out = out.Underline(true)
	}
	if st.Reverse {
		out = out.Reverse(true)
	}
	return out
}

// toTcellColor decodes the render/style.Color encoding.
func toTcellColor(c style.Color) tcellv2.Color {
	const truecolorBit = 0x1000000
	if c == 0 {
		return tcellv2.ColorDefault
	}
	if uint32(c) >= truecolorBit {
		rgb := uint32(c) & 0xFFFFFF
		// Each channel is masked to 8 bits, so the uint8 conversions
		// cannot overflow; gosec G115 cannot prove the mask bound.
		//nolint:gosec // spec-1.17 D-6a: & 0xFF masks each channel to 8 bits — uint8 conversion is provably in range.
		r := uint8((rgb >> 16) & 0xFF)
		//nolint:gosec // spec-1.17 D-6a: & 0xFF masks each channel to 8 bits — uint8 conversion is provably in range.
		g := uint8((rgb >> 8) & 0xFF)
		//nolint:gosec // spec-1.17 D-6a: & 0xFF masks each channel to 8 bits — uint8 conversion is provably in range.
		b := uint8(rgb & 0xFF)
		return tcellv2.NewRGBColor(int32(r), int32(g), int32(b))
	}
	// Palette: stored as idx+1 so 0 stays distinct from palette[0].
	return tcellv2.PaletteColor(int(c) - 1)
}
