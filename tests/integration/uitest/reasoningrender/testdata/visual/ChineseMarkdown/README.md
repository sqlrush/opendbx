# ChineseMarkdown (parked-only, spec-1.20.2 D-6)

13th independent block-style visual fixture (env gate
`REASONINGRENDER_VISUAL_REQUIRED`). Production-like smoke at
tests/integration/uitest/reasoningrender/reasoningrender_test.go
exercises the spec D-6 invariants (no mojibake / no mixed reasoning /
no stderr tear) without a captured golden; this directory is reserved
for the CC-rendered golden.png pinned at T-9 + after capture SOP runs
post-stage-1 acceptance.

See spec-1.10..1.21 parking precedent for the exact format of the
golden.png + input.ansi + metadata.json triple.
