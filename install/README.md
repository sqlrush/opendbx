# `install/` — platform-specific installer assets

Reserved for **spec-4.7-install** which lands the actual installer scripts
(homebrew formula / `.deb` / `.rpm` / windows MSI / etc.). Stage 0 ~ Stage 4
keep this directory as a placeholder so the layout matches `cmd/`,
`internal/`, `pkg/` etc.

## Layout

```
install/
├── README.md     ← this file
├── darwin/       ← macOS installers (homebrew, .pkg)
│   └── README.md
└── linux/        ← Linux installers (.deb, .rpm, snap)
    └── README.md
```

## Why empty in spec-0.2 ~ spec-0.4

The relevant installer mechanism depends on:

- spec-0.7-version-numbering (semver tag scheme)
- spec-0.9-ci-github-actions (release artifact CI)
- spec-4.7-install (installer authoring + hosting strategy)

Until those land, manual install is the only supported path:

```bash
make build  # produces ./bin/opendbx
ln -sf "$PWD/bin/opendbx" ~/.local/bin/opendbx
```

## Related specs

- spec-0.2 § 2.1 (file layout reservation)
- spec-0.7-version-numbering
- spec-4.7-install (this directory's owner)
