package cmd

// ckeletin:allow-custom-command

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/alfariiizi/vandor/internal/vpkg"
)

func tryHandleAliasShortcut(executeErr error, rawArgs []string, stdout, stderr io.Writer) (bool, error) {
	if executeErr == nil {
		return false, nil
	}
	if !strings.Contains(strings.ToLower(executeErr.Error()), "unknown command") {
		return false, nil
	}
	parsed, ok := parseAliasShortcut(rawArgs)
	if !ok {
		return false, nil
	}
	if parsed.outputMode != "text" && parsed.outputMode != "json" {
		return true, fmt.Errorf("invalid --output %q, allowed values: text, json", parsed.outputMode)
	}
	root, err := resolvePath(parsed.projectPath)
	if err != nil {
		return true, err
	}
	manager := vpkg.NewManager(root)
	actionStdout := stdout
	if parsed.outputMode == "json" {
		actionStdout = stderr
	}
	result, err := manager.ExecAlias(parsed.alias, parsed.args, actionStdout, stderr)
	if err != nil {
		return true, err
	}
	message := fmt.Sprintf("Executed alias %s (%s:%s)", parsed.alias, result.Package, result.Action)
	if parsed.outputMode == "json" {
		payload := map[string]any{
			"message": message,
			"data":    result,
		}
		if err := json.NewEncoder(stdout).Encode(payload); err != nil {
			return true, err
		}
		return true, nil
	}
	_, err = fmt.Fprintln(stdout, message)
	return true, err
}

type aliasShortcutInvocation struct {
	projectPath string
	outputMode  string
	alias       string
	args        []string
}

func parseAliasShortcut(rawArgs []string) (aliasShortcutInvocation, bool) {
	inv := aliasShortcutInvocation{
		projectPath: ".",
		outputMode:  outputMode,
	}
	for i := 0; i < len(rawArgs); i++ {
		arg := strings.TrimSpace(rawArgs[i])
		if arg == "" {
			continue
		}
		if arg == "--" {
			return aliasShortcutInvocation{}, false
		}
		switch {
		case arg == "--path":
			if i+1 >= len(rawArgs) {
				return aliasShortcutInvocation{}, false
			}
			inv.projectPath = rawArgs[i+1]
			i++
			continue
		case strings.HasPrefix(arg, "--path="):
			inv.projectPath = strings.TrimPrefix(arg, "--path=")
			continue
		case arg == "--output":
			if i+1 >= len(rawArgs) {
				return aliasShortcutInvocation{}, false
			}
			inv.outputMode = rawArgs[i+1]
			i++
			continue
		case strings.HasPrefix(arg, "--output="):
			inv.outputMode = strings.TrimPrefix(arg, "--output=")
			continue
		case isGlobalBoolFlag(arg):
			continue
		case strings.HasPrefix(arg, "-"):
			continue
		default:
			if !isAliasShortcutCandidate(arg) {
				return aliasShortcutInvocation{}, false
			}
			inv.alias = arg
			args, path, mode := parseAliasTail(rawArgs[i+1:], inv.projectPath, inv.outputMode)
			inv.args = args
			inv.projectPath = path
			inv.outputMode = mode
			return inv, true
		}
	}
	return aliasShortcutInvocation{}, false
}

func parseAliasTail(raw []string, projectPath, mode string) ([]string, string, string) {
	args := make([]string, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		arg := strings.TrimSpace(raw[i])
		if arg == "--" {
			args = append(args, raw[i+1:]...)
			break
		}
		switch {
		case arg == "--path":
			if i+1 < len(raw) {
				projectPath = raw[i+1]
				i++
			}
			continue
		case strings.HasPrefix(arg, "--path="):
			projectPath = strings.TrimPrefix(arg, "--path=")
			continue
		case arg == "--output":
			if i+1 < len(raw) {
				mode = raw[i+1]
				i++
			}
			continue
		case strings.HasPrefix(arg, "--output="):
			mode = strings.TrimPrefix(arg, "--output=")
			continue
		case isGlobalBoolFlag(arg):
			continue
		default:
			args = append(args, raw[i])
		}
	}
	return args, projectPath, mode
}

func isGlobalBoolFlag(arg string) bool {
	switch arg {
	case "-i", "--interactive", "-t", "--interactive-alias", "-it", "--yes", "--no-input":
		return true
	default:
		return false
	}
}

func isAliasShortcutCandidate(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	if strings.HasPrefix(token, "-") {
		return false
	}
	return strings.Contains(token, ":")
}
