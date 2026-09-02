package bitbucketdc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vegard/prui/internal/auth"
	"github.com/vegard/prui/internal/diff"
	"github.com/vegard/prui/internal/domain"
	"github.com/vegard/prui/internal/provider/httputil"
)

// Client implements provider.Host for Bitbucket Data Center / Server.
type Client struct {
	host domain.Host
	cred auth.Credentials
	http *http.Client
	api  string
}

// New constructs a Bitbucket DC adapter.
func New(host domain.Host, cred auth.Credentials, httpClient *http.Client) (*Client, error) {
	base := strings.TrimRight(host.BaseURL, "/")
	api := host.APIURL
	if api == "" {
		api = base + "/rest/api/1.0"
	}
	api = strings.TrimRight(api, "/")
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{host: host, cred: cred, http: httpClient, api: api}, nil
}

func (c *Client) Kind() domain.HostKind { return domain.HostBitbucketDC }

func (c *Client) headers() map[string]string {
	h := map[string]string{"Accept": "application/json"}
	if c.cred.Cookie != "" {
		h["Cookie"] = c.cred.Cookie
		return h
	}
	if c.cred.Username != "" {
		h["Authorization"] = "Basic " + basicAuth(c.cred.Username, c.cred.Token)
	} else if c.cred.Token != "" {
		h["Authorization"] = "Bearer " + c.cred.Token
	}
	return h
}

func basicAuth(user, pass string) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	in := []byte(user + ":" + pass)
	var out strings.Builder
	for i := 0; i < len(in); i += 3 {
		var n uint32
		remain := len(in) - i
		switch {
		case remain >= 3:
			n = uint32(in[i])<<16 | uint32(in[i+1])<<8 | uint32(in[i+2])
			out.WriteByte(table[n>>18&0x3f])
			out.WriteByte(table[n>>12&0x3f])
			out.WriteByte(table[n>>6&0x3f])
			out.WriteByte(table[n&0x3f])
		case remain == 2:
			n = uint32(in[i])<<16 | uint32(in[i+1])<<8
			out.WriteByte(table[n>>18&0x3f])
			out.WriteByte(table[n>>12&0x3f])
			out.WriteByte(table[n>>6&0x3f])
			out.WriteByte('=')
		case remain == 1:
			n = uint32(in[i]) << 16
			out.WriteByte(table[n>>18&0x3f])
			out.WriteByte(table[n>>12&0x3f])
			out.WriteByte('=')
			out.WriteByte('=')
		}
	}
	return out.String()
}

func (c *Client) repoPath(ref domain.RepoRef) string {
	return fmt.Sprintf("%s/projects/%s/repos/%s", c.api, url.PathEscape(ref.Owner), url.PathEscape(ref.Name))
}

func (c *Client) prPath(ref domain.PRRef) string {
	return fmt.Sprintf("%s/pull-requests/%d", c.repoPath(ref.Repo), ref.Number)
}

func (c *Client) ListPullRequests(ctx context.Context, ref domain.RepoRef, opts domain.ListOpts) ([]domain.PullRequest, error) {
	state := opts.State
	if state == "" {
		state = "OPEN"
	}
	state = strings.ToUpper(state)
	u := fmt.Sprintf("%s/pull-requests?state=%s&limit=%d", c.repoPath(ref), url.QueryEscape(state), limit(opts.Limit))
	var page struct {
		Values []dcPR `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &page); err != nil {
		return nil, err
	}
	out := make([]domain.PullRequest, 0, len(page.Values))
	for _, p := range page.Values {
		out = append(out, p.toDomain(ref, c.host.BaseURL))
	}
	return out, nil
}

func (c *Client) GetPullRequest(ctx context.Context, ref domain.PRRef) (*domain.PullRequest, error) {
	var p dcPR
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, c.prPath(ref), c.headers(), nil, &p); err != nil {
		return nil, err
	}
	pr := p.toDomain(ref.Repo, c.host.BaseURL)
	return &pr, nil
}

func (c *Client) ListFiles(ctx context.Context, ref domain.PRRef) ([]domain.FileChange, error) {
	u := c.prPath(ref) + "/changes?limit=1000"
	var page struct {
		Values []dcChange `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &page); err != nil {
		return nil, err
	}
	out := make([]domain.FileChange, 0, len(page.Values))
	for _, ch := range page.Values {
		path := ch.Path.ToString
		oldPath := ch.SrcPath.ToString
		st := domain.FileModified
		switch strings.ToUpper(ch.Type) {
		case "ADD":
			st = domain.FileAdded
		case "DELETE":
			st = domain.FileRemoved
		case "MOVE", "COPY":
			st = domain.FileRenamed
		}
		out = append(out, domain.FileChange{Path: path, OldPath: oldPath, Status: st})
	}
	return out, nil
}

func (c *Client) GetFileDiff(ctx context.Context, ref domain.PRRef, path string) (*domain.FileDiff, error) {
	// DC raw diff: /pull-requests/{id}/diff?path=
	u := fmt.Sprintf("%s/diff?path=%s&contextLines=3", c.prPath(ref), url.QueryEscape(path))
	var raw dcDiffResponse
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &raw); err != nil {
		return nil, err
	}
	unified := dcDiffToUnified(path, raw)
	return diff.ParseUnified(path, unified)
}

func (c *Client) ListComments(ctx context.Context, ref domain.PRRef) ([]domain.Comment, error) {
	u := c.prPath(ref) + "/activities?limit=100"
	var page struct {
		Values []dcActivity `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &page); err != nil {
		// fallback comments endpoint
		return c.listCommentsEndpoint(ctx, ref)
	}
	out := make([]domain.Comment, 0)
	for _, a := range page.Values {
		if a.Action != "COMMENTED" || a.Comment == nil {
			continue
		}
		out = append(out, a.Comment.toDomain())
	}
	return out, nil
}

func (c *Client) listCommentsEndpoint(ctx context.Context, ref domain.PRRef) ([]domain.Comment, error) {
	u := c.prPath(ref) + "/comments?limit=200"
	var page struct {
		Values []dcComment `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &page); err != nil {
		return nil, err
	}
	out := make([]domain.Comment, 0, len(page.Values))
	for _, cm := range page.Values {
		out = append(out, cm.toDomain())
	}
	return out, nil
}

func (c *Client) StartReview(ctx context.Context, ref domain.PRRef) (*domain.DraftReview, error) {
	return &domain.DraftReview{}, nil
}

func (c *Client) SubmitReview(ctx context.Context, ref domain.PRRef, draft domain.DraftReview) error {
	for _, d := range draft.Comments {
		body := map[string]any{"text": d.Body}
		if d.Anchor != nil {
			lineType := string(d.Anchor.LineType)
			if lineType == "" {
				if d.Anchor.Side == domain.SideLeft {
					lineType = "REMOVED"
				} else {
					lineType = "ADDED"
				}
			}
			body["anchor"] = map[string]any{
				"path":     d.Anchor.Path,
				"line":     d.Anchor.Line,
				"lineType": lineType,
				"fileType": "FROM",
			}
			if d.Anchor.Side == domain.SideRight {
				body["anchor"].(map[string]any)["fileType"] = "TO"
			}
		}
		if _, err := httputil.DoJSON(ctx, c.http, http.MethodPost, c.prPath(ref)+"/comments", c.headers(), body, nil); err != nil {
			return fmt.Errorf("post comment: %w", err)
		}
	}
	if draft.Summary != "" {
		body := map[string]any{"text": draft.Summary}
		if _, err := httputil.DoJSON(ctx, c.http, http.MethodPost, c.prPath(ref)+"/comments", c.headers(), body, nil); err != nil {
			return fmt.Errorf("post summary: %w", err)
		}
	}
	if draft.Action == domain.ActionApprove {
		return c.Approve(ctx, ref)
	}
	return nil
}

func (c *Client) Approve(ctx context.Context, ref domain.PRRef) error {
	_, err := httputil.DoJSON(ctx, c.http, http.MethodPost, c.prPath(ref)+"/approve", c.headers(), nil, nil)
	return err
}

func (c *Client) Unapprove(ctx context.Context, ref domain.PRRef) error {
	_, err := httputil.DoJSON(ctx, c.http, http.MethodDelete, c.prPath(ref)+"/approve", c.headers(), nil, nil)
	return err
}

func limit(n int) int {
	if n <= 0 || n > 100 {
		return 50
	}
	return n
}

type dcPR struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	Author      struct {
		User struct {
			Name         string `json:"name"`
			DisplayName  string `json:"displayName"`
			Slug         string `json:"slug"`
		} `json:"user"`
	} `json:"author"`
	FromRef struct {
		LatestCommit string `json:"latestCommit"`
	} `json:"fromRef"`
	ToRef struct {
		LatestCommit string `json:"latestCommit"`
	} `json:"toRef"`
	Links struct {
		Self []struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
	CreatedDate int64 `json:"createdDate"`
	UpdatedDate int64 `json:"updatedDate"`
}

func (p dcPR) toDomain(repo domain.RepoRef, baseURL string) domain.PullRequest {
	author := p.Author.User.Slug
	if author == "" {
		author = p.Author.User.Name
	}
	urlStr := ""
	if len(p.Links.Self) > 0 {
		urlStr = p.Links.Self[0].Href
	}
	if urlStr == "" && baseURL != "" {
		urlStr = fmt.Sprintf("%s/projects/%s/repos/%s/pull-requests/%d", strings.TrimRight(baseURL, "/"), repo.Owner, repo.Name, p.ID)
	}
	return domain.PullRequest{
		Ref:       domain.PRRef{Repo: repo, Number: p.ID},
		Title:     p.Title,
		Body:      p.Description,
		Author:    author,
		State:     strings.ToLower(p.State),
		URL:       urlStr,
		HeadSHA:   p.FromRef.LatestCommit,
		BaseSHA:   p.ToRef.LatestCommit,
		CreatedAt: time.UnixMilli(p.CreatedDate),
		UpdatedAt: time.UnixMilli(p.UpdatedDate),
	}
}

type dcChange struct {
	Type    string `json:"type"`
	Path    struct{ ToString string `json:"toString"` } `json:"path"`
	SrcPath struct{ ToString string `json:"toString"` } `json:"srcPath"`
}

// flexInt unmarshals JSON numbers or numeric strings.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = 0
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("flexInt: %w", err)
		}
		*f = flexInt(n)
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

func (f flexInt) Int() int { return int(f) }

type dcDiffResponse struct {
	Diffs []dcDiff `json:"diffs"`
}

type dcDiff struct {
	Hunks []dcHunk `json:"hunks"`
}

type dcHunk struct {
	SourceLine      flexInt     `json:"sourceLine"`
	DestinationLine flexInt     `json:"destinationLine"`
	Segments        []dcSegment `json:"segments"`
}

type dcSegment struct {
	Type  string   `json:"type"`
	Lines []dcLine `json:"lines"`
}

type dcLine struct {
	// Line is the line *content* in Bitbucket DC (not a line number).
	Line        string  `json:"line"`
	Source      flexInt `json:"source"`
	Destination flexInt `json:"destination"`
	Truncated   bool    `json:"truncated"`
}

func dcDiffToUnified(path string, raw dcDiffResponse) string {
	var b strings.Builder
	b.WriteString("diff --git a/" + path + " b/" + path + "\n")
	b.WriteString("--- a/" + path + "\n")
	b.WriteString("+++ b/" + path + "\n")
	for _, d := range raw.Diffs {
		for _, h := range d.Hunks {
			oldStart := h.SourceLine.Int()
			newStart := h.DestinationLine.Int()
			if oldStart == 0 {
				oldStart = 1
			}
			if newStart == 0 {
				newStart = 1
			}
			b.WriteString(fmt.Sprintf("@@ -%d +%d @@\n", oldStart, newStart))
			for _, seg := range h.Segments {
				prefix := " "
				switch strings.ToUpper(seg.Type) {
				case "ADDED":
					prefix = "+"
				case "REMOVED":
					prefix = "-"
				}
				for _, ln := range seg.Lines {
					b.WriteString(prefix + ln.Line + "\n")
				}
			}
		}
	}
	return b.String()
}

type dcActivity struct {
	Action  string     `json:"action"`
	Comment *dcComment `json:"comment"`
}

type dcComment struct {
	ID      int    `json:"id"`
	Text    string `json:"text"`
	Author  struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Slug        string `json:"slug"`
	} `json:"author"`
	Anchor *struct {
		Path     string `json:"path"`
		Line     int    `json:"line"`
		LineType string `json:"lineType"`
		FileType string `json:"fileType"`
	} `json:"anchor"`
	CreatedDate int64 `json:"createdDate"`
}

func (cm dcComment) toDomain() domain.Comment {
	author := cm.Author.Slug
	if author == "" {
		author = cm.Author.Name
	}
	c := domain.Comment{
		ID:      strconv.Itoa(cm.ID),
		Body:    cm.Text,
		Author:  author,
		Created: time.UnixMilli(cm.CreatedDate),
	}
	if cm.Anchor != nil {
		c.Path = cm.Anchor.Path
		side := domain.SideRight
		lt := domain.LineType(cm.Anchor.LineType)
		if cm.Anchor.FileType == "FROM" || lt == domain.LineRemoved {
			side = domain.SideLeft
		}
		c.Anchor = &domain.Anchor{
			Path:     cm.Anchor.Path,
			Side:     side,
			Line:     cm.Anchor.Line,
			LineType: lt,
		}
	}
	return c
}
