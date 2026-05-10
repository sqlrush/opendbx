// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// SettingSource — provenance of each Config field's final value.
//
// Per user R3 Q1 decision: opendbx adopts CC `SettingSource` 1:1 naming
// (UserSettings / ProjectSettings / LocalSettings / FlagSettings /
// PolicySettings) + opendbx-specific extensions (ENV / CLI flag).
//
// Override chain (low → high priority; later overrides earlier):
//
//	SourceDefault → SourcePolicySettings → SourceUserSettings →
//	SourceProjectSettings → SourceLocalSettings → SourceFlagSettings →
//	SourceENV → SourceCLIFlag

package config

// SettingSource enumerates the provenance of a Config field's final value.
// Order in the const block IS the override priority (later = higher).
type SettingSource int

const (
	// SourceDefault — the field has not been written by any source above
	// defaults; value comes from Default().
	SourceDefault SettingSource = iota

	// SourcePolicySettings — admin / org policy (e.g. /etc/opendbx/managed.yaml
	// or remote-managed settings). Highest TIER but **lowest priority** in
	// override chain (per CC: policy can be amended by user/project/local).
	// Stage 0+: yaml file at platform-specific managed path (D-5).
	SourcePolicySettings

	// SourceUserSettings — Tier 1 user-global config: ~/.opendbx/config.yaml
	// (or $XDG_CONFIG_HOME/opendbx/config.yaml on Linux).
	SourceUserSettings

	// SourceProjectSettings — Tier 2 project-shared config: ./.opendbx/config.yaml
	// (committed to repo).
	SourceProjectSettings

	// SourceLocalSettings — Tier 2 project-local config: ./.opendbx/local.yaml
	// (gitignored — personal overrides per checkout).
	SourceLocalSettings

	// SourceENV — opendbx-specific. OPENDBX_* environment variables. Per spec
	// § 1.1 D-2 override chain: Local < ENV < --settings < CLI.
	SourceENV

	// SourceFlagSettings — settings supplied via `--settings <file-or-json>`
	// CLI flag. Beats ENV per spec.
	SourceFlagSettings

	// SourceCLIFlag — opendbx-specific. Other CLI flags from cmd/opendbx
	// (e.g. --output-format / --debug / --model). Highest priority.
	SourceCLIFlag
)

// String returns the canonical name (used by `admin config sources` output).
func (s SettingSource) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourcePolicySettings:
		return "policy"
	case SourceUserSettings:
		return "user"
	case SourceProjectSettings:
		return "project"
	case SourceLocalSettings:
		return "local"
	case SourceFlagSettings:
		return "flag-settings"
	case SourceENV:
		return "env"
	case SourceCLIFlag:
		return "cli"
	default:
		return "unknown"
	}
}

// AllSources returns the chain in override-priority order.
func AllSources() []SettingSource {
	return []SettingSource{
		SourceDefault,
		SourcePolicySettings,
		SourceUserSettings,
		SourceProjectSettings,
		SourceLocalSettings,
		SourceENV,
		SourceFlagSettings,
		SourceCLIFlag,
	}
}
