// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package docker

import (
	"os"
	"runtime"
	"syscall"
	"time"
)

// SessionMembers is Linux-only (it reads /proc). Elsewhere `run stop` relies on
// the run's own graceful teardown, which reaps every child it started.
func SessionMembers(sid int, notBefore time.Time) []int { return nil }

// ProcessCmdline is unavailable off Linux.
func ProcessCmdline(pid int) string { return "" }

// PidAlive reports whether pid is a live process.
func PidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		// FindProcess opens a handle there, which fails for a pid that is gone.
		return true
	}
	return p.Signal(syscall.Signal(0)) == nil
}
