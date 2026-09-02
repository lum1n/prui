package domain

import (
	"strings"
	"time"
)

// HostKind identifies a forge API family.
type HostKind string

const (
	HostGitHub         HostKind = "github"
	HostBitbucketCloud HostKind = "bitbucket_cloud"
	HostBitbucketDC    HostKind = "bitbucket_dc"
)

// Side is the left (old) or right (new) side of a diff.
type Side string

const (
	SideLeft  Side = "LEFT"
	SideRight Side = "RIGHT"
)

// ReviewAction is the outcome when submitting a review.
type ReviewAction string

const (
	ActionComment        ReviewAction = "COMMENT"
	ActionApprove        ReviewAction = "APPROVE"
	ActionRequestChanges ReviewAction = "REQUEST_CHANGES"
)

// FileStatus describes how a path changed in a PR.
type FileStatus string

const (
	FileAdded    FileStatus = "added"
	FileModified FileStatus = "modified"
	FileRemoved  FileStatus = "removed"
	FileRenamed  FileStatus = "renamed"
)

// LineType classifies a diff line for Bitbucket DC anchors.
type LineType string

const (
	LineAdded   LineType = "ADDED"
	LineRemoved LineType = "REMOVED"
	LineContext LineType = "CONTEXT"
)

// Host describes a configured forge endpoint.
type Host struct {
	Name       string
	Kind       HostKind
	BaseURL    string
	APIURL     string
	TokenEnv   string
	CookieEnv  string   // env var holding a raw Cookie header value (session auth)
	MatchHosts []string // extra hostnames (e.g. git SSH) that map to this host
	Username   string
	CACert     string
}

// RepoRef identifies a repository on a host.
type RepoRef struct {
	Owner string // workspace / org / project key
	Name  string // repo slug
}

// PRRef identifies a pull request.
type PRRef struct {
	Repo   RepoRef
	Number int
	ID     string // some hosts use string IDs; Number is preferred when set
}

// PullRequest is a normalized PR/MR.
type PullRequest struct {
	Ref       PRRef
	Title     string
	Body      string
	Author    string
	State     string
	Draft     bool
	Blocked   bool // open required tasks / merge blockers
	URL       string
	HeadSHA   string
	BaseSHA   string
	CreatedAt time.Time
	UpdatedAt time.Time
	Reviews   ReviewStatus // approvals / change requests (may be empty until loaded)
}

// ReviewDecision is a reviewer's latest verdict on a PR.
type ReviewDecision string

const (
	DecisionNone             ReviewDecision = ""
	DecisionApproved         ReviewDecision = "approved"
	DecisionChangesRequested ReviewDecision = "changes_requested"
)

// Reviewer is one person's latest review decision.
type Reviewer struct {
	Login    string
	Name     string
	Decision ReviewDecision
}

// ReviewStatus summarizes approvals and change requests.
type ReviewStatus struct {
	Reviewers        []Reviewer
	ViewerLogin      string   // primary login/slug for the signed-in user
	ViewerAliases    []string // extra identities (display name, config username, …)
	ViewerDecision   ReviewDecision
	Approvers        []string // display labels (derived)
	ChangeRequesters []string // display labels (derived)
}

// HasReviews reports whether any approval or change-request is known.
func (s ReviewStatus) HasReviews() bool {
	return len(s.Approvers) > 0 || len(s.ChangeRequesters) > 0 || s.ViewerDecision != DecisionNone
}

// Normalize fills Approvers / ChangeRequesters from Reviewers and sets ViewerDecision.
func (s *ReviewStatus) Normalize() {
	s.Approvers = nil
	s.ChangeRequesters = nil
	s.ViewerDecision = DecisionNone
	for _, r := range s.Reviewers {
		label := FormatAuthor(r.Login, r.Name)
		switch r.Decision {
		case DecisionApproved:
			s.Approvers = append(s.Approvers, label)
		case DecisionChangesRequested:
			s.ChangeRequesters = append(s.ChangeRequesters, label)
		}
		if s.isViewer(r) {
			s.ViewerDecision = r.Decision
		}
	}
}

func (s ReviewStatus) isViewer(r Reviewer) bool {
	ids := make([]string, 0, 2+len(s.ViewerAliases))
	if s.ViewerLogin != "" {
		ids = append(ids, s.ViewerLogin)
	}
	ids = append(ids, s.ViewerAliases...)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if strings.EqualFold(r.Login, id) || strings.EqualFold(r.Name, id) {
			return true
		}
		if strings.EqualFold(AuthorLogin(FormatAuthor(r.Login, r.Name)), id) {
			return true
		}
	}
	return false
}

// Task is a required or optional PR checklist/blocker item.
type Task struct {
	ID       string
	Body     string
	Done     bool
	Author   string
	Required bool
	Version  int // Bitbucket DC optimistic lock; 0 when unused
}

// FileChange is one path in a PR diff.
type FileChange struct {
	Path    string
	OldPath string
	Status  FileStatus
	Patch   string // unified patch when available
}

// Anchor locates a comment on a diff line across forges.
type Anchor struct {
	Path     string
	Side     Side
	Line     int // primary line number on Side
	EndLine  int // inclusive end for multi-line; 0 = single line
	LineType LineType
	// DiffPosition is the GitHub legacy position (optional).
	DiffPosition int
}

// DiffLine is one rendered line of a parsed unified diff.
type DiffLine struct {
	Kind      LineType
	OldNumber int // 0 if none
	NewNumber int // 0 if none
	Text      string
	HunkIndex int
	Anchor    Anchor
}

// FileDiff is a fully parsed file-level diff.
type FileDiff struct {
	Path    string
	OldPath string
	Status  FileStatus
	Lines   []DiffLine
	Raw     string
}

// Comment is an existing PR comment (general or inline).
type Comment struct {
	ID       string
	Body     string
	Author   string
	Path     string
	Anchor   *Anchor
	ParentID string
	Resolved bool
	URL      string
	Created  time.Time
}

// DraftComment is a locally staged inline or general comment.
type DraftComment struct {
	ID       string
	Body     string
	Path     string
	Anchor   *Anchor // nil = general / review-level
	ParentID string  // non-empty = reply in an existing thread
}

// DraftReview is the in-progress review batch.
type DraftReview struct {
	Summary  string
	Action   ReviewAction
	Comments []DraftComment
	// RemoteID is a pending GitHub review ID when applicable.
	RemoteID int64
}

// ListOpts filters PR listings.
type ListOpts struct {
	State    string // open (ready, non-draft), draft, merged, closed, all
	Author   string
	Reviewer string // "me" means authenticated user when supported
	Limit    int
}

// NormalizeListState maps UI/API aliases to open|draft|merged|closed|all.
func NormalizeListState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "open", "ready":
		return "open"
	case "draft", "drafts":
		return "draft"
	case "merged":
		return "merged"
	case "closed", "declined", "superseded":
		return "closed"
	case "all":
		return "all"
	default:
		return strings.ToLower(strings.TrimSpace(state))
	}
}

// ViewOnly reports whether the PR should be browsed without review mutations.
func (p PullRequest) ViewOnly() bool {
	switch strings.ToLower(p.State) {
	case "merged", "closed", "declined", "superseded":
		return true
	default:
		return false
	}
}
