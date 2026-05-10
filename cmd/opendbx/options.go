// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Options struct (spec-0.3 D-7).
//
// Per user D6+D11 decision: 50+ flag values are split into sub-structs by
// concern, not collapsed into a god struct. Each sub-struct ≤ 12 fields
// (spec-0.3 § 6 R-4 mitigation).

package main

// Options is the typed in-memory shape of all CLI flags collected by
// cmd/opendbx. flags.go binds cobra flags to the matching field via the
// optionSpec table (D-7 single source of truth).
type Options struct {
	// Debug — observability flags (CC class A).
	Debug DebugOptions

	// Session — session lifecycle, resumption, naming (CC class A).
	Session SessionOptions

	// Model — LLM model + tier configuration (CC class A/B + opendbx class C).
	Model ModelOptions

	// Print — non-interactive / pipe mode (CC class A).
	Print PrintOptions

	// Tools — tool allow/deny/select (CC class A).
	Tools ToolOptions

	// MCP — MCP server config (CC class B).
	MCP MCPOptions

	// DB — opendbx-specific database connection options (class C, NEW).
	DB DBOptions

	// IO — settings file, add-dir, IDE, files (CC class A).
	IO IOOptions

	// Hidden — flags marked hideHelp by CC (class D mostly): kept for
	// compatibility but not in --help.
	Hidden HiddenOptions
}

// DebugOptions — CC class A (1:1 sympathy).
type DebugOptions struct {
	Debug         string // -d, --debug [filter]
	DebugToStderr bool   // -d2e, --debug-to-stderr (hidden)
	DebugFile     string // --debug-file <path>
	Verbose       bool   // --verbose
}

// SessionOptions — CC class A.
type SessionOptions struct {
	Prompt           string // [prompt] positional argument
	Continue         bool   // -c, --continue
	Resume           string // -r, --resume [value]
	ForkSession      bool   // --fork-session
	FromPR           string // --from-pr [value]
	NoSessionPersist bool   // --no-session-persistence
	SessionID        string // --session-id <uuid>
	Name             string // -n, --name <name>
	Prefill          string // --prefill <text> (hidden)
}

// ModelOptions — class A (--model) + class C (--llm-tier semantically independent per D5).
type ModelOptions struct {
	Model         string // --model <model>
	Agent         string // --agent <agent>
	FallbackModel string // --fallback-model <model>
	LLMTier       string // --llm-tier <tier> (NEW, opendbx-specific; semantically independent of --model per spec-0.3 § 2.3 D5)
	Effort        string // --effort <level>
}

// PrintOptions — class A.
type PrintOptions struct {
	Print              bool    // -p, --print
	OutputFormat       string  // --output-format <format>
	InputFormat        string  // --input-format <format>
	IncludeHookEvents  bool    // --include-hook-events
	IncludePartialMsgs bool    // --include-partial-messages
	JSONSchema         string  // --json-schema <schema>
	ReplayUserMessages bool    // --replay-user-messages
	MaxBudgetUSD       float64 // --max-budget-usd <amount>
}

// ToolOptions — class A.
type ToolOptions struct {
	AllowedTools    []string // --allowedTools, --allowed-tools <tools...>
	DisallowedTools []string // --disallowedTools, --disallowed-tools <tools...>
	Tools           []string // --tools <tools...>
	DisableSlash    bool     // --disable-slash-commands
}

// MCPOptions — class B (CC name kept, DB-friendly description).
type MCPOptions struct {
	MCPConfig       []string // --mcp-config <configs...>
	StrictMCPConfig bool     // --strict-mcp-config
}

// DBOptions — class C, opendbx-specific.
type DBOptions struct {
	DB              string // --db <type> (postgres/mysql/oracle/...)
	Connection      string // --connection <dsn>
	ConnectionAlias string // --connection-alias <alias>
}

// IOOptions — class A.
type IOOptions struct {
	Settings        string   // --settings <file-or-json>
	AddDir          []string // --add-dir <directories...>
	IDE             bool     // --ide
	SystemPrompt    string   // --system-prompt <prompt>
	AppendSystem    string   // --append-system-prompt <prompt>
	SettingSources  string   // --setting-sources <sources>
	PluginDir       []string // --plugin-dir <path>
	File            []string // --file <specs...>
	Bare            bool     // --bare
	PermissionMode  string   // --permission-mode <mode>
	DangerouslySkip bool     // --dangerously-skip-permissions
	AllowDangerous  bool     // --allow-dangerously-skip-permissions
}

// HiddenOptions — class D (CC hideHelp flags kept for compatibility).
type HiddenOptions struct {
	Init        bool   // --init
	InitOnly    bool   // --init-only
	Maintenance bool   // --maintenance
	Thinking    string // --thinking <mode>
}

// validOutputFormats / validInputFormats / validPermissionModes are the
// allowed enum values for the corresponding flags. spec § 3.1 requires
// invalid values to exit 1; flags.go validateOptions enforces this in
// PreRunE.
var (
	validOutputFormats   = []string{"text", "json", "stream-json"}
	validInputFormats    = []string{"text", "stream-json"}
	validPermissionModes = []string{"acceptEdits", "auto", "bypassPermissions", "default", "dontAsk", "plan"}
)

// newOptions returns a zero-valued Options struct. Used by root.go to
// share a single Options instance across the root command and all
// subcommands (each subcommand reads from `opts` after cobra has parsed).
func newOptions() *Options {
	return &Options{}
}
