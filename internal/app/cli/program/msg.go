// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

// KeyMsg is dispatched into the main loop for every key event observed
// by the PollEvent goroutine. R3.5 routes KeyMsg via scheduler.EmitMsg
// (priority queue) so worker pool saturation cannot delay key delivery.
//
// Code/Rune/Mod follow the spec-0.13 terminal.Event convention
// (Code = terminal.KeyCtrlC / KeyEscape / etc.; Rune populated when
// Code == KeyRune).
type KeyMsg struct {
	Code int
	Rune rune
	Mod  uint8
}

// ResizeMsg is dispatched when the terminal observes a resize. Pull-
// style is the primary contract (Model.View(cols, rows) receives the
// current scrollback size every frame); ResizeMsg is a hint for Models
// that want to invalidate caches or re-wrap content.
type ResizeMsg struct {
	Cols, Rows int
}

// QuitMsg signals the program to shut down. Emitted on the second
// Ctrl+C within the quit window (see D-5) or from the /quit slash
// command path introduced in spec-1.16.
type QuitMsg struct{}

// CancelCmdMsg is emitted on Esc (first press) or Ctrl+C (first press,
// within the quit window). Carrying intent for Models to cancel any
// in-flight Cmd context without quitting the program.
type CancelCmdMsg struct{}

// quitDisarmMsg is an internal Msg posted by the quit-window timer
// when it fires. It clears p.quitArmed back to false on the scheduler
// main goroutine, avoiding the cross-goroutine race that would result
// from writing p.quitArmed directly from the timer callback.
type quitDisarmMsg struct{}
