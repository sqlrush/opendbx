# `configs/` — environment-specific config templates

Per **spec-0.2 § 2.1 D-10** this directory hosts **example / starter** config
templates for each environment (dev / test / prod). It is **not** the runtime
config-file location — runtime paths follow **spec-0.4 § 2 D-5**:

- Linux: `$XDG_CONFIG_HOME/opendbx/config.yaml` (default
  `~/.config/opendbx/config.yaml`)
- macOS: `~/Library/Application Support/opendbx/config.yaml`, fallback
  `~/.opendbx/config.yaml`
- Windows: `%APPDATA%/opendbx/config.yaml`

Override at session level with `--settings <path>`.

## Layout

```
configs/
├── README.md     ← this file
├── dev/          ← developer machine examples
│   └── README.md
├── test/         ← CI / integration test examples
│   └── README.md
└── prod/         ← production deployment examples
    └── README.md
```

## When does this differ from `~/.opendbx/config.yaml`?

Each environment subdirectory holds reference templates that:

- Document a known-good baseline for that environment
- Get committed to the repo for new contributors
- Get generated via `opendbx admin config dump-defaults > configs/dev/example.yaml`

Users still copy or symlink these into their actual XDG/macOS path; the
templates are not auto-loaded.

## Source-of-truth

Schema lives in **spec-0.4 § 2.2** and `internal/platform/config/config.go`.
Run `opendbx admin config dump-schema` to emit the live JSON Schema for
IDE autocomplete.

## Related specs

- spec-0.2 § 2.1 (file layout)
- spec-0.4 § 2 (config framework + path resolution + 5 source override
  chain)
- spec-0.4 D-8 (admin config dump-defaults / dump-schema / sources /
  validate / dump-env-map)
