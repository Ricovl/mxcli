// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// setDetachAttrs starts the background run in a session of its own: it no
// longer belongs to the calling shell (so the end of a tool call, a closed
// terminal or a SIGHUP does not take it down), and every process it starts
// shares its session id — the handle `run stop` uses to prove none is left.
func setDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// terminateProcess asks a process to shut down (SIGTERM). `run --local`
// handles it like Ctrl-C and tears down its children.
func terminateProcess(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }

// killProcess force-kills a process that did not stop when asked.
func killProcess(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }
