// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureNode is the JSON-decoded form of a FlexNode for CC UI sample
// golden tests. Labels uniquely identify each node for golden-box
// mapping; pointer identity is lost across JSON load.
type fixtureNode struct {
	Label     string         `json:"label"`
	Direction string         `json:"direction,omitempty"` // "row" | "column" | ""
	Grow      float64        `json:"grow,omitempty"`
	Shrink    float64        `json:"shrink,omitempty"`
	Basis     int            `json:"basis,omitempty"`
	BasisMode string         `json:"basis_mode,omitempty"` // "fixed" | "auto" | ""
	Justify   string         `json:"justify,omitempty"`    // "start"|"center"|"end"|"between"|"around"
	Align     string         `json:"align,omitempty"`      // "stretch"|"start"|"center"|"end"
	Intrinsic *intrinsicSize `json:"intrinsic,omitempty"`
	Children  []*fixtureNode `json:"children,omitempty"`
}

type intrinsicSize struct {
	W int `json:"w"`
	H int `json:"h"`
}

type fixtureViewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type fixtureBox struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type fixtureSource struct {
	Path string `json:"path"`
	Line string `json:"line,omitempty"`
	Note string `json:"note,omitempty"`
}

// fixture is a complete CC UI sample with input + expected output.
// Provenance fields (cc_commit / sources / artifact) are required so
// every golden fixture can be re-checked against Claude Code.
type fixture struct {
	Name     string                `json:"name"`
	Critical bool                  `json:"critical"`
	Note     string                `json:"note,omitempty"`
	CCCommit string                `json:"cc_commit"`
	Sources  []fixtureSource       `json:"sources"`
	Artifact string                `json:"artifact"`
	Viewport fixtureViewport       `json:"viewport"`
	Root     *fixtureNode          `json:"root"`
	Expected map[string]fixtureBox `json:"expected"`
}

func loadFixture(path string) (*fixture, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open fixture %q: %w", path, err)
	}
	defer f.Close()
	var fx fixture
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fx); err != nil {
		return nil, fmt.Errorf("parse fixture %q: %w", path, err)
	}
	if err := fx.validate(); err != nil {
		return nil, fmt.Errorf("validate fixture %q: %w", path, err)
	}
	return &fx, nil
}

func (fx *fixture) validate() error {
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

func validateFixtureNode(fn *fixtureNode) error {
	if fn == nil {
		return fmt.Errorf("nil child node")
	}
	if _, err := parseDirection(fn.Direction); err != nil {
		return err
	}
	if _, err := parseBasisMode(fn.BasisMode); err != nil {
		return err
	}
	if _, err := parseJustify(fn.Justify); err != nil {
		return err
	}
	if _, err := parseAlign(fn.Align); err != nil {
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

// buildTree converts the fixtureNode tree into a FlexNode tree and a
// label → *FlexNode index for box mapping. Returns an error on
// duplicate labels.
func (fn *fixtureNode) buildTree() (*FlexNode, map[string]*FlexNode, error) {
	if fn == nil {
		return nil, nil, fmt.Errorf("fixture: root is nil")
	}
	index := make(map[string]*FlexNode)
	root, err := buildFixtureNode(fn, index)
	if err != nil {
		return nil, nil, err
	}
	return root, index, nil
}

func buildFixtureNode(fn *fixtureNode, index map[string]*FlexNode) (*FlexNode, error) {
	if fn == nil {
		return nil, fmt.Errorf("fixture: nil child node")
	}
	dir, _ := parseDirection(fn.Direction)
	basisMode, _ := parseBasisMode(fn.BasisMode)
	justify, _ := parseJustify(fn.Justify)
	align, _ := parseAlign(fn.Align)
	node := &FlexNode{
		Direction: dir,
		Grow:      fn.Grow,
		Shrink:    fn.Shrink,
		Basis:     fn.Basis,
		BasisMode: basisMode,
		Justify:   justify,
		Align:     align,
	}
	if fn.Intrinsic != nil {
		w, h := fn.Intrinsic.W, fn.Intrinsic.H
		node.Measure = func() (int, int) { return w, h }
	}
	for _, c := range fn.Children {
		child, err := buildFixtureNode(c, index)
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
	switch strings.ToLower(s) {
	case "", "row":
		return Row, nil
	case "column":
		return Column, nil
	default:
		return Row, fmt.Errorf("fixture: invalid direction %q", s)
	}
}

func parseBasisMode(s string) (BasisMode, error) {
	switch strings.ToLower(s) {
	case "", "auto":
		return BasisAuto, nil
	case "fixed":
		return BasisFixed, nil
	default:
		return BasisAuto, fmt.Errorf("fixture: invalid basis_mode %q", s)
	}
}

func parseJustify(s string) (Justify, error) {
	switch strings.ToLower(s) {
	case "", "start":
		return JustifyStart, nil
	case "center":
		return JustifyCenter, nil
	case "end":
		return JustifyEnd, nil
	case "between", "space-between":
		return JustifySpaceBetween, nil
	case "around", "space-around":
		return JustifySpaceAround, nil
	default:
		return JustifyStart, fmt.Errorf("fixture: invalid justify %q", s)
	}
}

func parseAlign(s string) (Align, error) {
	switch strings.ToLower(s) {
	case "", "stretch":
		return AlignStretch, nil
	case "start":
		return AlignStart, nil
	case "center":
		return AlignCenter, nil
	case "end":
		return AlignEnd, nil
	default:
		return AlignStretch, fmt.Errorf("fixture: invalid align %q", s)
	}
}

func (fv fixtureViewport) viewportBox() Box {
	return Box{X: 0, Y: 0, Width: fv.Width, Height: fv.Height}
}

func (fb fixtureBox) toBox() Box {
	return Box{X: fb.X, Y: fb.Y, Width: fb.W, Height: fb.H}
}

// writeFixtureForTest writes a JSON fixture body to a temp file and
// returns its path. Used by fixture-parser negative tests.
func writeFixtureForTest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}
