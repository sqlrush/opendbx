// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build spike
// +build spike

package spike

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sqlrush/opendbx/internal/app/cli/render/layout"
)

// FixtureNode is the JSON-decoded form of a FlexNode. Labels uniquely
// identify each node for golden-box mapping; pointer identity is lost
// across JSON load.
type FixtureNode struct {
	Label     string         `json:"label"`
	Direction string         `json:"direction,omitempty"` // "row" | "column" | ""
	Grow      float64        `json:"grow,omitempty"`
	Shrink    float64        `json:"shrink,omitempty"`
	Basis     int            `json:"basis,omitempty"`
	BasisMode string         `json:"basis_mode,omitempty"` // "fixed" | "auto" | ""
	Intrinsic *IntrinsicSize `json:"intrinsic,omitempty"`
	Children  []*FixtureNode `json:"children,omitempty"`
}

// IntrinsicSize is a leaf's natural (w, h) in cells.
type IntrinsicSize struct {
	W int `json:"w"`
	H int `json:"h"`
}

// FixtureViewport is the root box.
type FixtureViewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// FixtureBox is the JSON-decoded form of an expected layout.Box.
type FixtureBox struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// FixtureSource records the CC source/artifact that motivated a hand-written
// fixture. The spike deliberately uses manual fixtures, so every sample needs
// provenance strong enough for spec-1.1 to re-check against Claude Code.
type FixtureSource struct {
	Path string `json:"path"`
	Line string `json:"line,omitempty"`
	Note string `json:"note,omitempty"`
}

// Fixture is a complete CC UI sample with input + expected output.
type Fixture struct {
	Name     string                `json:"name"`
	Critical bool                  `json:"critical"`
	Note     string                `json:"note,omitempty"`
	CCCommit string                `json:"cc_commit"`
	Sources  []FixtureSource       `json:"sources"`
	Artifact string                `json:"artifact"`
	Viewport FixtureViewport       `json:"viewport"`
	Root     *FixtureNode          `json:"root"`
	Expected map[string]FixtureBox `json:"expected"`
}

// LoadFixture reads a fixture JSON file from disk.
//
// Uses json.Decoder with DisallowUnknownFields to catch typos in fixture
// authoring (T-10 claude-code-reviewer MED-2): a field name typo would
// otherwise produce a zero-value silently.
func LoadFixture(path string) (*Fixture, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open fixture %q: %w", path, err)
	}
	defer f.Close()
	var fx Fixture
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fx); err != nil {
		return nil, fmt.Errorf("parse fixture %q: %w", path, err)
	}
	if err := fx.Validate(); err != nil {
		return nil, fmt.Errorf("validate fixture %q: %w", path, err)
	}
	return &fx, nil
}

// Validate checks semantic fixture invariants not covered by JSON decoding.
func (fx *Fixture) Validate() error {
	if fx.Name == "" {
		return fmt.Errorf("fixture: name is required")
	}
	if fx.CCCommit == "" {
		return fmt.Errorf("fixture %q: cc_commit is required", fx.Name)
	}
	if len(fx.Sources) == 0 {
		return fmt.Errorf("fixture %q: at least one source is required", fx.Name)
	}
	for i, src := range fx.Sources {
		if src.Path == "" {
			return fmt.Errorf("fixture %q: sources[%d].path is required", fx.Name, i)
		}
	}
	if fx.Artifact == "" {
		return fmt.Errorf("fixture %q: artifact is required", fx.Name)
	}
	if fx.Viewport.Width <= 0 || fx.Viewport.Height <= 0 {
		return fmt.Errorf("fixture %q: viewport must be positive, got %dx%d", fx.Name, fx.Viewport.Width, fx.Viewport.Height)
	}
	if fx.Root == nil {
		return fmt.Errorf("fixture %q: root is required", fx.Name)
	}
	if err := validateFixtureNode(fx.Root); err != nil {
		return fmt.Errorf("fixture %q: %w", fx.Name, err)
	}
	if len(fx.Expected) == 0 {
		return fmt.Errorf("fixture %q: expected boxes are required", fx.Name)
	}
	return nil
}

func validateFixtureNode(fn *FixtureNode) error {
	if fn == nil {
		return fmt.Errorf("nil child node")
	}
	if _, err := parseDirection(fn.Direction); err != nil {
		return err
	}
	if _, err := parseBasisMode(fn.BasisMode); err != nil {
		return err
	}
	if fn.Grow < 0 {
		return fmt.Errorf("node %q: grow must be >= 0, got %v", fn.Label, fn.Grow)
	}
	if fn.Shrink < 0 {
		return fmt.Errorf("node %q: shrink must be >= 0, got %v", fn.Label, fn.Shrink)
	}
	if fn.Basis < 0 {
		return fmt.Errorf("node %q: basis must be >= 0, got %d", fn.Label, fn.Basis)
	}
	if fn.Intrinsic != nil && (fn.Intrinsic.W < 0 || fn.Intrinsic.H < 0) {
		return fmt.Errorf("node %q: intrinsic must be >= 0, got %dx%d", fn.Label, fn.Intrinsic.W, fn.Intrinsic.H)
	}
	for _, child := range fn.Children {
		if err := validateFixtureNode(child); err != nil {
			return err
		}
	}
	return nil
}

// BuildTree converts the FixtureNode tree into a FlexNode tree, also
// returning a label → *FlexNode index for box mapping.
//
// Returns an error on duplicate labels (T-10 go-reviewer MED-1: prior
// version panicked from an unexported helper, reachable from this
// exported method; converted to error return for CLAUDE rule 5 / 7).
func (fn *FixtureNode) BuildTree() (*FlexNode, map[string]*FlexNode, error) {
	if fn == nil {
		return nil, nil, fmt.Errorf("fixture: root is nil")
	}
	index := make(map[string]*FlexNode)
	root, err := buildNode(fn, index)
	if err != nil {
		return nil, nil, err
	}
	return root, index, nil
}

func buildNode(fn *FixtureNode, index map[string]*FlexNode) (*FlexNode, error) {
	if fn == nil {
		return nil, fmt.Errorf("fixture: nil child node")
	}
	dir, err := parseDirection(fn.Direction)
	if err != nil {
		return nil, err
	}
	basisMode, err := parseBasisMode(fn.BasisMode)
	if err != nil {
		return nil, err
	}
	if fn.Grow < 0 {
		return nil, fmt.Errorf("fixture node %q: grow must be >= 0, got %v", fn.Label, fn.Grow)
	}
	if fn.Shrink < 0 {
		return nil, fmt.Errorf("fixture node %q: shrink must be >= 0, got %v", fn.Label, fn.Shrink)
	}
	if fn.Basis < 0 {
		return nil, fmt.Errorf("fixture node %q: basis must be >= 0, got %d", fn.Label, fn.Basis)
	}
	node := &FlexNode{
		Direction: dir,
		Grow:      fn.Grow,
		Shrink:    fn.Shrink,
		Basis:     fn.Basis,
		BasisMode: basisMode,
	}
	if fn.Intrinsic != nil {
		w, h := fn.Intrinsic.W, fn.Intrinsic.H
		if w < 0 || h < 0 {
			return nil, fmt.Errorf("fixture node %q: intrinsic must be >= 0, got %dx%d", fn.Label, w, h)
		}
		node.Intrinsic = func() (int, int) { return w, h }
	}
	for _, c := range fn.Children {
		child, err := buildNode(c, index)
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, child)
	}
	if fn.Label != "" {
		if _, dup := index[fn.Label]; dup {
			return nil, fmt.Errorf("fixture: duplicate label %q", fn.Label)
		}
		index[fn.Label] = node
	}
	return node, nil
}

func parseDirection(s string) (Direction, error) {
	switch s {
	case "", "row", "Row":
		return Row, nil
	case "column", "Column":
		return Column, nil
	default:
		return Row, fmt.Errorf("fixture: invalid direction %q", s)
	}
}

func parseBasisMode(s string) (BasisMode, error) {
	switch s {
	case "", "auto", "Auto":
		return BasisAuto, nil
	case "fixed", "Fixed":
		return BasisFixed, nil
	default:
		return BasisAuto, fmt.Errorf("fixture: invalid basis_mode %q", s)
	}
}

// ViewportBox returns the viewport as a layout.Box (origin 0, 0).
func (fv FixtureViewport) ViewportBox() layout.Box {
	return layout.Box{X: 0, Y: 0, Width: fv.Width, Height: fv.Height}
}

// ToBox converts FixtureBox → layout.Box.
func (fb FixtureBox) ToBox() layout.Box {
	return layout.Box{X: fb.X, Y: fb.Y, Width: fb.W, Height: fb.H}
}
