package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	projectRoot, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	if filepath.Base(projectRoot) == "scripts" {
		projectRoot = filepath.Dir(projectRoot)
	}
	if err := stopBinary(); err != nil {
		fail(err)
	}
	binPath := filepath.Join(projectRoot, "bin")
	if err := os.MkdirAll(binPath, 0755); err != nil {
		fail(err)
	}
	name := "MorenoWoW"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	command := exec.Command("go", "build", "-o", filepath.Join(binPath, name), ".")
	command.Dir = projectRoot
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fail(err)
	}
}

func stopBinary() error {
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		command = exec.Command("taskkill.exe", "//IM", "MorenoWoW.exe", "//F")
	} else {
		command = exec.Command("pkill", "-x", "MorenoWoW")
	}
	if err := command.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil
		}
		return err
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
