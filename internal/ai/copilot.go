package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/vegard/prui/internal/auth"
)

type copilot struct {
	client    *http.Client
	githubAPI string
	cred      auth.Credentials
	model     string
}

func newCopilot(opts Options) (*copilot, error) {
	apiURL, cred, _, err := resolveGitHubCred(opts)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(opts.Provider.Model)
	if model == "" {
		model = "gpt-4.1"
	}
	return &copilot{
		client:    httpClient(opts),
		githubAPI: apiURL,
		cred:      cred,
		model:     model,
	}, nil
}

func (c *copilot) Kind() string { return "copilot" }

type copilotSession struct {
	token   string
	chatAPI string
}

func (c *copilot) Complete(ctx context.Context, req Request) (string, error) {
	sess, err := c.session(ctx)
	if err != nil {
		return "", err
	}
	model := req.Model
	if model == "" {
		model = c.model
	}
	messages := []map[string]string{}
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.User})

	body := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   true,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(sess.chatAPI, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+sess.token)
	setCopilotHeaders(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("copilot chat: HTTP %d: %s", resp.StatusCode, truncateErr(data))
	}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") || strings.Contains(ct, "stream") {
		return readSSEContent(resp.Body)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return parseChatCompletion(data)
}

func (c *copilot) session(ctx context.Context) (copilotSession, error) {
	// Prefer token exchange (Business/Enterprise); fall back to /user discovery + GitHub token.
	if sess, err := c.exchangeToken(ctx); err == nil {
		return sess, nil
	}
	return c.discoverUser(ctx)
}

func (c *copilot) exchangeToken(ctx context.Context) (copilotSession, error) {
	url := c.githubAPI + "/copilot_internal/v2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return copilotSession{}, err
	}
	c.applyGitHubAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return copilotSession{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return copilotSession{}, err
	}
	if resp.StatusCode >= 300 {
		return copilotSession{}, fmt.Errorf("copilot token: HTTP %d: %s", resp.StatusCode, truncateErr(data))
	}
	var out struct {
		Token     string `json:"token"`
		Endpoints struct {
			API     string `json:"api"`
			ProxyEP string `json:"proxy-ep"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return copilotSession{}, err
	}
	if out.Token == "" {
		return copilotSession{}, fmt.Errorf("copilot token: empty token")
	}
	chat := strings.TrimSpace(out.Endpoints.API)
	if chat == "" {
		chat = chatAPIFromProxyEP(out.Endpoints.ProxyEP)
	}
	if chat == "" {
		chat = "https://api.githubcopilot.com"
	}
	return copilotSession{token: out.Token, chatAPI: chat}, nil
}

func (c *copilot) discoverUser(ctx context.Context) (copilotSession, error) {
	url := c.githubAPI + "/copilot_internal/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return copilotSession{}, err
	}
	c.applyGitHubAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return copilotSession{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return copilotSession{}, err
	}
	if resp.StatusCode >= 300 {
		return copilotSession{}, fmt.Errorf("copilot user: HTTP %d: %s", resp.StatusCode, truncateErr(data))
	}
	var out struct {
		Endpoints struct {
			API     string `json:"api"`
			ProxyEP string `json:"proxy-ep"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return copilotSession{}, err
	}
	chat := strings.TrimSpace(out.Endpoints.API)
	if chat == "" {
		chat = chatAPIFromProxyEP(out.Endpoints.ProxyEP)
	}
	if chat == "" {
		chat = "https://api.githubcopilot.com"
	}
	token := c.cred.Token
	if token == "" {
		return copilotSession{}, fmt.Errorf("copilot: cookie auth cannot call chat without session token; set token_env")
	}
	return copilotSession{token: token, chatAPI: chat}, nil
}

func (c *copilot) applyGitHubAuth(req *http.Request) {
	if c.cred.Cookie != "" {
		req.Header.Set("Cookie", c.cred.Cookie)
		return
	}
	if c.cred.Token != "" {
		req.Header.Set("Authorization", "token "+c.cred.Token)
	}
}

func chatAPIFromProxyEP(proxy string) string {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return ""
	}
	// proxy-ep looks like "proxy.enterprise.githubcopilot.com" → api.enterprise.githubcopilot.com
	host := strings.TrimPrefix(proxy, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	if strings.HasPrefix(host, "proxy.") {
		host = "api." + strings.TrimPrefix(host, "proxy.")
	}
	if !strings.HasPrefix(host, "http") {
		return "https://" + host
	}
	return host
}

func setCopilotHeaders(req *http.Request) {
	req.Header.Set("Editor-Version", "prui/0.1.0")
	req.Header.Set("Editor-Plugin-Version", "prui/0.1.0")
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("User-Agent", "GitHubCopilotChat/0.1.0")
	req.Header.Set("Openai-Intent", "conversation-panel")
}

func readSSEContent(r io.Reader) (string, error) {
	var b strings.Builder
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				b.WriteString(ch.Delta.Content)
			} else if ch.Message.Content != "" {
				b.WriteString(ch.Message.Content)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", fmt.Errorf("copilot: empty stream")
	}
	return text, nil
}

func parseChatCompletion(data []byte) (string, error) {
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("copilot: decode: %w", err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("copilot: empty response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
