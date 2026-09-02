package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/vegard/prui/internal/ai"
	"github.com/vegard/prui/internal/config"
	"github.com/vegard/prui/internal/domain"
)

func TestClaudeComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("x-api-key") != "test-key" {
			http.Error(w, "bad key", 401)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "claude-test" {
			http.Error(w, "bad model", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "Summary OK"}},
		})
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	c, err := ai.New(ai.Options{
		Provider: config.AIProvider{
			Kind:    "claude",
			Model:   "claude-test",
			BaseURL: srv.URL,
		},
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Complete(context.Background(), ai.Request{
		System: "sys",
		User:   "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "Summary OK" {
		t.Fatalf("got %q", out)
	}
}

func TestCopilotSSEStream(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/copilot_internal/v2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":     "tid=session",
			"endpoints": map[string]string{"api": srv.URL},
		})
	})
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tid=session" {
			http.Error(w, "bad bearer", 401)
			return
		}
		if r.Header.Get("Editor-Version") == "" {
			http.Error(w, "missing editor", 400)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("GITHUB_TOKEN", "gho_test")
	c, err := ai.New(ai.Options{
		Provider:   config.AIProvider{Kind: "copilot", APIURL: srv.URL},
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Complete(context.Background(), ai.Request{User: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "Hi" {
		t.Fatalf("got %q", out)
	}
}

func TestCopilotCloudAndGHE(t *testing.T) {
	run := func(t *testing.T, githubPath string, useCookie bool) {
		t.Helper()
		var sawChat bool
		var srv *httptest.Server
		mux := http.NewServeMux()
		mux.HandleFunc(githubPath+"/copilot_internal/v2/token", func(w http.ResponseWriter, r *http.Request) {
			if useCookie {
				if r.Header.Get("Cookie") == "" {
					http.Error(w, "need cookie", 401)
					return
				}
			} else if !strings.Contains(r.Header.Get("Authorization"), "token ") {
				http.Error(w, "need token", 401)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token": "tid=abc",
				"endpoints": map[string]string{
					"api": srv.URL,
				},
			})
		})
		mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
			sawChat = true
			if r.Header.Get("Authorization") != "Bearer tid=abc" {
				http.Error(w, "bad auth", 401)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"content": "GHE summary"}},
				},
			})
		})
		srv = httptest.NewServer(mux)
		defer srv.Close()

		apiURL := srv.URL + githubPath
		prov := config.AIProvider{Kind: "copilot", Model: "gpt-4.1", APIURL: apiURL}
		if useCookie {
			t.Setenv("GHE_COOKIE", "user_session=abc")
			cfg := &config.Config{
				Hosts: []config.HostConfig{{
					Name:      "work-ghe",
					Kind:      "github",
					BaseURL:   srv.URL,
					APIURL:    apiURL,
					CookieEnv: "GHE_COOKIE",
				}},
			}
			c, err := ai.New(ai.Options{
				Provider: config.AIProvider{
					Kind:       "copilot",
					Model:      "gpt-4.1",
					GitHubHost: "work-ghe",
				},
				Config:     cfg,
				HTTPClient: srv.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Complete(context.Background(), ai.Request{User: "sum"})
			if err != nil {
				t.Fatal(err)
			}
			if out != "GHE summary" || !sawChat {
				t.Fatalf("out=%q sawChat=%v", out, sawChat)
			}
			return
		}

		t.Setenv("GITHUB_TOKEN", "gho_x")
		c, err := ai.New(ai.Options{
			Provider:   prov,
			HTTPClient: srv.Client(),
		})
		if err != nil {
			t.Fatal(err)
		}
		out, err := c.Complete(context.Background(), ai.Request{User: "sum"})
		if err != nil {
			t.Fatal(err)
		}
		if out != "GHE summary" || !sawChat {
			t.Fatalf("out=%q sawChat=%v", out, sawChat)
		}
	}

	t.Run("cloud", func(t *testing.T) { run(t, "", false) })
	t.Run("ghe", func(t *testing.T) { run(t, "/api/v3", true) })
}

func TestCopilotUserFallback(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/copilot_internal/v2/token", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", 404)
	})
	mux.HandleFunc("/copilot_internal/user", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"endpoints": map[string]string{"api": srv.URL},
		})
	})
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gho_direct" {
			http.Error(w, "bad", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "fallback"}},
			},
		})
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("GITHUB_TOKEN", "gho_direct")
	c, err := ai.New(ai.Options{
		Provider:   config.AIProvider{Kind: "copilot", APIURL: srv.URL},
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Complete(context.Background(), ai.Request{User: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "fallback" {
		t.Fatalf("got %q", out)
	}
}

func TestNewUnknownKind(t *testing.T) {
	_, err := ai.New(ai.Options{Provider: config.AIProvider{Kind: "nope"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveActiveGitHubHost(t *testing.T) {
	t.Setenv("GHE_TOKEN", "tok")
	c, err := ai.New(ai.Options{
		Provider: config.AIProvider{Kind: "copilot"},
		ActiveHost: domain.Host{
			Name:     "work-ghe",
			Kind:     domain.HostGitHub,
			APIURL:   "https://ghe.example.com/api/v3",
			TokenEnv: "GHE_TOKEN",
		},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, os.ErrClosed
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind() != "copilot" {
		t.Fatal(c.Kind())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
