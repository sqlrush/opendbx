// Copyright 2026 opendbx contributors. See LICENSE.
// Author: sqlrush
//
// Package main is the opendbx binary entry point.
//
// Stage 0 骨架：仅实现 --version。
// 后续 stage 加入 interact / agent / cluster / admin 子命令。
// Design: opendbrb/docs/architecture.md, opendbrb/specs/stage-0/spec-0.3-cmd-entrypoints.md (TBD).
package main

import (
	"flag"
	"fmt"
	"os"
)

// version is set by linker flags via Makefile's -X main.version.
var version = "dev"

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("opendbx %s\n", version)
		os.Exit(0)
	}

	fmt.Println("opendbx — Stage 0 skeleton. Run with --version to see version.")
	fmt.Println("See https://github.com/sqlrush/opendbrb (private) for design docs.")
}
