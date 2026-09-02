package bitbucketcloud

import (
	"context"
	"fmt"
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

// Client implements provider.Host for Bitbucket Cloud.
type Client struct {
	host domain.Host
	cred auth.Credentials
	http *http.Client
	api  string
}

// New constructs a Bitbucket Cloud adapter.
func New(host domain.Host, cred auth.Credentials, httpClient *http.Client) (*Client, error) {
	api := host.APIURL
	if api == "" {
		api = "https://api.bitbucket.org/2.0"
	}
	api = strings.TrimRight(api, "/")
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{host: host, cred: cred, http: httpClient, api: api}, nil
}

func (c *Client) Kind() domain.HostKind { return domain.HostBitbucketCloud }

func (c *Client) headers() map[string]string {
	h := map[string]string{"Accept": "application/json"}
	if c.cred.Cookie != "" {
		h["Cookie"] = c.cred.Cookie
		return h
	}
	if c.cred.Username != "" {
		// basic with app password
		token := c.cred.Token
		h["Authorization"] = "Basic " + basicAuth(c.cred.Username, token)
	} else if c.cred.Token != "" {
		h["Authorization"] = "Bearer " + c.cred.Token
	}
	return h
}

func basicAuth(user, pass string) string {
	return base64Encode(user + ":" + pass)
}

func base64Encode(s string) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	in := []byte(s)
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

func (c *Client) prURL(ref domain.PRRef, suffix string) string {
	base := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d", c.api, url.PathEscape(ref.Repo.Owner), url.PathEscape(ref.Repo.Name), ref.Number)
	if suffix != "" {
		return base + "/" + strings.TrimPrefix(suffix, "/")
	}
	return base
}

func (c *Client) ListPullRequests(ctx context.Context, ref domain.RepoRef, opts domain.ListOpts) ([]domain.PullRequest, error) {
	state := opts.State
	if state == "" {
		state = "OPEN"
	}
	state = strings.ToUpper(state)
	q := url.Values{}
	q.Set("state", state)
	pagelen := 50
	if opts.Limit > 0 && opts.Limit < 50 {
		pagelen = opts.Limit
	}
	q.Set("pagelen", strconv.Itoa(pagelen))
	u := fmt.Sprintf("%s/repositories/%s/%s/pullrequests?%s", c.api, url.PathEscape(ref.Owner), url.PathEscape(ref.Name), q.Encode())

	var page struct {
		Values []bbPR `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u, c.headers(), nil, &page); err != nil {
		return nil, err
	}
	out := make([]domain.PullRequest, 0, len(page.Values))
	for _, p := range page.Values {
		if opts.Author != "" && p.Author.DisplayName != opts.Author && p.Author.Nickname != opts.Author {
			continue
		}
		pr := p.toDomain(ref)
		pr.Reviews = reviewStatusFromBBParticipants(p.Participants, c.cred.Username)
		out = append(out, pr)
	}
	return out, nil
}

func (c *Client) GetPullRequest(ctx context.Context, ref domain.PRRef) (*domain.PullRequest, error) {
	var p bbPR
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, c.prURL(ref, ""), c.headers(), nil, &p); err != nil {
		return nil, err
	}
	pr := p.toDomain(ref.Repo)
	if st, err := c.GetReviewStatus(ctx, ref); err == nil {
		pr.Reviews = st
	}
	return &pr, nil
}

func (c *Client) ListFiles(ctx context.Context, ref domain.PRRef) ([]domain.FileChange, error) {
	u := c.prURL(ref, "diffstat")
	var page struct {
		Values []bbDiffStat `json:"values"`
	}
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, u+"?pagelen=100", c.headers(), nil, &page); err != nil {
		return nil, err
	}
	out := make([]domain.FileChange, 0, len(page.Values))
	for _, d := range page.Values {
		path := d.New.Path
		oldPath := d.Old.Path
		if path == "" {
			path = oldPath
		}
		st := domain.FileModified
		switch d.Status {
		case "added":
			st = domain.FileAdded
		case "removed":
			st = domain.FileRemoved
		case "renamed":
			st = domain.FileRenamed
		}
		out = append(out, domain.FileChange{Path: path, OldPath: oldPath, Status: st})
	}
	return out, nil
}

func (c *Client) GetFileDiff(ctx context.Context, ref domain.PRRef, path string) (*domain.FileDiff, error) {
	u := c.prURL(ref, "diff/"+url.PathEscape(path))
	hdr := c.headers()
	hdr["Accept"] = "text/plain"
	data, _, err := httputil.DoRaw(ctx, c.http, http.MethodGet, u, hdr, nil)
	if err != nil {
		// fallback: full PR diff and filter
		return c.getFromFullDiff(ctx, ref, path)
	}
	return diff.ParseUnified(path, string(data))
}

func (c *Client) getFromFullDiff(ctx context.Context, ref domain.PRRef, path string) (*domain.FileDiff, error) {
	u := c.prURL(ref, "diff")
	hdr := c.headers()
	hdr["Accept"] = "text/plain"
	data, _, err := httputil.DoRaw(ctx, c.http, http.MethodGet, u, hdr, nil)
	if err != nil {
		return nil, err
	}
	parts := splitUnifiedByFile(string(data))
	if patch, ok := parts[path]; ok {
		return diff.ParseUnified(path, patch)
	}
	for p, patch := range parts {
		if strings.HasSuffix(p, path) || strings.HasSuffix(path, p) {
			return diff.ParseUnified(path, patch)
		}
	}
	return nil, fmt.Errorf("file %q not found in diff", path)
}

func (c *Client) ListComments(ctx context.Context, ref domain.PRRef) ([]domain.Comment, error) {
	u := c.prURL(ref, "comments?pagelen=100")
	var page struct {
		Values []bbComment `json:"values"`
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
		body := map[string]any{
			"content": map[string]string{"raw": d.Body},
		}
		if d.ParentID != "" {
			id, err := strconv.Atoi(d.ParentID)
			if err != nil {
				return fmt.Errorf("parent id: %w", err)
			}
			body["parent"] = map[string]any{"id": id}
		} else if d.Anchor != nil {
			inline := map[string]any{"path": d.Anchor.Path}
			if d.Anchor.Side == domain.SideLeft {
				inline["from"] = d.Anchor.Line
				if d.Anchor.EndLine > d.Anchor.Line {
					inline["start_from"] = d.Anchor.Line
					inline["from"] = d.Anchor.EndLine
				}
			} else {
				inline["to"] = d.Anchor.Line
				if d.Anchor.EndLine > d.Anchor.Line {
					inline["start_to"] = d.Anchor.Line
					inline["to"] = d.Anchor.EndLine
				}
			}
			body["inline"] = inline
		}
		if _, err := httputil.DoJSON(ctx, c.http, http.MethodPost, c.prURL(ref, "comments"), c.headers(), body, nil); err != nil {
			return fmt.Errorf("post comment: %w", err)
		}
	}
	if draft.Summary != "" {
		body := map[string]any{"content": map[string]string{"raw": draft.Summary}}
		if _, err := httputil.DoJSON(ctx, c.http, http.MethodPost, c.prURL(ref, "comments"), c.headers(), body, nil); err != nil {
			return fmt.Errorf("post summary: %w", err)
		}
	}
	switch draft.Action {
	case domain.ActionApprove:
		return c.Approve(ctx, ref)
	case domain.ActionRequestChanges:
		return c.RequestChanges(ctx, ref)
	}
	return nil
}

func (c *Client) Approve(ctx context.Context, ref domain.PRRef) error {
	_, err := httputil.DoJSON(ctx, c.http, http.MethodPost, c.prURL(ref, "approve"), c.headers(), nil, nil)
	return err
}

func (c *Client) Unapprove(ctx context.Context, ref domain.PRRef) error {
	_, err := httputil.DoJSON(ctx, c.http, http.MethodDelete, c.prURL(ref, "approve"), c.headers(), nil, nil)
	return err
}

func (c *Client) RequestChanges(ctx context.Context, ref domain.PRRef) error {
	_, err := httputil.DoJSON(ctx, c.http, http.MethodPost, c.prURL(ref, "request-changes"), c.headers(), nil, nil)
	return err
}

func (c *Client) GetReviewStatus(ctx context.Context, ref domain.PRRef) (domain.ReviewStatus, error) {
	var p bbPR
	if _, err := httputil.DoJSON(ctx, c.http, http.MethodGet, c.prURL(ref, ""), c.headers(), nil, &p); err != nil {
		return domain.ReviewStatus{}, err
	}
	return reviewStatusFromBBParticipants(p.Participants, c.cred.Username), nil
}

func reviewStatusFromBBParticipants(parts []bbParticipant, viewer string) domain.ReviewStatus {
	st := domain.ReviewStatus{ViewerLogin: viewer}
	for _, part := range parts {
		login := part.User.Nickname
		if login == "" {
			login = part.User.UUID
		}
		dec := domain.DecisionNone
		state := strings.ToLower(part.State)
		switch {
		case state == "approved" || part.Approved:
			dec = domain.DecisionApproved
		case state == "changes_requested":
			dec = domain.DecisionChangesRequested
		default:
			continue
		}
		st.Reviewers = append(st.Reviewers, domain.Reviewer{
			Login:    login,
			Name:     part.User.DisplayName,
			Decision: dec,
		})
	}
	st.Normalize()
	return st
}

func (c *Client) ListTasks(ctx context.Context, ref domain.PRRef) ([]domain.Task, error) {
	u := c.prURL(ref, "tasks?pagelen=100")
	var page struct {
		Values []bbTask `json:"values"`
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
	state := "UNRESOLVED"
	if done {
		state = "RESOLVED"
	}
	body := map[string]any{"state": state}
	_, err := httputil.DoJSON(ctx, c.http, http.MethodPut, c.prURL(ref, "tasks/"+url.PathEscape(taskID)), c.headers(), body, nil)
	return err
}

type bbTask struct {
	ID      int    `json:"id"`
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	State  string `json:"state"`
	Author struct {
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
	} `json:"creator"`
}

func (t bbTask) toDomain() domain.Task {
	done := strings.EqualFold(t.State, "RESOLVED")
	return domain.Task{
		ID:       strconv.Itoa(t.ID),
		Body:     t.Content.Raw,
		Done:     done,
		Author:   domain.FormatAuthor(t.Author.Nickname, t.Author.DisplayName),
		Required: true,
	}
}

type bbPR struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	Draft       bool   `json:"draft"`
	Author      struct {
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
	} `json:"author"`
	Source struct {
		Commit struct {
			Hash string `json:"hash"`
		} `json:"commit"`
	} `json:"source"`
	Destination struct {
		Commit struct {
			Hash string `json:"hash"`
		} `json:"commit"`
	} `json:"destination"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
	CreatedOn    time.Time `json:"created_on"`
	UpdatedOn    time.Time `json:"updated_on"`
	Participants []bbParticipant `json:"participants"`
}

type bbParticipant struct {
	User struct {
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
		UUID        string `json:"uuid"`
	} `json:"user"`
	Role     string `json:"role"`
	Approved bool   `json:"approved"`
	State    string `json:"state"`
}

func (p bbPR) toDomain(repo domain.RepoRef) domain.PullRequest {
	return domain.PullRequest{
		Ref:       domain.PRRef{Repo: repo, Number: p.ID},
		Title:     p.Title,
		Body:      p.Description,
		Author:    domain.FormatAuthor(p.Author.Nickname, p.Author.DisplayName),
		State:     strings.ToLower(p.State),
		Draft:     p.Draft,
		URL:       p.Links.HTML.Href,
		HeadSHA:   p.Source.Commit.Hash,
		BaseSHA:   p.Destination.Commit.Hash,
		CreatedAt: p.CreatedOn,
		UpdatedAt: p.UpdatedOn,
	}
}

type bbDiffStat struct {
	Status string `json:"status"`
	Old    struct {
		Path string `json:"path"`
	} `json:"old"`
	New struct {
		Path string `json:"path"`
	} `json:"new"`
}

type bbComment struct {
	ID      int `json:"id"`
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	User struct {
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
	} `json:"user"`
	Parent *struct {
		ID int `json:"id"`
	} `json:"parent"`
	Inline *struct {
		Path string `json:"path"`
		To   *int   `json:"to"`
		From *int   `json:"from"`
	} `json:"inline"`
	CreatedOn time.Time `json:"created_on"`
	Links     struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

func (cm bbComment) toDomain() domain.Comment {
	c := domain.Comment{
		ID:      strconv.Itoa(cm.ID),
		Body:    cm.Content.Raw,
		Author:  domain.FormatAuthor(cm.User.Nickname, cm.User.DisplayName),
		URL:     cm.Links.HTML.Href,
		Created: cm.CreatedOn,
	}
	if cm.Parent != nil && cm.Parent.ID != 0 {
		c.ParentID = strconv.Itoa(cm.Parent.ID)
	}
	if cm.Inline != nil {
		c.Path = cm.Inline.Path
		a := &domain.Anchor{Path: cm.Inline.Path, Side: domain.SideRight}
		if cm.Inline.To != nil {
			a.Line = *cm.Inline.To
			a.Side = domain.SideRight
			a.LineType = domain.LineAdded
		} else if cm.Inline.From != nil {
			a.Line = *cm.Inline.From
			a.Side = domain.SideLeft
			a.LineType = domain.LineRemoved
		}
		c.Anchor = a
	}
	return c
}

func splitUnifiedByFile(patch string) map[string]string {
	out := map[string]string{}
	parts := strings.Split(patch, "diff --git ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		full := "diff --git " + part
		lines := strings.SplitN(part, "\n", 2)
		fields := strings.Fields(lines[0])
		path := ""
		if len(fields) >= 2 {
			path = strings.TrimPrefix(fields[len(fields)-1], "b/")
		}
		if path != "" {
			out[path] = full
		}
	}
	return out
}
