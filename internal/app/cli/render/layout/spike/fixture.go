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

// Fixture is a complete CC UI sample with input + expected output.
type Fixture struct {
	Name     string                `json:"name"`
	Critical bool                  `json:"critical"`
	Note     string                `json:"note,omitempty"`
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
	return &fx, nil
}

// BuildTree converts the FixtureNode tree into a FlexNode tree, also
// returning a label → *FlexNode index for box mapping.
//
// Returns an error on duplicate labels (T-10 go-reviewer MED-1: prior
// version panicked from an unexported helper, reachable from this
// exported method; converted to error return for CLAUDE rule 5 / 7).
func (fn *FixtureNode) BuildTree() (*FlexNode, map[string]*FlexNode, error) {
	index := make(map[string]*FlexNode)
	root, err := buildNode(fn, index)
	if err != nil {
		return nil, nil, err
	}
	return root, index, nil
}

func buildNode(fn *FixtureNode, index map[string]*FlexNode) (*FlexNode, error) {
	node := &FlexNode{
		Direction: parseDirection(fn.Direction),
		Grow:      fn.Grow,
		Shrink:    fn.Shrink,
		Basis:     fn.Basis,
		BasisMode: parseBasisMode(fn.BasisMode),
	}
	if fn.Intrinsic != nil {
		w, h := fn.Intrinsic.W, fn.Intrinsic.H
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

func parseDirection(s string) Direction {
	switch s {
	case "column", "Column":
		return Column
	default:
		return Row
	}
}

func parseBasisMode(s string) BasisMode {
	switch s {
	case "fixed", "Fixed":
		return BasisFixed
	default:
		return BasisAuto
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
