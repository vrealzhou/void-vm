package vmctl

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labstack/echo/v5"
)

func handleHostFiles() echo.HandlerFunc {
	return func(c *echo.Context) error {
		path := c.QueryParam("path")
		root := c.QueryParam("root")

		homeDir, _ := os.UserHomeDir()
		if path == "" {
			if root != "" {
				path = root
			} else if homeDir != "" {
				path = homeDir
			} else {
				path = "/"
			}
		}

		if root != "" {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid path"})
			}
			absRoot, err := filepath.Abs(root)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid root"})
			}
			if !strings.HasPrefix(absPath, absRoot) && absPath != absRoot {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "cannot navigate above root"})
			}
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		var result []VMFileEntry
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			result = append(result, VMFileEntry{
				Name:  name,
				IsDir: entry.IsDir(),
			})
		}

		sort.Slice(result, func(i, j int) bool {
			if result[i].IsDir != result[j].IsDir {
				return result[i].IsDir
			}
			return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
		})

		return c.JSON(http.StatusOK, map[string]any{
			"path":     path,
			"entries":  result,
		})
	}
}
