package runner

import (
	"strings"

	"github.com/redbackthomson/nix-tasks/internal/config"
)

// GenerateScript creates a bash script from task definition
func GenerateScript(task config.Task) string {
	// If raw script is provided, use it directly
	if task.Script != "" {
		return task.Script
	}

	// Otherwise, join commands with newlines
	return strings.Join(task.Commands, "\n")
}
