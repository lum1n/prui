package domain

import "time"

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
	Name      string
	Kind      HostKind
	BaseURL   string
	APIURL    string
	TokenEnv  string
	CookieEnv string // env var holding a raw Cookie header value (session auth)
	Username  string
	CACert    string
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
	URL       string
	HeadSHA   string
	BaseSHA   string
	CreatedAt time.Time
	UpdatedAt time.Time
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
	ID     string
	Body   string
	Path   string
	Anchor *Anchor // nil = general / review-level
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
