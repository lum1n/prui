package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vegard/prui/internal/auth"
	"github.com/vegard/prui/internal/domain"
	"github.com/vegard/prui/internal/provider"
	"github.com/vegard/prui/internal/provider/bitbucketcloud"
	"github.com/vegard/prui/internal/provider/bitbucketdc"
	ghprov "github.com/vegard/prui/internal/provider/github"
)

func TestGitHubContract(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/acme/widgets/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"number": 1,
					"title":  "Hello",
					"state":  "open",
					"user":   map[string]any{"login": "alice"},
					"html_url": "http://example/pull/1",
					"head":   map[string]any{"sha": "abc"},
					"base":   map[string]any{"sha": "def"},
					"created_at": "2024-01-01T00:00:00Z",
					"updated_at": "2024-01-01T00:00:00Z",
				},
			})
			return
		}
	})
	mux.HandleFunc("/api/v3/repos/acme/widgets/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch || r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 1, "title": "Hello", "body": "- [x] one\n- [ ] two", "state": "open", "draft": false,
				"user": map[string]any{"login": "alice"},
				"html_url": "http://example/pull/1",
				"head": map[string]any{"sha": "abc"},
				"base": map[string]any{"sha": "def"},
				"created_at": "2024-01-01T00:00:00Z",
				"updated_at": "2024-01-01T00:00:00Z",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 1, "title": "Hello", "body": "- [ ] one\n- [x] two", "state": "open", "draft": true,
			"user": map[string]any{"login": "alice"},
			"html_url": "http://example/pull/1",
			"head": map[string]any{"sha": "abc"},
			"base": map[string]any{"sha": "def"},
			"created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-01-01T00:00:00Z",
		})
	})
	mux.HandleFunc("/api/v3/repos/acme/widgets/pulls/1/files", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"filename": "a.go", "status": "modified", "patch": "@@ -1 +1 @@\n-old\n+new\n"},
		})
	})
	mux.HandleFunc("/api/v3/repos/acme/widgets/pulls/1/comments", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{})
	})
	mux.HandleFunc("/api/v3/repos/acme/widgets/issues/1/comments", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{})
	})
	mux.HandleFunc("/api/v3/repos/acme/widgets/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 99, "state": "COMMENTED"})
			return
		}
		_ = json.NewEncoder(w).Encode([]any{})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host := domain.Host{
		Name: "ghe", Kind: domain.HostGitHub,
		BaseURL: srv.URL, APIURL: srv.URL + "/api/v3/",
	}
	client, err := ghprov.New(host, auth.Credentials{Token: "token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	runHostContract(t, client, domain.RepoRef{Owner: "acme", Name: "widgets"}, 1)
}

func TestBitbucketCloudContract(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/repositories/acme/widgets/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pullrequests/") {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{
				"id": 1, "title": "Hello", "state": "OPEN",
				"author": map[string]any{"nickname": "alice"},
				"source": map[string]any{"commit": map[string]any{"hash": "abc"}},
				"destination": map[string]any{"commit": map[string]any{"hash": "def"}},
				"links": map[string]any{"html": map[string]any{"href": "http://bb/1"}},
			}},
		})
	})
	mux.HandleFunc("/2.0/repositories/acme/widgets/pullrequests/1", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2.0/repositories/acme/widgets/pullrequests/1" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "title": "Hello", "description": "body", "state": "OPEN",
			"author": map[string]any{"nickname": "alice"},
			"source": map[string]any{"commit": map[string]any{"hash": "abc"}},
			"destination": map[string]any{"commit": map[string]any{"hash": "def"}},
			"links": map[string]any{"html": map[string]any{"href": "http://bb/1"}},
		})
	})
	mux.HandleFunc("/2.0/repositories/acme/widgets/pullrequests/1/diffstat", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{
				"status": "modified",
				"new":    map[string]any{"path": "a.go"},
				"old":    map[string]any{"path": "a.go"},
			}},
		})
	})
	mux.HandleFunc("/2.0/repositories/acme/widgets/pullrequests/1/diff/a.go", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n"))
	})
	mux.HandleFunc("/2.0/repositories/acme/widgets/pullrequests/1/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{}})
	})
	mux.HandleFunc("/2.0/repositories/acme/widgets/pullrequests/1/approve", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"approved": true})
	})
	mux.HandleFunc("/2.0/repositories/acme/widgets/pullrequests/1/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut || strings.Contains(r.URL.Path, "/tasks/") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 9, "state": "RESOLVED"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{
				"id": 9, "state": "UNRESOLVED",
				"content": map[string]any{"raw": "fix tests"},
				"creator": map[string]any{"nickname": "bob", "display_name": "Bob"},
			}},
		})
	})
	mux.HandleFunc("/2.0/repositories/acme/widgets/pullrequests/1/tasks/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 9, "state": "RESOLVED"})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host := domain.Host{Name: "bb", Kind: domain.HostBitbucketCloud, APIURL: srv.URL + "/2.0"}
	client, err := bitbucketcloud.New(host, auth.Credentials{Token: "tok"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	runHostContract(t, client, domain.RepoRef{Owner: "acme", Name: "widgets"}, 1)
}

func TestBitbucketDCContract(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/1.0/projects/PROJ/repos/repo/pull-requests" {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{
				"id": 1, "title": "Hello", "state": "OPEN",
				"author": map[string]any{"user": map[string]any{"slug": "alice", "displayName": "Alice Andersen"}},
				"fromRef": map[string]any{"latestCommit": "abc"},
				"toRef":   map[string]any{"latestCommit": "def"},
				"links":   map[string]any{"self": []map[string]any{{"href": "http://bb/1"}}},
			}},
		})
	})
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "title": "Hello", "description": "body", "state": "OPEN",
			"author":  map[string]any{"user": map[string]any{"slug": "alice"}},
			"fromRef": map[string]any{"latestCommit": "abc"},
			"toRef":   map[string]any{"latestCommit": "def"},
			"links":   map[string]any{"self": []map[string]any{{"href": "http://bb/1"}}},
		})
	})
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1/changes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{
				"type": "MODIFY",
				"path": map[string]any{"toString": "a.go"},
			}},
		})
	})
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1/diff/", func(w http.ResponseWriter, r *http.Request) {
		writeDCDiff(w)
	})
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1/diff", func(w http.ResponseWriter, r *http.Request) {
		writeDCDiff(w)
	})
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1/activities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{}})
	})
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1/comments", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	})
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1/blocker-comments", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{
				"id": 7, "text": "Address feedback", "state": "OPEN", "version": 1,
				"author": map[string]any{"slug": "bob", "displayName": "Bob"},
			}},
		})
	})
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1/blocker-comments/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "state": "RESOLVED", "version": 2})
	})
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/1/approve", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"approved": true})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host := domain.Host{
		Name: "bbdc", Kind: domain.HostBitbucketDC,
		BaseURL: srv.URL, APIURL: srv.URL + "/rest/api/1.0",
	}
	client, err := bitbucketdc.New(host, auth.Credentials{Token: "tok"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	runHostContract(t, client, domain.RepoRef{Owner: "PROJ", Name: "repo"}, 1)
}

func writeDCDiff(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"diffs": []map[string]any{{
			"hunks": []map[string]any{{
				"sourceLine": 1, "destinationLine": 1,
				"segments": []map[string]any{
					{"type": "REMOVED", "lines": []map[string]any{{"source": 1, "line": "old"}}},
					{"type": "ADDED", "lines": []map[string]any{{"destination": 1, "line": "new"}}},
				},
			}},
		}},
	})
}

func TestBitbucketDCCookieAuth(t *testing.T) {
	var sawCookie string
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/1.0/projects/PROJ/repos/repo/pull-requests", func(w http.ResponseWriter, r *http.Request) {
		sawCookie = r.Header.Get("Cookie")
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization with cookie auth")
		}
		if r.URL.Path != "/rest/api/1.0/projects/PROJ/repos/repo/pull-requests" {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host := domain.Host{
		Name: "bbdc", Kind: domain.HostBitbucketDC,
		BaseURL: srv.URL, APIURL: srv.URL + "/rest/api/1.0",
	}
	client, err := bitbucketdc.New(host, auth.Credentials{Cookie: "JSESSIONID=abc"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListPullRequests(context.Background(), domain.RepoRef{Owner: "PROJ", Name: "repo"}, domain.ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if sawCookie != "JSESSIONID=abc" {
		t.Fatalf("cookie %q", sawCookie)
	}
}

func TestGitHubCookieAuth(t *testing.T) {
	var sawCookie string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/acme/widgets/pulls", func(w http.ResponseWriter, r *http.Request) {
		sawCookie = r.Header.Get("Cookie")
		_ = json.NewEncoder(w).Encode([]any{})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host := domain.Host{
		Name: "ghe", Kind: domain.HostGitHub,
		BaseURL: srv.URL, APIURL: srv.URL + "/api/v3/",
	}
	client, err := ghprov.New(host, auth.Credentials{Cookie: "user_session=xyz"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListPullRequests(context.Background(), domain.RepoRef{Owner: "acme", Name: "widgets"}, domain.ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if sawCookie != "user_session=xyz" {
		t.Fatalf("cookie %q", sawCookie)
	}
}

func runHostContract(t *testing.T, h provider.Host, repo domain.RepoRef, num int) {
	t.Helper()
	ctx := context.Background()
	prs, err := h.ListPullRequests(ctx, repo, domain.ListOpts{State: "open"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(prs) == 0 {
		t.Fatal("expected PRs")
	}
	ref := domain.PRRef{Repo: repo, Number: num}
	pr, err := h.GetPullRequest(ctx, ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if pr.Title == "" {
		t.Fatal("empty title")
	}
	files, err := h.ListFiles(ctx, ref)
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected files")
	}
	fd, err := h.GetFileDiff(ctx, ref, files[0].Path)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if fd == nil || len(fd.Lines) == 0 {
		t.Fatal("expected diff lines")
	}
	if _, err := h.ListComments(ctx, ref); err != nil {
		t.Fatalf("comments: %v", err)
	}
	tasks, err := h.ListTasks(ctx, ref)
	if err != nil {
		t.Fatalf("tasks: %v", err)
	}
	if len(tasks) > 0 {
		if err := h.SetTaskDone(ctx, ref, tasks[0].ID, true); err != nil {
			t.Fatalf("set task: %v", err)
		}
	}
	draft, err := h.StartReview(ctx, ref)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = draft
	line := fd.Lines[0]
	for _, ln := range fd.Lines {
		if ln.Kind == domain.LineAdded || ln.Kind == domain.LineRemoved {
			line = ln
			break
		}
	}
	err = h.SubmitReview(ctx, ref, domain.DraftReview{
		Action: domain.ActionComment,
		Comments: []domain.DraftComment{{
			Body:   "nits",
			Path:   files[0].Path,
			Anchor: &line.Anchor,
		}},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
}
