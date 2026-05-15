// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package visualgolden

// Theme bundles the freeze rendering config. Defaults map to spec-
// 0.11.5 reference theme: JetBrains Mono 14pt + Noto CJK fallback,
// black background, 16px padding. CI installs the apt packages
// fonts-jetbrains-mono + fonts-noto-cjk to ensure deterministic font
// fallback.
type Theme struct {
	FontFamily string
	FontSize   int
	Background string
	Padding    int
}

// DefaultTheme returns the spec-0.11.5 reference theme.
func DefaultTheme() Theme {
	return Theme{
		FontFamily: "JetBrains Mono",
		FontSize:   14,
		Background: "#000000",
		Padding:    16,
	}
}
