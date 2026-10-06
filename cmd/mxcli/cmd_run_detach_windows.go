// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// setDetachAttrs starts the background run without a console and in a process
// group of its own, so closing the calling console does not end it.
func setDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess, HideWindow: true}
}

// terminateProcess ends the run and its whole process tree. Windows has no
// SIGTERM for a console-less process, so this is not a graceful stop: the
// run's own teardown (removing --test-endpoint) does not run.
func terminateProcess(pid int) error {
	return exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(pid)).Run()
}

// killProcess force-kills a process.
func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
