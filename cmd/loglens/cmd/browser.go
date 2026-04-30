package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser tries to open url in the user's default browser, returns an error if no opener is available
func openBrowser(url string) error {
	var bin string
	var args []string
	switch runtime.GOOS {
	case "linux":
		bin = "xdg-open"
		args = []string{url}
	//for macOS
	case "darwin":
		bin = "open"
		args = []string{url}
	case "windows":
		bin = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported platform %q", runtime.GOOS)
	}
	return exec.Command(bin, args...).Start()
}
