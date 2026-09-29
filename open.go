package main

import (
	"os/exec"
	goruntime "runtime"
)

// openFolder shows dir in the OS file manager. The server runs on the user's
// machine, so this works for the desktop window and the headless browser UI.
func openFolder(dir string) error {
	switch goruntime.GOOS {
	case "darwin":
		return exec.Command("open", dir).Run()
	case "windows":
		// explorer exits with 1 even on success.
		_ = exec.Command("explorer", dir).Run()
		return nil
	default:
		return exec.Command("xdg-open", dir).Run()
	}
}
