package release

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func runCommand(ctx context.Context, workdir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workdir
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if combined == "" {
			combined = err.Error()
		}
		return "", fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, combined)
	}

	combined := strings.TrimSpace(stdout.String())
	if combined == "" {
		combined = strings.TrimSpace(stderr.String())
	}
	return combined, nil
}

func defaultCommandContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Minute)
}
