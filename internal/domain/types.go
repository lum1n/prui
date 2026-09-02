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
	ViewerLogin      string
	ViewerDecision   ReviewDecision
	Approvers        []string // logins (derived)
	ChangeRequesters []string // logins (derived)
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
		switch r.Decision {
		case DecisionApproved:
			s.Approvers = append(s.Approvers, displayLogin(r))
		case DecisionChangesRequested:
			s.ChangeRequesters = append(s.ChangeRequesters, displayLogin(r))
		}
		if s.ViewerLogin != "" && strings.EqualFold(r.Login, s.ViewerLogin) {
			s.ViewerDecision = r.Decision
		}
	}
}

func displayLogin(r Reviewer) string {
	if r.Login != "" {
		return r.Login
	}
	return r.Name
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
	State    string // open, closed, merged, all
	Author   string
	Reviewer string // "me" means authenticated user when supported
	Limit    int
}
