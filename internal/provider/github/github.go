package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/vegard/prui/internal/auth"
	"github.com/vegard/prui/internal/diff"
	"github.com/vegard/prui/internal/domain"
	"golang.org/x/oauth2"
)

// Client implements provider.Host for GitHub Cloud and Enterprise.
type Client struct {
	host domain.Host
	gh   *github.Client
	http *http.Client
}

// New constructs a GitHub adapter.
// Prefer Cookie session auth when Credentials.Cookie is set (common on GHE without PATs).
func New(host domain.Host, cred auth.Credentials, httpClient *http.Client) (*Client, error) {
	var tc *http.Client
	if cred.Cookie != "" {
		tc = auth.WrapClient(httpClient, cred.Cookie)
		if tc.Timeout == 0 {
			tc.Timeout = 60 * time.Second
		}
	} else {
		if cred.Token == "" {
			return nil, fmt.Errorf("github auth: need token or cookie")
		}
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cred.Token})
		tc = oauth2.NewClient(context.Background(), ts)
		if httpClient != nil && httpClient.Transport != nil {
			base := httpClient.Transport
			tc.Transport = &oauth2.Transport{Source: ts, Base: base}
			tc.Timeout = httpClient.Timeout
		}
	}

	gh := github.NewClient(tc)
	api := host.APIURL
	if api == "" {
		api = "https://api.github.com/"
	}
	if !strings.HasSuffix(api, "/") {
		api += "/"
	}
	isDotCom := strings.Contains(api, "api.github.com")
	if !isDotCom {
		base := host.BaseURL
		if base == "" {
			base = strings.TrimSuffix(api, "/api/v3/")
			base = strings.TrimSuffix(base, "/api/v3")
		}
		upload := strings.TrimRight(base, "/") + "/api/uploads/"
		var err error
		gh, err = gh.WithEnterpriseURLs(api, upload)
		if err != nil {
			return nil, fmt.Errorf("enterprise urls: %w", err)
		}
	}
	return &Client{host: host, gh: gh, http: tc}, nil
}

func (c *Client) Kind() domain.HostKind { return domain.HostGitHub }

func (c *Client) ListPullRequests(ctx context.Context, ref domain.RepoRef, opts domain.ListOpts) ([]domain.PullRequest, error) {
	state := opts.State
	if state == "" {
		state = "open"
	}
	listOpts := &github.PullRequestListOptions{
		State: state,
		ListOptions: github.ListOptions{
			PerPage: perPage(opts.Limit),
		},
	}
	prs, _, err := c.gh.PullRequests.List(ctx, ref.Owner, ref.Name, listOpts)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PullRequest, 0, len(prs))
	for _, p := range prs {
		if opts.Author != "" && p.GetUser().GetLogin() != opts.Author {
			continue
		}
		out = append(out, mapPR(ref, p))
	}
	return out, nil
}

func (c *Client) GetPullRequest(ctx context.Context, ref domain.PRRef) (*domain.PullRequest, error) {
	p, _, err := c.gh.PullRequests.Get(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return nil, err
	}
	pr := mapPR(ref.Repo, p)
	return &pr, nil
}

func (c *Client) ListFiles(ctx context.Context, ref domain.PRRef) ([]domain.FileChange, error) {
	var all []domain.FileChange
	opt := &github.ListOptions{PerPage: 100}
	for {
		files, resp, err := c.gh.PullRequests.ListFiles(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, opt)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			all = append(all, domain.FileChange{
				Path:    f.GetFilename(),
				OldPath: f.GetPreviousFilename(),
				Status:  mapFileStatus(f.GetStatus()),
				Patch:   f.GetPatch(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return all, nil
}

func (c *Client) GetFileDiff(ctx context.Context, ref domain.PRRef, path string) (*domain.FileDiff, error) {
	files, err := c.ListFiles(ctx, ref)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.Path == path || f.OldPath == path {
			if f.Patch == "" {
				return &domain.FileDiff{Path: f.Path, OldPath: f.OldPath, Status: f.Status}, nil
			}
			return diff.ParseUnified(f.Path, f.Patch)
		}
	}
	return nil, fmt.Errorf("file %q not in pull request", path)
}

func (c *Client) ListComments(ctx context.Context, ref domain.PRRef) ([]domain.Comment, error) {
	var out []domain.Comment

	opt := &github.PullRequestListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}
	for {
		comments, resp, err := c.gh.PullRequests.ListComments(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, opt)
		if err != nil {
			return nil, err
		}
		for _, cm := range comments {
			anchor := &domain.Anchor{
				Path: cm.GetPath(),
				Side: domain.Side(cm.GetSide()),
				Line: cm.GetLine(),
			}
			if cm.GetStartLine() > 0 {
				anchor.Line = cm.GetStartLine()
				anchor.EndLine = cm.GetLine()
			}
			if anchor.Side == "" {
				anchor.Side = domain.SideRight
			}
			parent := ""
			if cm.GetInReplyTo() != 0 {
				parent = fmt.Sprintf("%d", cm.GetInReplyTo())
			}
			out = append(out, domain.Comment{
				ID:       fmt.Sprintf("%d", cm.GetID()),
				Body:     cm.GetBody(),
				Author:   domain.FormatAuthor(cm.GetUser().GetLogin(), cm.GetUser().GetName()),
				Path:     cm.GetPath(),
				Anchor:   anchor,
				ParentID: parent,
				URL:      cm.GetHTMLURL(),
				Created:  cm.GetCreatedAt().Time,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	issueOpt := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}
	for {
		comments, resp, err := c.gh.Issues.ListComments(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, issueOpt)
		if err != nil {
			return nil, err
		}
		for _, cm := range comments {
			out = append(out, domain.Comment{
				ID:      fmt.Sprintf("issue-%d", cm.GetID()),
				Body:    cm.GetBody(),
				Author:  domain.FormatAuthor(cm.GetUser().GetLogin(), cm.GetUser().GetName()),
				URL:     cm.GetHTMLURL(),
				Created: cm.GetCreatedAt().Time,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		issueOpt.Page = resp.NextPage
	}
	return out, nil
}

func (c *Client) ListTasks(ctx context.Context, ref domain.PRRef) ([]domain.Task, error) {
	p, _, err := c.gh.PullRequests.Get(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return nil, err
	}
	return parseChecklistTasks(p.GetBody()), nil
}

func (c *Client) SetTaskDone(ctx context.Context, ref domain.PRRef, taskID string, done bool) error {
	p, _, err := c.gh.PullRequests.Get(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return err
	}
	next, err := setChecklistDone(p.GetBody(), taskID, done)
	if err != nil {
		return err
	}
	_, _, err = c.gh.PullRequests.Edit(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, &github.PullRequest{
		Body: github.Ptr(next),
	})
	return err
}

func (c *Client) StartReview(ctx context.Context, ref domain.PRRef) (*domain.DraftReview, error) {
	// Resume pending review if one exists.
	reviews, _, err := c.gh.PullRequests.ListReviews(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, &github.ListOptions{PerPage: 50})
	if err != nil {
		return nil, err
	}
	for _, r := range reviews {
		if r.GetState() == "PENDING" {
			return &domain.DraftReview{RemoteID: r.GetID()}, nil
		}
	}
	return &domain.DraftReview{}, nil
}

func (c *Client) SubmitReview(ctx context.Context, ref domain.PRRef, draft domain.DraftReview) error {
	event := string(draft.Action)
	if event == "" {
		event = string(domain.ActionComment)
	}

	pr, _, err := c.gh.PullRequests.Get(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return err
	}
	commitID := pr.GetHead().GetSHA()

	var comments []*github.DraftReviewComment
	var replies []domain.DraftComment
	for _, d := range draft.Comments {
		if d.ParentID != "" {
			replies = append(replies, d)
			continue
		}
		if d.Anchor == nil {
			continue
		}
		side := string(d.Anchor.Side)
		if side == "" {
			side = "RIGHT"
		}
		dc := &github.DraftReviewComment{
			Path: github.Ptr(d.Anchor.Path),
			Body: github.Ptr(d.Body),
			Side: github.Ptr(side),
			Line: github.Ptr(d.Anchor.Line),
		}
		if d.Anchor.EndLine > d.Anchor.Line {
			dc.StartLine = github.Ptr(d.Anchor.Line)
			dc.Line = github.Ptr(d.Anchor.EndLine)
			dc.StartSide = github.Ptr(side)
		}
		comments = append(comments, dc)
	}

	body := draft.Summary
	for _, d := range draft.Comments {
		if d.Anchor == nil && d.ParentID == "" && d.Body != "" {
			if body != "" {
				body += "\n\n"
			}
			body += d.Body
		}
	}

	req := &github.PullRequestReviewRequest{
		CommitID: github.Ptr(commitID),
		Body:     github.Ptr(body),
		Event:    github.Ptr(event),
		Comments: comments,
	}

	postReplies := func() error {
		for _, d := range replies {
			if strings.HasPrefix(d.ParentID, "issue-") {
				// Issue comments are not threaded via the review API.
				_, _, err := c.gh.Issues.CreateComment(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, &github.IssueComment{
					Body: github.Ptr(d.Body),
				})
				if err != nil {
					return err
				}
				continue
			}
			parent, err := strconv.ParseInt(d.ParentID, 10, 64)
			if err != nil {
				return fmt.Errorf("parent id: %w", err)
			}
			_, _, err = c.gh.PullRequests.CreateComment(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, &github.PullRequestComment{
				Body:      github.Ptr(d.Body),
				InReplyTo: github.Ptr(parent),
			})
			if err != nil {
				return err
			}
		}
		return nil
	}

	if draft.RemoteID != 0 {
		for _, dc := range comments {
			_, _, err := c.gh.PullRequests.CreateComment(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, &github.PullRequestComment{
				Path:     dc.Path,
				Body:     dc.Body,
				Side:     dc.Side,
				Line:     dc.Line,
				CommitID: github.Ptr(commitID),
			})
			if err != nil {
				return err
			}
		}
		if err := postReplies(); err != nil {
			return err
		}
		_, _, err := c.gh.PullRequests.SubmitReview(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, draft.RemoteID, &github.PullRequestReviewRequest{
			Body:  github.Ptr(body),
			Event: github.Ptr(event),
		})
		return err
	}

	_, _, err = c.gh.PullRequests.CreateReview(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, req)
	if err != nil {
		return err
	}
	return postReplies()
}

func (c *Client) Approve(ctx context.Context, ref domain.PRRef) error {
	return c.SubmitReview(ctx, ref, domain.DraftReview{Action: domain.ActionApprove})
}

func (c *Client) Unapprove(ctx context.Context, ref domain.PRRef) error {
	reviews, _, err := c.gh.PullRequests.ListReviews(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, &github.ListOptions{PerPage: 50})
	if err != nil {
		return err
	}
	me, _, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return err
	}
	for _, r := range reviews {
		if r.GetUser().GetLogin() == me.GetLogin() && r.GetState() == "APPROVED" {
			_, _, err := c.gh.PullRequests.DismissReview(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, r.GetID(), &github.PullRequestReviewDismissalRequest{
				Message: github.Ptr("Dismissed via prui"),
			})
			return err
		}
	}
	return fmt.Errorf("no approval from current user to dismiss")
}

func mapPR(repo domain.RepoRef, p *github.PullRequest) domain.PullRequest {
	tasks := parseChecklistTasks(p.GetBody())
	return domain.PullRequest{
		Ref:       domain.PRRef{Repo: repo, Number: p.GetNumber()},
		Title:     p.GetTitle(),
		Body:      p.GetBody(),
		Author:    domain.FormatAuthor(p.GetUser().GetLogin(), p.GetUser().GetName()),
		State:     p.GetState(),
		Draft:     p.GetDraft(),
		Blocked:   anyOpenRequired(tasks),
		URL:       p.GetHTMLURL(),
		HeadSHA:   p.GetHead().GetSHA(),
		BaseSHA:   p.GetBase().GetSHA(),
		CreatedAt: p.GetCreatedAt().Time,
		UpdatedAt: p.GetUpdatedAt().Time,
	}
}

func mapFileStatus(s string) domain.FileStatus {
	switch s {
	case "added":
		return domain.FileAdded
	case "removed":
		return domain.FileRemoved
	case "renamed":
		return domain.FileRenamed
	default:
		return domain.FileModified
	}
}

func perPage(limit int) int {
	if limit <= 0 || limit > 100 {
		return 50
	}
	return limit
}
