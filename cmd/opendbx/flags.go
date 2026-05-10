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

	// === Class A: direct 1:1 ===

	// Debug
	cmd.PersistentFlags().StringVarP(&opts.Debug.Debug, "debug", "d", "",
		"Enable debug mode with optional category filtering (e.g., \"api,hooks\" or \"!1p,!file\")")
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
	cmd.Flags().BoolVarP(&opts.Session.Continue, "continue", "c", false,
		"Continue the most recent diagnosis session in the current directory")
	cmd.Flags().StringVarP(&opts.Session.Resume, "resume", "r", "",
		"Resume a session by session ID, or open interactive picker with optional search term")
	cmd.Flags().Lookup("resume").NoOptDefVal = " " // [value] optional
	cmd.Flags().BoolVar(&opts.Session.ForkSession, "fork-session", false,
		"When resuming, create a new session ID instead of reusing the original (use with --resume or --continue)")
	cmd.Flags().StringVar(&opts.Session.FromPR, "from-pr", "",
		"Resume a session linked to a PR by PR number/URL, or open interactive picker with optional search term")
	cmd.Flags().Lookup("from-pr").NoOptDefVal = " "
	cmd.Flags().BoolVar(&opts.Session.NoSessionPersist, "no-session-persistence", false,
		"Disable session persistence - sessions will not be saved to disk and cannot be resumed (only works with --print)")
	cmd.Flags().StringVar(&opts.Session.SessionID, "session-id", "",
		"Use a specific session ID for the conversation (must be a valid UUID)")
	cmd.Flags().StringVarP(&opts.Session.Name, "name", "n", "",
		"Set a display name for this session (shown in /resume picker and terminal title)")
	cmd.Flags().StringVar(&opts.Session.Prefill, "prefill", "",
		"Pre-fill the prompt input with text without submitting it")
	if err := cmd.Flags().MarkHidden("prefill"); err != nil {
		panic(err)
	}

	// Print
	cmd.Flags().BoolVarP(&opts.Print.Print, "print", "p", false,
		"Print response and exit (useful for pipes). Note: trust dialog skipped in non-interactive mode.")
	cmd.Flags().StringVar(&opts.Print.OutputFormat, "output-format", "",
		"Output format (only works with --print): \"text\" (default), \"json\" (single result), or \"stream-json\" (realtime streaming)")
	cmd.Flags().StringVar(&opts.Print.InputFormat, "input-format", "",
		"Input format (only works with --print): \"text\" (default), or \"stream-json\" (realtime streaming input)")
	cmd.Flags().BoolVar(&opts.Print.IncludeHookEvents, "include-hook-events", false,
		"Include all hook lifecycle events in the output stream (only works with --output-format=stream-json)")
	cmd.Flags().BoolVar(&opts.Print.IncludePartialMsgs, "include-partial-messages", false,
		"Include partial message chunks as they arrive (only works with --print and --output-format=stream-json)")
	cmd.Flags().StringVar(&opts.Print.JSONSchema, "json-schema", "",
		"JSON Schema for structured output validation. Example: {\"type\":\"object\",\"properties\":{\"name\":{\"type\":\"string\"}},\"required\":[\"name\"]}")
	cmd.Flags().BoolVar(&opts.Print.ReplayUserMessages, "replay-user-messages", false,
		"Re-emit user messages from stdin back on stdout for acknowledgment (only works with --input-format=stream-json and --output-format=stream-json)")
	cmd.Flags().Float64Var(&opts.Print.MaxBudgetUSD, "max-budget-usd", 0,
		"Maximum dollar amount to spend on API calls (only works with --print)")

	// Tools
	cmd.Flags().StringSliceVar(&opts.Tools.AllowedTools, "allowed-tools", nil,
		"Comma or space-separated list of tool names to allow (e.g. \"Bash(psql:*) Query\")")
	// CC camelCase alias for parity (hidden to avoid duplicating in --help).
	cmd.Flags().StringSliceVar(&opts.Tools.AllowedTools, "allowedTools", nil, "")
	if err := cmd.Flags().MarkHidden("allowedTools"); err != nil {
		panic(err)
	}
	cmd.Flags().StringSliceVar(&opts.Tools.DisallowedTools, "disallowed-tools", nil,
		"Comma or space-separated list of tool names to deny (e.g. \"Bash(psql:*) Query\")")
	cmd.Flags().StringSliceVar(&opts.Tools.DisallowedTools, "disallowedTools", nil, "")
	if err := cmd.Flags().MarkHidden("disallowedTools"); err != nil {
		panic(err)
	}
	cmd.Flags().StringSliceVar(&opts.Tools.Tools, "tools", nil,
		"Specify the list of available tools from the built-in set. Use \"\" to disable all tools, \"default\" for all, or specify names.")
	cmd.Flags().BoolVar(&opts.Tools.DisableSlash, "disable-slash-commands", false,
		"Disable all skills")

	// IO
	cmd.Flags().StringVar(&opts.IO.Settings, "settings", "",
		"Path to a settings JSON file or a JSON string to load additional settings from")
	cmd.Flags().StringSliceVar(&opts.IO.AddDir, "add-dir", nil,
		"Additional directories to allow tool access to")
	cmd.Flags().BoolVar(&opts.IO.IDE, "ide", false,
		"Automatically connect to IDE on startup if exactly one valid IDE is available")
	cmd.Flags().StringVar(&opts.IO.SystemPrompt, "system-prompt", "",
		"System prompt to use for the session")
	cmd.Flags().StringVar(&opts.IO.AppendSystem, "append-system-prompt", "",
		"Append a system prompt to the default system prompt")
	// Hidden file variants of the system prompt (per spec § 2.3 Class D).
	var sysPromptFile, appendSysPromptFile string
	cmd.Flags().StringVar(&sysPromptFile, "system-prompt-file", "",
		"Read system prompt from a file")
	cmd.Flags().StringVar(&appendSysPromptFile, "append-system-prompt-file", "",
		"Read system prompt from a file and append to the default system prompt")
	for _, name := range []string{"system-prompt-file", "append-system-prompt-file"} {
		if err := cmd.Flags().MarkHidden(name); err != nil {
			panic(err)
		}
	}
	cmd.Flags().StringVar(&opts.IO.SettingSources, "setting-sources", "",
		"Comma-separated list of setting sources to load (user, project, local).")
	cmd.Flags().StringSliceVar(&opts.IO.PluginDir, "plugin-dir", nil,
		"Load plugins from a directory for this session only (repeatable)")
	cmd.Flags().StringSliceVar(&opts.IO.File, "file", nil,
		"File resources to download at startup. Format: file_id:relative_path")
	cmd.Flags().BoolVar(&opts.IO.Bare, "bare", false,
		"Minimal mode: skip hooks, plugin sync, auto-memory, background prefetches, and CLAUDE.md auto-discovery. Sets OPENDBX_SIMPLE=1.")
	cmd.Flags().StringVar(&opts.IO.PermissionMode, "permission-mode", "",
		"Permission mode to use for the session")
	cmd.Flags().BoolVar(&opts.IO.DangerouslySkip, "dangerously-skip-permissions", false,
		"Bypass all permission checks. Recommended only for sandboxes with no internet access.")
	cmd.Flags().BoolVar(&opts.IO.AllowDangerous, "allow-dangerously-skip-permissions", false,
		"Enable bypassing all permission checks as an option, without it being enabled by default.")

	// === Class B: CC name kept, DB-flavored description ===

	cmd.Flags().StringVar(&opts.Model.Model, "model", "",
		"Model for the current diagnosis session. Provide an alias (e.g. 'sonnet' or 'opus') or a full model name.")
	cmd.Flags().StringVar(&opts.Model.Agent, "agent", "",
		"Diagnosis agent profile (overrides 'agent' setting in config).")
	cmd.Flags().StringVar(&opts.Model.FallbackModel, "fallback-model", "",
		"Enable automatic fallback to specified model when default model is overloaded (only works with --print)")
	cmd.Flags().StringVar(&opts.Model.Effort, "effort", "",
		"Effort level for the current diagnosis session (low, medium, high, max)")

	cmd.Flags().StringSliceVar(&opts.MCP.MCPConfig, "mcp-config", nil,
		"Load MCP servers from JSON files or strings (space-separated). DB-related MCP servers can be configured here.")
	cmd.Flags().BoolVar(&opts.MCP.StrictMCPConfig, "strict-mcp-config", false,
		"Only use MCP servers from --mcp-config, ignoring all other MCP configurations")

	// === Class C: opendbx-specific NEW flags (per D5: --llm-tier semantically independent of --model) ===

	cmd.Flags().StringVar(&opts.DB.DB, "db", "",
		"Database type for the session: postgres (MVP), mysql/oracle/opengauss (Stage 6+ reserved)")
	cmd.Flags().StringVar(&opts.DB.Connection, "connection", "",
		"Database connection DSN, e.g. \"postgres://user:pass@host:5432/dbname\". Mutually exclusive with --connection-alias.")
	cmd.Flags().StringVar(&opts.DB.ConnectionAlias, "connection-alias", "",
		"Database connection alias from 'opendbx db list'. Mutually exclusive with --connection.")
	cmd.Flags().StringVar(&opts.Model.LLMTier, "llm-tier", "",
		"LLM model tier (strategy layer, semantically independent of --model): tier-1 (Opus primary) / tier-2 (glm-5 backup) / tier-3 (deepseek deep-dive) / tier-4 (local). tier→model mapping resolved from config.")

	// === Class D: hidden flags (kept for compatibility, not in --help) ===

	cmd.Flags().BoolVar(&opts.Hidden.Init, "init", false, "Run Setup hooks with init trigger, then continue")
	cmd.Flags().BoolVar(&opts.Hidden.InitOnly, "init-only", false, "Run Setup and SessionStart:startup hooks, then exit")
	cmd.Flags().BoolVar(&opts.Hidden.Maintenance, "maintenance", false, "Run Setup hooks with maintenance trigger, then continue")
	cmd.Flags().StringVar(&opts.Hidden.Thinking, "thinking", "",
		"Thinking mode: enabled (equivalent to adaptive), disabled")
	for _, name := range []string{"init", "init-only", "maintenance", "thinking"} {
		if err := cmd.Flags().MarkHidden(name); err != nil {
			panic(err)
		}
	}
}

// optionSpecRow records one CLI flag's adaptation decision (spec-0.3 D-7).
type optionSpecRow struct {
	Name   string     // pflag long name (no leading --)
	Short  string     // shorthand (single letter, "" if none)
	Class  adaptClass // A/B/C/D
	Hidden bool       // true if MarkHidden in registerFlags
}

// optionSpecs is the curated adaptation table. Validated by
// TestOptionSpecMatchesFlags in main_test.go: every row resolves via
// cmd.Flags().Lookup(); no flag exists outside the table.
var optionSpecs = []optionSpecRow{
	// === --version + -v handled separately (manual) ===
	{Name: "version", Short: "v", Class: classA},

	// === Class A: direct 1:1 ===
	{Name: "debug", Short: "d", Class: classA},
	{Name: "debug-to-stderr", Class: classA, Hidden: true},
	{Name: "debug-file", Class: classA},
	{Name: "verbose", Class: classA},
	{Name: "continue", Short: "c", Class: classA},
	{Name: "resume", Short: "r", Class: classA},
	{Name: "fork-session", Class: classA},
	{Name: "from-pr", Class: classA},
	{Name: "no-session-persistence", Class: classA},
	{Name: "session-id", Class: classA},
	{Name: "name", Short: "n", Class: classA},
	{Name: "prefill", Class: classA, Hidden: true},
	{Name: "print", Short: "p", Class: classA},
	{Name: "output-format", Class: classA},
	{Name: "input-format", Class: classA},
	{Name: "include-hook-events", Class: classA},
	{Name: "include-partial-messages", Class: classA},
	{Name: "json-schema", Class: classA},
	{Name: "replay-user-messages", Class: classA},
	{Name: "max-budget-usd", Class: classA},
	{Name: "allowed-tools", Class: classA},
	{Name: "allowedTools", Class: classA, Hidden: true}, // CC camelCase alias
	{Name: "disallowed-tools", Class: classA},
	{Name: "disallowedTools", Class: classA, Hidden: true}, // CC camelCase alias
	{Name: "tools", Class: classA},
	{Name: "disable-slash-commands", Class: classA},
	{Name: "settings", Class: classA},
	{Name: "add-dir", Class: classA},
	{Name: "ide", Class: classA},
	{Name: "system-prompt", Class: classA},
	{Name: "append-system-prompt", Class: classA},
	{Name: "system-prompt-file", Class: classA, Hidden: true},
	{Name: "append-system-prompt-file", Class: classA, Hidden: true},
	{Name: "setting-sources", Class: classA},
	{Name: "plugin-dir", Class: classA},
	{Name: "file", Class: classA},
	{Name: "bare", Class: classA},
	{Name: "permission-mode", Class: classA},
	{Name: "dangerously-skip-permissions", Class: classA},
	{Name: "allow-dangerously-skip-permissions", Class: classA},

	// === Class B: CC name kept, DB-flavored description ===
	{Name: "model", Class: classB},
	{Name: "agent", Class: classB},
	{Name: "fallback-model", Class: classB},
	{Name: "effort", Class: classB},
	{Name: "mcp-config", Class: classB},
	{Name: "strict-mcp-config", Class: classB},

	// === Class C: opendbx-specific NEW ===
	{Name: "db", Class: classC},
	{Name: "connection", Class: classC},
	{Name: "connection-alias", Class: classC},
	{Name: "llm-tier", Class: classC},

	// === Class D: hidden ===
	{Name: "init", Class: classD, Hidden: true},
	{Name: "init-only", Class: classD, Hidden: true},
	{Name: "maintenance", Class: classD, Hidden: true},
	{Name: "thinking", Class: classD, Hidden: true},
}
