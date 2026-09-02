package ai

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lum1n/prui/internal/config"
)

type codex struct {
	binary string
	model  string
}

func newCodex(p config.AIProvider) (*codex, error) {
	bin := strings.TrimSpace(p.Binary)
	if bin == "" {
		bin = "codex"
	}
	return &codex{binary: bin, model: strings.TrimSpace(p.Model)}, nil
}

func (c *codex) Kind() string { return "codex" }

func (c *codex) Complete(ctx context.Context, req Request) (string, error) {
	prompt := req.User
	if strings.TrimSpace(req.System) != "" {
		prompt = req.System + "\n\n" + req.User
	}
	args := []string{"exec", "--sandbox", "read-only"}
	model := req.Model
	if model == "" {
		model = c.model
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	cmd := exec.CommandContext(ctx, c.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("codex: %s", msg)
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("codex: empty output")
	}
	return out, nil
}

type openCode struct {
	binary string
	model  string
}

func newOpenCode(p config.AIProvider) (*openCode, error) {
	bin := strings.TrimSpace(p.Binary)
	if bin == "" {
		bin = "opencode"
	}
	return &openCode{binary: bin, model: strings.TrimSpace(p.Model)}, nil
}

func (o *openCode) Kind() string { return "opencode" }

func (o *openCode) Complete(ctx context.Context, req Request) (string, error) {
	prompt := req.User
	if strings.TrimSpace(req.System) != "" {
		prompt = req.System + "\n\n" + req.User
	}
	args := []string{"run"}
	model := req.Model
	if model == "" {
		model = o.model
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	cmd := exec.CommandContext(ctx, o.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("opencode: %s", msg)
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("opencode: empty output")
	}
	return out, nil
}

// CodexArgs returns the argv for tests (without binary).
func CodexArgs(model, prompt string) []string {
	args := []string{"exec", "--sandbox", "read-only"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	return args
}

// OpenCodeArgs returns the argv for tests (without binary).
func OpenCodeArgs(model, prompt string) []string {
	args := []string{"run"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	return args
}
