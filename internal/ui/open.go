package ui

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser opens url with the OS default browser.
func openBrowser(url string) error {
	if url == "" {
		return fmt.Errorf("no URL")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported OS %s", runtime.GOOS)
	}
	return cmd.Start()
}
