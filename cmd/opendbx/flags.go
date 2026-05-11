// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Flag registration + optionSpec table (spec-0.3 D-3 + D-7).
//
// optionSpec is the single source of truth for every flag that cmd/opendbx
// exposes. Each entry records:
//   - the CC origin (file + line + the original CC description string)
//   - the adaptation Class (A/B/C/D — see spec-0.3 § 2.3)
//   - the adapted opendbx description string
//   - rationale for any deviation
//
// This keeps adaptation auditable: opendbrb/docs/cc-vs-opendbx-help-diff.md
// is a curated rendering of this table, NOT a hand-written essay.
//
// CC source reference: ~/claude-code-source-code/src/main.tsx L968-L1010
// Baseline captured: opendbrb/docs/cc-help-baseline-v2.1.138.txt

package main

import "github.com/spf13/cobra"

// adaptClass classifies how each CC option lands in opendbx.
type adaptClass string

const (
	classA adaptClass = "A" // direct 1:1
	classB adaptClass = "B" // CC name kept, DB-flavored description
	classC adaptClass = "C" // opendbx-replaced or NEW
	classD adaptClass = "D" // hidden (skip from help)
)

// registerFlags binds every CLI flag to the corresponding Options field on
// the root command.
//
// We do not iterate optionSpec to register flags via reflection — Go's flag
// API is cleanly typed, and 50 explicit pflag calls are easier to read than
// a reflect-driven generator. optionSpec exists for documentation and
// drift-checking (TestOptionSpecMatchesFlags in main_test.go ensures
// optionSpec stays in sync with this function).
func registerFlags(cmd *cobra.Command, opts *Options) {
	// --version + -v shorthand (manual; cobra's auto Version field doesn't
	// support shorthand). The flag is consumed in root.go RunE.
	cmd.Flags().BoolP("version", "v", false, "Output the version number")

	// --version-verbose (spec-0.7 D-4 / T-6) — opendbx-only multi-line
	// diagnostic block (Version + Commit + BuildDate + workdir + Go +
	// os/arch). Root-only Flag (not PersistentFlag) matching --version's
	// scope. No shorthand by design — `-V` reserved for future use; users
	// type the long form when they need the diagnostic detail. Precedence
	// when both --version and --version-verbose set: verbose wins (more
	// detailed; spec § 8 Q8 + § 1.1 D-4).
	cmd.Flags().Bool("version-verbose", false,
		"Output build version + commit + build date + Go runtime + os/arch (multi-line)")

	// === Class A: direct 1:1 ===

	// Debug
	cmd.PersistentFlags().StringVarP(&opts.Debug.Debug, "debug", "d", "",
		"Enable debug mode with optional category filtering (e.g., \"api,hooks\" or \"!1p,!file\")")
	// Naked `--debug` must NOT eat the next token as filter value (CC parity:
	// `claude --debug mcp` toggles debug + dispatches to mcp). NoOptDefVal=" "
	// marks --debug as "valid without =value"; downstream treats any non-empty
	// Debug as "enabled".
	cmd.PersistentFlags().Lookup("debug").NoOptDefVal = " "
	cmd.PersistentFlags().BoolVar(&opts.Debug.DebugToStderr, "debug-to-stderr", false,
		"Enable debug mode (to stderr)")
	if err := cmd.PersistentFlags().MarkHidden("debug-to-stderr"); err != nil {
		panic(err)
	}
	cmd.PersistentFlags().StringVar(&opts.Debug.DebugFile, "debug-file", "",
		"Write debug logs to a specific file path (implicitly enables debug mode)")
	cmd.PersistentFlags().BoolVar(&opts.Debug.Verbose, "verbose", false,
		"Override verbose mode setting from config")

	// Session
	cmd.PersistentFlags().BoolVarP(&opts.Session.Continue, "continue", "c", false,
		"Continue the most recent diagnosis session in the current directory")
	cmd.PersistentFlags().StringVarP(&opts.Session.Resume, "resume", "r", "",
		"Resume a session by session ID, or open interactive picker with optional search term")
	cmd.PersistentFlags().Lookup("resume").NoOptDefVal = " " // [value] optional
	cmd.PersistentFlags().BoolVar(&opts.Session.ForkSession, "fork-session", false,
		"When resuming, create a new session ID instead of reusing the original (use with --resume or --continue)")
	cmd.PersistentFlags().StringVar(&opts.Session.FromPR, "from-pr", "",
		"Resume a session linked to a PR by PR number/URL, or open interactive picker with optional search term")
	cmd.PersistentFlags().Lookup("from-pr").NoOptDefVal = " "
	cmd.PersistentFlags().BoolVar(&opts.Session.NoSessionPersist, "no-session-persistence", false,
		"Disable session persistence - sessions will not be saved to disk and cannot be resumed (only works with --print)")
	cmd.PersistentFlags().StringVar(&opts.Session.SessionID, "session-id", "",
		"Use a specific session ID for the conversation (must be a valid UUID)")
	cmd.PersistentFlags().StringVarP(&opts.Session.Name, "name", "n", "",
		"Set a display name for this session (shown in /resume picker and terminal title)")
	cmd.PersistentFlags().StringVar(&opts.Session.Prefill, "prefill", "",
		"Pre-fill the prompt input with text without submitting it")
	if err := cmd.PersistentFlags().MarkHidden("prefill"); err != nil {
		panic(err)
	}

	// Print
	cmd.PersistentFlags().BoolVarP(&opts.Print.Print, "print", "p", false,
		"Print response and exit (useful for pipes). Note: trust dialog skipped in non-interactive mode.")
	cmd.PersistentFlags().StringVar(&opts.Print.OutputFormat, "output-format", "",
		"Output format (only works with --print): \"text\" (default), \"json\" (single result), or \"stream-json\" (realtime streaming)")
	cmd.PersistentFlags().StringVar(&opts.Print.InputFormat, "input-format", "",
		"Input format (only works with --print): \"text\" (default), or \"stream-json\" (realtime streaming input)")
	cmd.PersistentFlags().BoolVar(&opts.Print.IncludeHookEvents, "include-hook-events", false,
		"Include all hook lifecycle events in the output stream (only works with --output-format=stream-json)")
	cmd.PersistentFlags().BoolVar(&opts.Print.IncludePartialMsgs, "include-partial-messages", false,
		"Include partial message chunks as they arrive (only works with --print and --output-format=stream-json)")
	cmd.PersistentFlags().StringVar(&opts.Print.JSONSchema, "json-schema", "",
		"JSON Schema for structured output validation. Example: {\"type\":\"object\",\"properties\":{\"name\":{\"type\":\"string\"}},\"required\":[\"name\"]}")
	cmd.PersistentFlags().BoolVar(&opts.Print.ReplayUserMessages, "replay-user-messages", false,
		"Re-emit user messages from stdin back on stdout for acknowledgment (only works with --input-format=stream-json and --output-format=stream-json)")
	cmd.PersistentFlags().Float64Var(&opts.Print.MaxBudgetUSD, "max-budget-usd", 0,
		"Maximum dollar amount to spend on API calls (only works with --print)")

	// Tools
	cmd.PersistentFlags().StringSliceVar(&opts.Tools.AllowedTools, "allowed-tools", nil,
		"Comma or space-separated list of tool names to allow (e.g. \"Bash(psql:*) Query\")")
	// CC camelCase alias for parity (hidden to avoid duplicating in --help).
	cmd.PersistentFlags().StringSliceVar(&opts.Tools.AllowedTools, "allowedTools", nil, "")
	if err := cmd.PersistentFlags().MarkHidden("allowedTools"); err != nil {
		panic(err)
	}
	cmd.PersistentFlags().StringSliceVar(&opts.Tools.DisallowedTools, "disallowed-tools", nil,
		"Comma or space-separated list of tool names to deny (e.g. \"Bash(psql:*) Query\")")
	cmd.PersistentFlags().StringSliceVar(&opts.Tools.DisallowedTools, "disallowedTools", nil, "")
	if err := cmd.PersistentFlags().MarkHidden("disallowedTools"); err != nil {
		panic(err)
	}
	cmd.PersistentFlags().StringSliceVar(&opts.Tools.Tools, "tools", nil,
		"Specify the list of available tools from the built-in set. Use \"\" to disable all tools, \"default\" for all, or specify names.")
	cmd.PersistentFlags().BoolVar(&opts.Tools.DisableSlash, "disable-slash-commands", false,
		"Disable all skills")

	// IO
	cmd.PersistentFlags().StringVar(&opts.IO.Settings, "settings", "",
		"Path to a settings JSON file or a JSON string to load additional settings from")
	cmd.PersistentFlags().StringSliceVar(&opts.IO.AddDir, "add-dir", nil,
		"Additional directories to allow tool access to")
	cmd.PersistentFlags().BoolVar(&opts.IO.IDE, "ide", false,
		"Automatically connect to IDE on startup if exactly one valid IDE is available")
	cmd.PersistentFlags().StringVar(&opts.IO.SystemPrompt, "system-prompt", "",
		"System prompt to use for the session")
	cmd.PersistentFlags().StringVar(&opts.IO.AppendSystem, "append-system-prompt", "",
		"Append a system prompt to the default system prompt")
	// Hidden file variants of the system prompt (per spec § 2.3 Class D).
	var sysPromptFile, appendSysPromptFile string
	cmd.PersistentFlags().StringVar(&sysPromptFile, "system-prompt-file", "",
		"Read system prompt from a file")
	cmd.PersistentFlags().StringVar(&appendSysPromptFile, "append-system-prompt-file", "",
		"Read system prompt from a file and append to the default system prompt")
	for _, name := range []string{"system-prompt-file", "append-system-prompt-file"} {
		if err := cmd.PersistentFlags().MarkHidden(name); err != nil {
			panic(err)
		}
	}
	cmd.PersistentFlags().StringVar(&opts.IO.SettingSources, "setting-sources", "",
		"Comma-separated list of setting sources to load (user, project, local).")
	cmd.PersistentFlags().StringSliceVar(&opts.IO.PluginDir, "plugin-dir", nil,
		"Load plugins from a directory for this session only (repeatable)")
	cmd.PersistentFlags().StringSliceVar(&opts.IO.File, "file", nil,
		"File resources to download at startup. Format: file_id:relative_path")
	cmd.PersistentFlags().BoolVar(&opts.IO.Bare, "bare", false,
		"Minimal mode: skip hooks, plugin sync, auto-memory, background prefetches, and CLAUDE.md auto-discovery. Sets OPENDBX_SIMPLE=1.")
	cmd.PersistentFlags().StringVar(&opts.IO.PermissionMode, "permission-mode", "",
		"Permission mode to use for the session")
	cmd.PersistentFlags().BoolVar(&opts.IO.DangerouslySkip, "dangerously-skip-permissions", false,
		"Bypass all permission checks. Recommended only for sandboxes with no internet access.")
	cmd.PersistentFlags().BoolVar(&opts.IO.AllowDangerous, "allow-dangerously-skip-permissions", false,
		"Enable bypassing all permission checks as an option, without it being enabled by default.")

	// === Class B: CC name kept, DB-flavored description ===

	cmd.PersistentFlags().StringVar(&opts.Model.Model, "model", "",
		"Model for the current diagnosis session. Provide an alias (e.g. 'sonnet' or 'opus') or a full model name.")
	cmd.PersistentFlags().StringVar(&opts.Model.Agent, "agent", "",
		"Diagnosis agent profile (overrides 'agent' setting in config).")
	cmd.PersistentFlags().StringVar(&opts.Model.FallbackModel, "fallback-model", "",
		"Enable automatic fallback to specified model when default model is overloaded (only works with --print)")
	cmd.PersistentFlags().StringVar(&opts.Model.Effort, "effort", "",
		"Effort level for the current diagnosis session (low, medium, high, max)")

	cmd.PersistentFlags().StringSliceVar(&opts.MCP.MCPConfig, "mcp-config", nil,
		"Load MCP servers from JSON files or strings (space-separated). DB-related MCP servers can be configured here.")
	cmd.PersistentFlags().BoolVar(&opts.MCP.StrictMCPConfig, "strict-mcp-config", false,
		"Only use MCP servers from --mcp-config, ignoring all other MCP configurations")

	// === Class C: opendbx-specific NEW flags (per D5: --llm-tier semantically independent of --model) ===

	cmd.PersistentFlags().StringVar(&opts.DB.DB, "db", "",
		"Database type for the session: postgres (MVP), mysql/oracle/opengauss (Stage 6+ reserved)")
	cmd.PersistentFlags().StringVar(&opts.DB.Connection, "connection", "",
		"Database connection DSN, e.g. \"postgres://user:pass@host:5432/dbname\". Mutually exclusive with --connection-alias.")
	cmd.PersistentFlags().StringVar(&opts.DB.ConnectionAlias, "connection-alias", "",
		"Database connection alias from 'opendbx db list'. Mutually exclusive with --connection.")
	cmd.PersistentFlags().StringVar(&opts.Model.LLMTier, "llm-tier", "",
		"LLM model tier (strategy layer, semantically independent of --model): tier-1 (Opus primary) / tier-2 (glm-5 backup) / tier-3 (deepseek deep-dive) / tier-4 (local). tier→model mapping resolved from config.")

	// === Class D: hidden flags (kept for compatibility, not in --help) ===

	cmd.PersistentFlags().BoolVar(&opts.Hidden.Init, "init", false, "Run Setup hooks with init trigger, then continue")
	cmd.PersistentFlags().BoolVar(&opts.Hidden.InitOnly, "init-only", false, "Run Setup and SessionStart:startup hooks, then exit")
	cmd.PersistentFlags().BoolVar(&opts.Hidden.Maintenance, "maintenance", false, "Run Setup hooks with maintenance trigger, then continue")
	cmd.PersistentFlags().StringVar(&opts.Hidden.Thinking, "thinking", "",
		"Thinking mode: enabled (equivalent to adaptive), disabled")
	for _, name := range []string{"init", "init-only", "maintenance", "thinking"} {
		if err := cmd.PersistentFlags().MarkHidden(name); err != nil {
			panic(err)
		}
	}
}

// optionSpecRow records one CLI flag's adaptation decision.
//
// spec-0.4 D-12 (R3): R2 had only Name/Short/Class/Hidden. R3 expands to 8
// fields: + CCRef (CC main.tsx file:line) + CCDesc (CC verbatim description)
// + OdxDesc (opendbx adapted description) + Reason (≥ 1 sentence rationale).
//
// Used as single-source-of-truth audit table; TestAllFlagsInOptionSpec +
// TestOptionSpecsHaveCCRef + TestOptionSpecMatchesFlags enforce sync with
// the cobra registration in registerFlags().
type optionSpecRow struct {
	Name    string     // pflag long name (no leading --)
	Short   string     // shorthand (single letter, "" if none)
	Class   adaptClass // A/B/C/D
	Hidden  bool       // true if MarkHidden in registerFlags
	CCRef   string     // CC main.tsx file:line (e.g. "main.tsx:L988"); empty for opendbx-specific NEW flags
	CCDesc  string     // CC verbatim description (when adaptation is ★A 1:1)
	OdxDesc string     // opendbx adapted description (when class B/C and wording differs)
	Reason  string     // adaptation rationale, ≥ 1 sentence
}

// optionSpecs is the curated adaptation table. Validated by 3 tests:
//   - TestAllFlagsInOptionSpec: every cobra flag exists in this table
//   - TestOptionSpecsHaveCCRef:  every Class A/B row has non-empty CCRef
//   - TestOptionSpecMatchesFlags: every table row resolves via cobra Lookup
var optionSpecs = []optionSpecRow{
	// === --version + -v handled separately (manual) ===
	{Name: "version", Short: "v", Class: classA, CCRef: "main.tsx:L3808",
		CCDesc: "Output the version number",
		Reason: "1:1 — opendbx version semantic identical to CC",
	},
	{Name: "version-verbose", Class: classC, CCRef: "",
		CCDesc: "",
		Reason: "opendbx-only — CC has no equivalent; diagnostic block for issue reports " +
			"(spec-0.7 D-4 T-6; cc-vs-opendbx-help-diff.md \"opendbx-only --version-verbose\")",
	},

	// === Class A: direct 1:1 ===
	{Name: "debug", Short: "d", Class: classA, CCRef: "main.tsx:L972",
		CCDesc: "Enable debug mode with optional category filtering (e.g., \"api,hooks\" or \"!1p,!file\")",
		Reason: "1:1 — debug categories share spec-0.5 logger taxonomy; NoOptDefVal added per spec-0.3 hotfix"},
	{Name: "debug-to-stderr", Class: classA, Hidden: true, CCRef: "main.tsx:L976",
		CCDesc: "Enable debug mode (to stderr)", Reason: "1:1 hidden alias"},
	{Name: "debug-file", Class: classA, CCRef: "main.tsx:L977",
		CCDesc: "Write debug logs to a specific file path (implicitly enables debug mode)",
		Reason: "1:1 — same file-path semantics"},
	{Name: "verbose", Class: classA, CCRef: "main.tsx:L978",
		CCDesc: "Override verbose mode setting from config", Reason: "1:1 — opendbx config has same verbose semantic"},
	{Name: "continue", Short: "c", Class: classA, CCRef: "main.tsx:L1000",
		CCDesc:  "Continue the most recent conversation in the current directory",
		OdxDesc: "Continue the most recent diagnosis session in the current directory",
		Reason:  "Class A; 'conversation' → 'diagnosis session' (DB context)"},
	{Name: "resume", Short: "r", Class: classA, CCRef: "main.tsx:L1001",
		CCDesc:  "Resume a conversation by session ID, or open interactive picker with optional search term",
		OdxDesc: "Resume a session by session ID, or open interactive picker with optional search term",
		Reason:  "Class A; 'conversation' dropped"},
	{Name: "fork-session", Class: classA, CCRef: "main.tsx:L1002",
		CCDesc: "When resuming, create a new session ID instead of reusing the original (use with --resume or --continue)", Reason: "1:1"},
	{Name: "from-pr", Class: classA, CCRef: "main.tsx:L1006",
		CCDesc: "Resume a session linked to a PR by PR number/URL, or open interactive picker with optional search term", Reason: "1:1 — PR resume is generic"},
	{Name: "no-session-persistence", Class: classA, CCRef: "main.tsx:L1007",
		CCDesc: "Disable session persistence - sessions will not be saved to disk and cannot be resumed (only works with --print)", Reason: "1:1"},
	{Name: "session-id", Class: classA, CCRef: "main.tsx:L1008",
		CCDesc: "Use a specific session ID for the conversation (must be a valid UUID)", Reason: "1:1 — UUID semantic identical"},
	{Name: "name", Short: "n", Class: classA, CCRef: "main.tsx:L1009",
		CCDesc: "Set a display name for this session (shown in /resume picker and terminal title)", Reason: "1:1"},
	{Name: "prefill", Class: classA, Hidden: true, CCRef: "main.tsx:L1003",
		CCDesc: "Pre-fill the prompt input with text without submitting it", Reason: "1:1 hidden"},
	{Name: "print", Short: "p", Class: classA, CCRef: "main.tsx:L984",
		CCDesc: "Print response and exit (useful for pipes). Note: trust dialog skipped in non-interactive mode.", Reason: "1:1 — pipe friendly"},
	{Name: "output-format", Class: classA, CCRef: "main.tsx:L989",
		CCDesc: "Output format (only works with --print): \"text\" (default), \"json\" (single result), or \"stream-json\" (realtime streaming)", Reason: "1:1 — same enum"},
	{Name: "input-format", Class: classA, CCRef: "main.tsx:L991",
		CCDesc: "Input format (only works with --print): \"text\" (default), or \"stream-json\" (realtime streaming input)", Reason: "1:1"},
	{Name: "include-hook-events", Class: classA, CCRef: "main.tsx:L992", CCDesc: "Include all hook lifecycle events in the output stream (only works with --output-format=stream-json)", Reason: "1:1"},
	{Name: "include-partial-messages", Class: classA, CCRef: "main.tsx:L993", CCDesc: "Include partial message chunks as they arrive (only works with --print and --output-format=stream-json)", Reason: "1:1"},
	{Name: "json-schema", Class: classA, CCRef: "main.tsx:L990", CCDesc: "JSON Schema for structured output validation.", Reason: "1:1 — JSON Schema generic"},
	{Name: "replay-user-messages", Class: classA, CCRef: "main.tsx:L997", CCDesc: "Re-emit user messages from stdin back on stdout for acknowledgment (only works with --input-format=stream-json and --output-format=stream-json)", Reason: "1:1"},
	{Name: "max-budget-usd", Class: classA, CCRef: "main.tsx:L1010", CCDesc: "Maximum dollar amount to spend on API calls (only works with --print)", Reason: "1:1 — opendbx LLM tier billing same shape"},
	{Name: "allowed-tools", Class: classA, CCRef: "main.tsx:L988",
		CCDesc:  "Comma or space-separated list of tool names to allow (e.g. \"Bash(git:*) Edit\")",
		OdxDesc: "Comma or space-separated list of tool names to allow (e.g. \"Bash(psql:*) Query\")",
		Reason:  "Class A; example uses 'Bash(psql:*) Query' for DB context (spec-0.3 D-3 catalog)"},
	{Name: "allowedTools", Class: classA, Hidden: true, CCRef: "main.tsx:L988",
		CCDesc: "(camelCase alias of --allowed-tools)", Reason: "CC parity — both names accepted, hidden in --help"},
	{Name: "disallowed-tools", Class: classA, CCRef: "main.tsx:L988",
		CCDesc:  "Comma or space-separated list of tool names to deny (e.g. \"Bash(git:*) Edit\")",
		OdxDesc: "Comma or space-separated list of tool names to deny (e.g. \"Bash(psql:*) Query\")",
		Reason:  "Class A; same example adaptation as --allowed-tools"},
	{Name: "disallowedTools", Class: classA, Hidden: true, CCRef: "main.tsx:L988",
		CCDesc: "(camelCase alias of --disallowed-tools)", Reason: "CC parity"},
	{Name: "tools", Class: classA, CCRef: "main.tsx:L988", CCDesc: "Specify the list of available tools from the built-in set. Use \"\" to disable all tools, \"default\" to use all tools, or specify tool names.", Reason: "1:1"},
	{Name: "disable-slash-commands", Class: classA, CCRef: "main.tsx:L1011", CCDesc: "Disable all skills", Reason: "1:1 — opendbx 'skill' = CC 'skill'"},
	{Name: "settings", Class: classA, CCRef: "main.tsx:L1009",
		CCDesc:  "Path to a settings JSON file or a JSON string to load additional settings from",
		OdxDesc: "Path to a settings YAML/JSON file or a JSON string to load additional settings from",
		Reason:  "Class A; YAML accepted in addition to JSON (spec-0.4 § 1.4 yaml.v3 default + JSON-as-YAML-subset)"},
	{Name: "add-dir", Class: classA, CCRef: "main.tsx:L1009", CCDesc: "Additional directories to allow tool access to", Reason: "1:1"},
	{Name: "ide", Class: classA, CCRef: "main.tsx:L1009", CCDesc: "Automatically connect to IDE on startup if exactly one valid IDE is available", Reason: "1:1 — opendbx IDE plugin shape"},
	{Name: "system-prompt", Class: classA, CCRef: "main.tsx:L996", CCDesc: "System prompt to use for the session", Reason: "1:1"},
	{Name: "append-system-prompt", Class: classA, CCRef: "main.tsx:L996", CCDesc: "Append a system prompt to the default system prompt", Reason: "1:1"},
	{Name: "system-prompt-file", Class: classA, Hidden: true, CCRef: "main.tsx:L996", CCDesc: "Read system prompt from a file", Reason: "1:1 hidden file variant"},
	{Name: "append-system-prompt-file", Class: classA, Hidden: true, CCRef: "main.tsx:L996", CCDesc: "Read system prompt from a file and append to the default system prompt", Reason: "1:1 hidden"},
	{Name: "setting-sources", Class: classA, CCRef: "main.tsx:L1009", CCDesc: "Comma-separated list of setting sources to load (user, project, local).", Reason: "1:1 — sources match opendbx Q1 ★B SettingSource naming"},
	{Name: "plugin-dir", Class: classA, CCRef: "main.tsx:L1006", CCDesc: "Load plugins from a directory for this session only (repeatable)", Reason: "1:1 — plugin/Skill semantics aligned"},
	{Name: "file", Class: classA, CCRef: "main.tsx:L1006", CCDesc: "File resources to download at startup. Format: file_id:relative_path", Reason: "1:1"},
	{Name: "bare", Class: classA, CCRef: "main.tsx:L985",
		CCDesc:  "Minimal mode: skip hooks, LSP, plugin sync, attribution, auto-memory, background prefetches, keychain reads, and CLAUDE.md auto-discovery. Sets CLAUDE_CODE_SIMPLE=1.",
		OdxDesc: "Minimal mode: skip hooks, plugin sync, auto-memory, background prefetches, and CLAUDE.md auto-discovery. Sets OPENDBX_SIMPLE=1.",
		Reason:  "Class A; env var rename CLAUDE_CODE_SIMPLE → OPENDBX_SIMPLE; LSP/keychain/attribution dropped (not applicable)"},
	{Name: "permission-mode", Class: classA, CCRef: "main.tsx:L999", CCDesc: "Permission mode to use for the session", Reason: "1:1 — same enum"},
	{Name: "dangerously-skip-permissions", Class: classA, CCRef: "main.tsx:L995", CCDesc: "Bypass all permission checks. Recommended only for sandboxes with no internet access.", Reason: "1:1"},
	{Name: "allow-dangerously-skip-permissions", Class: classA, CCRef: "main.tsx:L995", CCDesc: "Enable bypassing all permission checks as an option, without it being enabled by default.", Reason: "1:1"},

	// === Class B: CC name kept, DB-flavored description ===
	{Name: "model", Class: classB, CCRef: "main.tsx:L1010",
		CCDesc:  "Model for the current session.",
		OdxDesc: "Model for the current diagnosis session.",
		Reason:  "Class B — DB diagnosis context"},
	{Name: "agent", Class: classB, CCRef: "main.tsx:L1010",
		CCDesc:  "Agent for the current session. Overrides the 'agent' setting.",
		OdxDesc: "Diagnosis agent profile (overrides 'agent' setting in config).",
		Reason:  "Class B — DB diagnosis context"},
	{Name: "fallback-model", Class: classB, CCRef: "main.tsx:L1010",
		CCDesc: "Enable automatic fallback to specified model when default model is overloaded (only works with --print)",
		Reason: "Class B — same fallback behavior; tier-based fallback also configurable via --llm-tier (spec-0.4 D-1)"},
	{Name: "effort", Class: classB, CCRef: "main.tsx:L1010",
		CCDesc:  "Effort level for the current session (low, medium, high, max)",
		OdxDesc: "Effort level for the current diagnosis session (low, medium, high, max)",
		Reason:  "Class B — DB diagnosis context"},
	{Name: "mcp-config", Class: classB, CCRef: "main.tsx:L1006",
		CCDesc:  "Load MCP servers from JSON files or strings (space-separated)",
		OdxDesc: "Load MCP servers from JSON files or strings (space-separated). DB-related MCP servers can be configured here.",
		Reason:  "Class B — DB-relevant note"},
	{Name: "strict-mcp-config", Class: classB, CCRef: "main.tsx:L1006",
		CCDesc: "Only use MCP servers from --mcp-config, ignoring all other MCP configurations",
		Reason: "1:1 — opendbx mcp config supports same strict mode"},

	// === Class C: opendbx-specific NEW ===
	{Name: "db", Class: classC,
		OdxDesc: "Database type for the session: postgres (MVP), mysql/oracle/opengauss (Stage 6+ reserved)",
		Reason:  "NEW — opendbx-specific DB type selector (no CC equivalent)"},
	{Name: "connection", Class: classC,
		OdxDesc: "Database connection DSN, e.g. \"postgres://user:pass@host:5432/dbname\". Mutually exclusive with --connection-alias.",
		Reason:  "NEW — opendbx-specific DB connection (no CC equivalent)"},
	{Name: "connection-alias", Class: classC,
		OdxDesc: "Database connection alias from 'opendbx db list'. Mutually exclusive with --connection.",
		Reason:  "NEW — alias resolution for connection-config (spec-1.19)"},
	{Name: "llm-tier", Class: classC,
		OdxDesc: "LLM model tier (strategy layer, semantically independent of --model): tier-1 / tier-2 / tier-3 / tier-4. tier→model mapping resolved from config.",
		Reason:  "NEW — opendbx tier strategy (user R3 Q4 D5: independent of --model)"},

	// === Class D: hidden ===
	{Name: "init", Class: classD, Hidden: true, CCRef: "main.tsx:L985", CCDesc: "Run Setup hooks with init trigger, then continue", Reason: "Class D — hidden, kept for compatibility"},
	{Name: "init-only", Class: classD, Hidden: true, CCRef: "main.tsx:L985", CCDesc: "Run Setup and SessionStart:startup hooks, then exit", Reason: "Class D — hidden"},
	{Name: "maintenance", Class: classD, Hidden: true, CCRef: "main.tsx:L985", CCDesc: "Run Setup hooks with maintenance trigger, then continue", Reason: "Class D — hidden"},
	{Name: "thinking", Class: classD, Hidden: true, CCRef: "main.tsx:L998", CCDesc: "Thinking mode: enabled (equivalent to adaptive), disabled", Reason: "Class D — hidden, deprecated; use --effort instead"},
}
