package cmd

// ckeletin:allow-custom-command

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

func emitResult(cmd *cobra.Command, message string, payload any) error {
	if outputMode == "json" {
		resp := map[string]any{
			"message": message,
			"data":    payload,
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(resp)
	}

	if message != "" {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), message); err != nil {
			return err
		}
	}
	return nil
}

func resolvePath(path string) (string, error) {
	if path == "" {
		path = "."
	}
	return filepath.Abs(path)
}
