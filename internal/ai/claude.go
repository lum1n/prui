package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type claude struct {
	client  *http.Client
	apiKey  string
	baseURL string
	model   string
}

func newClaude(opts Options) (*claude, error) {
	key, err := resolveToken(opts.Provider.TokenEnv, "ANTHROPIC_API_KEY")
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(strings.TrimSpace(opts.Provider.BaseURL), "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	model := strings.TrimSpace(opts.Provider.Model)
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	return &claude{
		client:  httpClient(opts),
		apiKey:  key,
		baseURL: base,
		model:   model,
	}, nil
}

func (c *claude) Kind() string { return "claude" }

func (c *claude) Complete(ctx context.Context, req Request) (string, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": req.User},
		},
	}
	if strings.TrimSpace(req.System) != "" {
		body["system"] = req.System
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("claude: HTTP %d: %s", resp.StatusCode, truncateErr(data))
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("claude: decode: %w", err)
	}
	var b strings.Builder
	for _, block := range out.Content {
		if block.Type == "text" || block.Type == "" {
			b.WriteString(block.Text)
		}
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", fmt.Errorf("claude: empty response")
	}
	return text, nil
}

func truncateErr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
