package vmctl

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var ManagedConfigKeys = []string{
	"VM_DEFAULT_SHELL",
	"VM_DEFAULT_EDITOR",
	"VM_WINDOW_MANAGER",
}

func DotEnvPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".vmctl.env")
}

func UpdateDotEnvFile(path string, updates map[string]string) error {
	existing := []string{}
	if data, err := os.ReadFile(path); err == nil {
		existing = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return err
	}

	remaining := map[string]string{}
	for key, value := range updates {
		remaining[key] = value
	}

	out := make([]string, 0, len(existing)+len(updates)+2)
	for _, line := range existing {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
			continue
		}

		key, _, ok := strings.Cut(line, "=")
		if !ok {
			out = append(out, line)
			continue
		}
		key = strings.TrimSpace(key)
		value, managed := remaining[key]
		if !managed {
			out = append(out, line)
			continue
		}
		out = append(out, fmt.Sprintf("%s=%s", key, quoteDotEnvValue(value)))
		delete(remaining, key)
	}

	if len(remaining) > 0 {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		for _, key := range ManagedConfigKeys {
			value, ok := remaining[key]
			if !ok {
				continue
			}
			out = append(out, fmt.Sprintf("%s=%s", key, quoteDotEnvValue(value)))
			delete(remaining, key)
		}
		if len(remaining) > 0 {
			extraKeys := make([]string, 0, len(remaining))
			for key := range remaining {
				extraKeys = append(extraKeys, key)
			}
			slices.Sort(extraKeys)
			for _, key := range extraKeys {
				out = append(out, fmt.Sprintf("%s=%s", key, quoteDotEnvValue(remaining[key])))
			}
		}
	}

	content := strings.Join(out, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func quoteDotEnvValue(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t#\"'") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return value
}
