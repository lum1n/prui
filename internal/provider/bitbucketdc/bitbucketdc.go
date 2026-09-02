package bitbucketdc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lum1n/prui/internal/auth"
	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
	"github.com/lum1n/prui/internal/provider/httputil"
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
	mode := domain.NormalizeListState(opts.State)
	state := "OPEN"
	switch mode {
	case "merged":
		state = "MERGED"
	case "closed":
		state = "DECLINED"
	case "all":
		state = "ALL"
	case "open", "draft":
		state = "OPEN"
	}
	u := fmt.Sprintf("%s/pull-requests?state=%s&limit=%d", c.repoPath(ref), url.QueryEscape(state), limit(opts.Limit))
	var page struct {
		Values []dcPR `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &page); err != nil {
		return nil, err
	}
	out := make([]domain.PullRequest, 0, len(page.Values))
	for _, p := range page.Values {
		pr := p.toDomain(ref, c.host.BaseURL)
		switch mode {
		case "open":
			if pr.Draft {
				continue
			}
		case "draft":
			if !pr.Draft {
				continue
			}
		}
		if st, err := c.reviewStatusFromPR(ctx, p); err == nil {
			pr.Reviews = st
		}
		out = append(out, pr)
	}
	return out, nil
}

func (c *Client) GetPullRequest(ctx context.Context, ref domain.PRRef) (*domain.PullRequest, error) {
	var p dcPR
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, c.prPath(ref), c.headers(), nil, &p); err != nil {
		return nil, err
	}
	pr := p.toDomain(ref.Repo, c.host.BaseURL)
	if st, err := c.reviewStatusFromPR(ctx, p); err == nil {
		pr.Reviews = st
	}
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
		path := ch.Path.String()
		oldPath := ch.SrcPath.String()
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
	var raw dcDiffResponse
	var errs []error

	// Try candidates in order. Dotfiles (.env) must percent-encode "." as %2E —
	// PathEscape leaves dots alone, and many proxies reset connections on "/.env".
	for _, u := range diffURLs(c.prPath(ref), path) {
		if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &raw); err != nil {
			errs = append(errs, err)
			continue
		}
		unified := dcDiffToUnified(path, raw)
		if strings.TrimSpace(unified) == "" || !strings.Contains(unified, "@@") {
			return &domain.FileDiff{Path: path, Status: domain.FileModified, Raw: unified}, nil
		}
		return diff.ParseUnified(path, unified)
	}
	return nil, fmt.Errorf("diff %s: %w", path, errors.Join(errs...))
}

func diffURLs(prBase, path string) []string {
	q := encodeDiffQueryPath(path)
	p := encodeDiffPath(path)
	out := []string{
		fmt.Sprintf("%s/diff?path=%s&contextLines=5", prBase, q),
		fmt.Sprintf("%s/diff/%s?contextLines=5", prBase, p),
	}
	// Also try unencoded query form for older DC versions.
	plain := url.QueryEscape(path)
	if plain != q {
		out = append(out, fmt.Sprintf("%s/diff?path=%s&contextLines=5", prBase, plain))
	}
	return out
}

// encodeDiffPath keeps slashes so Bitbucket's {path:.*} matcher works.
// Leading/embedded "." become %2E so names like .env survive proxies and routing.
func encodeDiffPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = encodePathSegment(p)
	}
	return strings.Join(parts, "/")
}

func encodeDiffQueryPath(path string) string {
	// QueryEscape turns "/" into %2F but leaves "."; force-encode dots.
	return strings.ReplaceAll(url.QueryEscape(path), ".", "%2E")
}

func encodePathSegment(p string) string {
	return strings.ReplaceAll(url.PathEscape(p), ".", "%2E")
}

func (c *Client) ListComments(ctx context.Context, ref domain.PRRef) ([]domain.Comment, error) {
	u := c.prPath(ref) + "/activities?limit=100"
	var page struct {
		Values []dcActivity `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &page); err != nil {
		return c.listCommentsEndpoint(ctx, ref)
	}
	out := make([]domain.Comment, 0)
	for _, a := range page.Values {
		if a.Action != "COMMENTED" || a.Comment == nil {
			continue
		}
		out = append(out, a.Comment.flatten("", a.CommentAnchor)...)
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
	out := make([]domain.Comment, 0)
	for _, cm := range page.Values {
		out = append(out, cm.flatten("", nil)...)
	}
	return out, nil
}

func (c *Client) StartReview(ctx context.Context, ref domain.PRRef) (*domain.DraftReview, error) {
	return &domain.DraftReview{}, nil
}

func (c *Client) SubmitReview(ctx context.Context, ref domain.PRRef, draft domain.DraftReview) error {
	for _, d := range draft.Comments {
		body := map[string]any{"text": d.Body}
		if d.ParentID != "" {
			id, err := strconv.Atoi(d.ParentID)
			if err != nil {
				return fmt.Errorf("parent id: %w", err)
			}
			body["parent"] = map[string]any{"id": id}
		} else if d.Anchor != nil {
			body["anchor"] = buildDCAnchor(d.Anchor)
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
	if draft.Action == domain.ActionRequestChanges {
		return c.RequestChanges(ctx, ref)
	}
	return nil
}

// buildDCAnchor maps a domain anchor to Bitbucket DC's comment anchor.
// Multi-line ranges (DC ≥ 9.3) use multilineMarker + multilineSpan; line is the end.
func buildDCAnchor(a *domain.Anchor) map[string]any {
	lineType := string(a.LineType)
	if lineType == "" {
		if a.Side == domain.SideLeft {
			lineType = "REMOVED"
		} else {
			lineType = "ADDED"
		}
	}
	fileType := "TO"
	if a.Side == domain.SideLeft || lineType == "REMOVED" {
		fileType = "FROM"
	}

	start, end := a.Line, a.EndLine
	if end <= 0 || end < start {
		end = start
	}
	if start <= 0 {
		start = end
	}

	anchor := map[string]any{
		"path":     a.Path,
		"diffType": "EFFECTIVE",
		"line":     end,
		"lineType": lineType,
		"fileType": fileType,
	}
	if end > start {
		// Marker sits on the first line; `line` is the last line of the span.
		anchor["multilineMarker"] = map[string]any{
			"startLine":     start,
			"startLineType": lineType,
		}
		if fileType == "FROM" {
			anchor["multilineSpan"] = map[string]any{
				"srcSpanStart": start,
				"srcSpanEnd":   end,
			}
		} else {
			anchor["multilineSpan"] = map[string]any{
				"dstSpanStart": start,
				"dstSpanEnd":   end,
			}
		}
	}
	return anchor
}

func (c *Client) Approve(ctx context.Context, ref domain.PRRef) error {
	_, err := httputil.DoJSON(ctx, c.http, http.MethodPost, c.prPath(ref)+"/approve", c.headers(), nil, nil)
	return err
}

func (c *Client) Unapprove(ctx context.Context, ref domain.PRRef) error {
	_, err := httputil.DoJSON(ctx, c.http, http.MethodDelete, c.prPath(ref)+"/approve", c.headers(), nil, nil)
	return err
}

func (c *Client) RequestChanges(ctx context.Context, ref domain.PRRef) error {
	slug, err := c.viewerSlug(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{"status": "NEEDS_WORK"}
	_, err = httputil.DoJSON(ctx, c.http, http.MethodPut, c.prPath(ref)+"/participants/"+url.PathEscape(slug), c.headers(), body, nil)
	return err
}

func (c *Client) GetReviewStatus(ctx context.Context, ref domain.PRRef) (domain.ReviewStatus, error) {
	var p dcPR
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, c.prPath(ref), c.headers(), nil, &p); err != nil {
		return domain.ReviewStatus{}, err
	}
	return c.reviewStatusFromPR(ctx, p)
}

func (c *Client) reviewStatusFromPR(ctx context.Context, p dcPR) (domain.ReviewStatus, error) {
	st := domain.ReviewStatus{}
	login, aliases := c.resolveViewer(ctx)
	st.ViewerLogin = login
	st.ViewerAliases = aliases
	for _, r := range p.Reviewers {
		login := r.User.Slug
		if login == "" {
			login = r.User.Name
		}
		dec := domain.DecisionNone
		switch strings.ToUpper(r.Status) {
		case "APPROVED":
			dec = domain.DecisionApproved
		case "NEEDS_WORK":
			dec = domain.DecisionChangesRequested
		default:
			if r.Approved {
				dec = domain.DecisionApproved
			} else {
				continue
			}
		}
		st.Reviewers = append(st.Reviewers, domain.Reviewer{
			Login:    login,
			Name:     r.User.DisplayName,
			Decision: dec,
		})
	}
	st.Normalize()
	return st, nil
}

func (c *Client) resolveViewer(ctx context.Context) (login string, aliases []string) {
	if c.cred.Username != "" {
		aliases = append(aliases, c.cred.Username)
		var u struct {
			Slug        string `json:"slug"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		}
		uURL := c.api + "/users/" + url.PathEscape(c.cred.Username)
		if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, uURL, c.headers(), nil, &u); err == nil {
			login = firstNonEmpty(u.Slug, u.Name, c.cred.Username)
			if u.DisplayName != "" {
				aliases = append(aliases, u.DisplayName)
			}
			if u.Name != "" && u.Name != login {
				aliases = append(aliases, u.Name)
			}
			if u.Slug != "" && u.Slug != login {
				aliases = append(aliases, u.Slug)
			}
			return login, aliases
		}
		login = c.cred.Username
	}
	// Cookie sessions: whoami servlet returns the username as plain text on many DC installs.
	if base := strings.TrimRight(c.host.BaseURL, "/"); base != "" {
		who := base + "/plugins/servlet/applinks/whoami"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, who, nil)
		if err == nil {
			for k, v := range c.headers() {
				req.Header.Set(k, v)
			}
			resp, err := c.http.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode < 300 {
					data, _ := io.ReadAll(resp.Body)
					id := strings.TrimSpace(string(data))
					if id != "" && !strings.Contains(id, "<") {
						if login == "" {
							login = id
						} else if !strings.EqualFold(login, id) {
							aliases = append(aliases, id)
						}
					}
				}
			}
		}
	}
	return login, aliases
}

func (c *Client) viewerSlug(ctx context.Context) (string, error) {
	login, _ := c.resolveViewer(ctx)
	if login != "" {
		return login, nil
	}
	return "", fmt.Errorf("could not resolve current user slug (set hosts[].username)")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (c *Client) ListTasks(ctx context.Context, ref domain.PRRef) ([]domain.Task, error) {
	tasks, err := c.listBlockerComments(ctx, ref)
	if err == nil {
		return tasks, nil
	}
	// Legacy /tasks on older Bitbucket Server.
	legacy, lerr := c.listLegacyTasks(ctx, ref)
	if lerr == nil {
		return legacy, nil
	}
	return nil, err
}

func (c *Client) listBlockerComments(ctx context.Context, ref domain.PRRef) ([]domain.Task, error) {
	u := c.prPath(ref) + "/blocker-comments?limit=200"
	var page struct {
		Values []dcBlockerComment `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &page); err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(page.Values))
	for _, t := range page.Values {
		out = append(out, t.toDomain())
	}
	return out, nil
}

func (c *Client) listLegacyTasks(ctx context.Context, ref domain.PRRef) ([]domain.Task, error) {
	u := c.prPath(ref) + "/tasks?limit=200"
	var page struct {
		Values []dcLegacyTask `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &page); err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(page.Values))
	for _, t := range page.Values {
		out = append(out, t.toDomain())
	}
	return out, nil
}

func (c *Client) SetTaskDone(ctx context.Context, ref domain.PRRef, taskID string, done bool) error {
	id, err := strconv.Atoi(taskID)
	if err != nil {
		return fmt.Errorf("task id: %w", err)
	}
	state := "OPEN"
	if done {
		state = "RESOLVED"
	}
	// Need current version for optimistic lock.
	tasks, err := c.ListTasks(ctx, ref)
	if err != nil {
		return err
	}
	version := 0
	found := false
	for _, t := range tasks {
		if t.ID == taskID {
			version = t.Version
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("task %s not found", taskID)
	}
	body := map[string]any{
		"id":      id,
		"version": version,
		"state":   state,
	}
	u := fmt.Sprintf("%s/blocker-comments/%d", c.prPath(ref), id)
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodPut, u, c.headers(), body, nil); err != nil {
		// Fallback legacy task endpoint.
		legacy := fmt.Sprintf("%s/tasks/%d", c.api, id)
		_, lerr := httputil.DoJSON(ctx, c.http, http.MethodPut, legacy, c.headers(), body, nil)
		if lerr != nil {
			return err
		}
	}
	return nil
}

type dcBlockerComment struct {
	ID      int    `json:"id"`
	Text    string `json:"text"`
	State   string `json:"state"`
	Version int    `json:"version"`
	Author  struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Slug        string `json:"slug"`
	} `json:"author"`
}

func (t dcBlockerComment) toDomain() domain.Task {
	login := t.Author.Slug
	if login == "" {
		login = t.Author.Name
	}
	return domain.Task{
		ID:       strconv.Itoa(t.ID),
		Body:     t.Text,
		Done:     strings.EqualFold(t.State, "RESOLVED"),
		Author:   domain.FormatAuthor(login, t.Author.DisplayName),
		Required: true,
		Version:  t.Version,
	}
}

type dcLegacyTask struct {
	ID      int    `json:"id"`
	Text    string `json:"text"`
	State   string `json:"state"`
	Version int    `json:"version"`
	Author  struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Slug        string `json:"slug"`
	} `json:"author"`
}

func (t dcLegacyTask) toDomain() domain.Task {
	login := t.Author.Slug
	if login == "" {
		login = t.Author.Name
	}
	return domain.Task{
		ID:       strconv.Itoa(t.ID),
		Body:     t.Text,
		Done:     strings.EqualFold(t.State, "RESOLVED"),
		Author:   domain.FormatAuthor(login, t.Author.DisplayName),
		Required: true,
		Version:  t.Version,
	}
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
	Draft       bool   `json:"draft"`
	Author      struct {
		User struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Slug        string `json:"slug"`
		} `json:"user"`
	} `json:"author"`
	Reviewers []dcReviewer `json:"reviewers"`
	FromRef   struct {
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

type dcReviewer struct {
	User struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Slug        string `json:"slug"`
	} `json:"user"`
	Role     string `json:"role"`
	Approved bool   `json:"approved"`
	Status   string `json:"status"`
}

func (p dcPR) toDomain(repo domain.RepoRef, baseURL string) domain.PullRequest {
	login := p.Author.User.Slug
	if login == "" {
		login = p.Author.User.Name
	}
	author := domain.FormatAuthor(login, p.Author.User.DisplayName)
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
		Draft:     p.Draft,
		URL:       urlStr,
		HeadSHA:   p.FromRef.LatestCommit,
		BaseSHA:   p.ToRef.LatestCommit,
		CreatedAt: time.UnixMilli(p.CreatedDate),
		UpdatedAt: time.UnixMilli(p.UpdatedDate),
	}
}

type dcChange struct {
	Type    string `json:"type"`
	Path    dcPath `json:"path"`
	SrcPath dcPath `json:"srcPath"`
}

type dcPath struct {
	ToString   string   `json:"toString"`
	Components []string `json:"components"`
}

func (p dcPath) String() string {
	if p.ToString != "" {
		return p.ToString
	}
	if len(p.Components) > 0 {
		return strings.Join(p.Components, "/")
	}
	return ""
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
	Source      *dcPath  `json:"source"`
	Destination *dcPath  `json:"destination"`
	Hunks       []dcHunk `json:"hunks"`
}

func (d dcDiff) path() string {
	if d.Destination != nil {
		if s := d.Destination.String(); s != "" {
			return s
		}
	}
	if d.Source != nil {
		return d.Source.String()
	}
	return ""
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

func dcDiffToUnified(wantPath string, raw dcDiffResponse) string {
	var b strings.Builder
	wroteHeader := false
	for _, d := range raw.Diffs {
		p := d.path()
		if p == "" {
			p = wantPath
		}
		// Single-file requests must not leak other paths from a whole-PR response.
		if wantPath != "" && p != wantPath {
			continue
		}
		if !wroteHeader {
			b.WriteString("diff --git a/" + p + " b/" + p + "\n")
			b.WriteString("--- a/" + p + "\n")
			b.WriteString("+++ b/" + p + "\n")
			wroteHeader = true
		}
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
	Action        string           `json:"action"`
	Comment       *dcComment       `json:"comment"`
	CommentAnchor *dcCommentAnchor `json:"commentAnchor"`
}

type dcComment struct {
	ID      int    `json:"id"`
	Text    string `json:"text"`
	Author  struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Slug        string `json:"slug"`
	} `json:"author"`
	Anchor      *dcCommentAnchor `json:"anchor"`
	Comments    []dcComment      `json:"comments"`
	CreatedDate int64            `json:"createdDate"`
}

type dcCommentAnchor struct {
	Path            string             `json:"path"`
	Line            int                `json:"line"`
	LineType        string             `json:"lineType"`
	FileType        string             `json:"fileType"`
	MultilineMarker *dcMultilineMarker `json:"multilineMarker"`
}

type dcMultilineMarker struct {
	StartLine     int    `json:"startLine"`
	StartLineType string `json:"startLineType"`
}

func (cm dcComment) flatten(parentID string, inherit *dcCommentAnchor) []domain.Comment {
	anchor := cm.Anchor
	if anchor == nil {
		anchor = inherit
	}
	c := cm.toDomainWith(parentID, anchor)
	out := []domain.Comment{c}
	for _, reply := range cm.Comments {
		out = append(out, reply.flatten(c.ID, anchor)...)
	}
	return out
}

func (cm dcComment) toDomain() domain.Comment {
	return cm.toDomainWith("", cm.Anchor)
}

func (cm dcComment) toDomainWith(parentID string, anchor *dcCommentAnchor) domain.Comment {
	login := cm.Author.Slug
	if login == "" {
		login = cm.Author.Name
	}
	c := domain.Comment{
		ID:       strconv.Itoa(cm.ID),
		Body:     cm.Text,
		Author:   domain.FormatAuthor(login, cm.Author.DisplayName),
		ParentID: parentID,
		Created:  time.UnixMilli(cm.CreatedDate),
	}
	if anchor != nil {
		c.Path = anchor.Path
		side := domain.SideRight
		lt := domain.LineType(anchor.LineType)
		if anchor.FileType == "FROM" || lt == domain.LineRemoved {
			side = domain.SideLeft
		}
		a := &domain.Anchor{
			Path:     anchor.Path,
			Side:     side,
			Line:     anchor.Line,
			LineType: lt,
		}
		if m := anchor.MultilineMarker; m != nil && m.StartLine > 0 && m.StartLine < anchor.Line {
			a.Line = m.StartLine
			a.EndLine = anchor.Line
			if m.StartLineType != "" {
				a.LineType = domain.LineType(m.StartLineType)
			}
		}
		c.Anchor = a
	}
	return c
}
