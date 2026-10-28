//go:build !unix

package main

import (
	"os"
	"os/exec"
)

func detach(cmd *exec.Cmd) {}

func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
