//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// detach puts a child in a session of its own, so it outlives the terminal.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

func terminate(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }
