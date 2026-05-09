# tests/perf

Performance benchmarks. Each Stage end freezes a baseline JSON
(`docs/perf/baseline-stage<N>.json`) and CI compares against it
(per CLAUDE.md rule 11).

> 3% regression → WARN. > 5% → FAIL + RCA.

