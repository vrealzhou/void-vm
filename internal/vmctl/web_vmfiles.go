package vmctl

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v5"
)

type VMFileEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
}

func handleVMFiles(cfg Config) echo.HandlerFunc {
	return func(c *echo.Context) error {
		path := c.QueryParam("path")
		root := c.QueryParam("root")

		if path == "" {
			if root != "" {
				path = root
			} else {
				path = "/home/" + cfg.GuestUser
			}
		}

		// Security: prevent escaping root
		if root != "" {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid path"})
			}
			absRoot, err := filepath.Abs(root)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid root"})
			}
			if !strings.HasPrefix(absPath, absRoot) {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "cannot navigate above root"})
			}
		}

		entries, err := listVMFiles(cfg, path)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, entries)
	}
}

func listVMFiles(cfg Config, path string) ([]VMFileEntry, error) {
	remoteCmd := fmt.Sprintf("ls -la %s", shellQuote(path))
	cmd := exec.Command("ssh", append(sshArgs(cfg), remoteCmd)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ssh ls failed: %w", err)
	}

	var entries []VMFileEntry
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 9 {
			continue
		}
		name := strings.Join(parts[8:], " ")
		if name == "." || name == ".." {
			continue
		}
		isDir := strings.HasPrefix(parts[0], "d")
		entries = append(entries, VMFileEntry{Name: name, IsDir: isDir})
	}
	return entries, nil
}
