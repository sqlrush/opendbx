module github.com/sqlrush/opendbx

go 1.24.0

require (
	github.com/alecthomas/chroma/v2 v2.24.1
	github.com/anthropics/anthropic-sdk-go v1.45.0
	github.com/creack/pty v1.1.24
	github.com/gdamore/tcell/v2 v2.13.9
	github.com/hinshun/vt10x v0.0.0-20220301184237-5011da428d02
	github.com/jackc/pgx/v5 v5.7.6
	github.com/mattn/go-runewidth v0.0.23
	github.com/openai/openai-go v1.12.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.9
	github.com/yuin/goldmark v1.7.8
	go.yaml.in/yaml/v3 v3.0.4
	golang.org/x/term v0.37.0
	golang.org/x/tools v0.38.0
)

require (
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/invopop/jsonschema v0.13.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/standard-webhooks/standard-webhooks/libraries v0.0.1 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	golang.org/x/crypto v0.40.0 // indirect
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

exclude github.com/rogpeppe/go-internal v1.15.0 // spec-1.18: v1.15.0 needs go1.25; cap at v1.14.1 (has fmtsort, go1.23) to keep project go 1.24.0
