// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tcell

import (
	"context"
	"errors"
	"sync"

	tcellv2 "github.com/gdamore/tcell/v2"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// ErrDriverBackpressure is returned by PostEvent when the tcell event
// queue is full (spec-0.13 T-13 code-reviewer MED-2 PostEvent contract).
// Callers may retry or drop the event; the queue holds tcell's default
// 10-event buffer.
var ErrDriverBackpressure = errors.New("tcell driver: PostEvent queue full")

// ErrScreenClosed is returned by PollEvent when the underlying tcell
// screen has been finalized (tcell.Screen.PollEvent returned nil). This
// is the normal shutdown path — the PollEvent goroutine treats it as a
// signal to exit. A sentinel error (rather than a bare (nil, nil))
// keeps the contract nilnil-clean (spec-1.17 R3 golangci fix).
var ErrScreenClosed = errors.New("tcell driver: screen closed")

// Driver implements terminal.Driver over a tcell.Screen. Constructed by
// NewDriver; lifecycle owned by scheduler.Run (Init/Fini via sync.Once).
type Driver struct {
	screen   tcellv2.Screen
	cols     int
	rows     int
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
		d.cols, d.rows = d.screen.Size()
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

// Size returns the cached cols/rows. Resize updates the cache; the
// underlying tcell.Screen.Size is consulted only at Init.
func (d *Driver) Size() (cols, rows int) {
	return d.cols, d.rows
}

// SetCell writes a single styled cell. Coordinates outside Size()
// are silently dropped by tcell.
func (d *Driver) SetCell(x, y int, ch rune, st style.Style) {
	d.screen.SetContent(x, y, ch, nil, toTcellStyle(st))
}

// PollEvent translates a tcell event to a terminal.Event, honoring
// ctx cancellation by posting an EventInterrupt sentinel.
//
// The translation table is encoded in fromTcellKey / fromTcellMod.
// Unknown tcell events return (nil, nil) so the caller loop continues
// (spec-1.17 D-6a: future event types fall-through safely).
//
// ctx.Done bridge: a goroutine paired with each PollEvent call would be
// expensive; instead callers (program.pollEvents) ensure their own
// ctx-cancel routing via scheduler.EmitMsg. tcell.PollEvent returns nil
// when screen.Fini is called, which the caller treats as exit.
func (d *Driver) PollEvent(ctx context.Context) (terminal.Event, error) {
	// Check ctx upfront — saves us from blocking in tcell when caller
	// has already cancelled.
	select {
	case <-ctx.Done():
		// errcode-lint:exempt -- spec-1.17 D-6a: ctx.Err is a stdlib sentinel (context.Canceled / DeadlineExceeded); caller maps it.
		return nil, ctx.Err()
	default:
	}

	ev := d.screen.PollEvent()
	if ev == nil {
		// Screen post-Fini; signal the caller loop to exit (normal shutdown).
		// errcode-lint:exempt -- spec-1.17 D-6a: ErrScreenClosed is a package sentinel; the PollEvent goroutine returns on any non-nil err.
		return nil, ErrScreenClosed
	}
	switch e := ev.(type) {
	case *tcellv2.EventKey:
		return fromTcellEventKey(e), nil
	case *tcellv2.EventResize:
		cols, rows := e.Size()
		d.cols, d.rows = cols, rows
		return terminal.EventResize{Cols: cols, Rows: rows}, nil
	case *tcellv2.EventInterrupt:
		return terminal.EventInterrupt{Data: e.Data()}, nil
	}
	// Unknown/uninteresting tcell event (mouse / paste / focus / time).
	// Return an EventInterrupt the program loop drops so polling continues —
	// returning (nil, nil) here would make the caller exit the loop.
	return terminal.EventInterrupt{Data: nil}, nil
}

// PostEvent enqueues a terminal.Event onto the tcell event queue so the
// next PollEvent observes it. Returns ErrDriverBackpressure when the
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
		// errcode-lint:exempt -- spec-1.17 D-6a: ErrDriverBackpressure is a package sentinel (spec-0.13 T-13 MED-2 PostEvent backpressure contract); callers errors.Is against it.
		return ErrDriverBackpressure
	}
	return nil
}

// Resize updates the cached cols/rows. Called by callers that learn of
// a resize through external channels (signal handler, etc.); tcell's
// own resize path runs through PollEvent → EventResize.
func (d *Driver) Resize(cols, rows int) {
	d.cols, d.rows = cols, rows
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
