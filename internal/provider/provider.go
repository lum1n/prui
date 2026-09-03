package provider

import (
	"context"

	"github.com/lum1n/prui/internal/domain"
)

// Host is the forge adapter contract.
type Host interface {
	Kind() domain.HostKind
	ListPullRequests(ctx context.Context, ref domain.RepoRef, opts domain.ListOpts) ([]domain.PullRequest, error)
	GetPullRequest(ctx context.Context, ref domain.PRRef) (*domain.PullRequest, error)
	ListFiles(ctx context.Context, ref domain.PRRef) ([]domain.FileChange, error)
	GetFileDiff(ctx context.Context, ref domain.PRRef, path string) (*domain.FileDiff, error)
	// GetFileContent returns the file text at sha (usually the PR head commit).
	GetFileContent(ctx context.Context, ref domain.PRRef, path, sha string) (string, error)
	ListComments(ctx context.Context, ref domain.PRRef) ([]domain.Comment, error)
	ListTasks(ctx context.Context, ref domain.PRRef) ([]domain.Task, error)
	SetTaskDone(ctx context.Context, ref domain.PRRef, taskID string, done bool) error
	StartReview(ctx context.Context, ref domain.PRRef) (*domain.DraftReview, error)
	SubmitReview(ctx context.Context, ref domain.PRRef, draft domain.DraftReview) error
	Approve(ctx context.Context, ref domain.PRRef) error
	Unapprove(ctx context.Context, ref domain.PRRef) error
	// GetReviewStatus returns approvals / change-requests and the viewer's decision.
	GetReviewStatus(ctx context.Context, ref domain.PRRef) (domain.ReviewStatus, error)
}

// SupportsRequestChanges reports whether the host has a first-class request-changes action.
func SupportsRequestChanges(kind domain.HostKind) bool {
	switch kind {
	case domain.HostGitHub, domain.HostBitbucketDC, domain.HostBitbucketCloud:
		return true
	default:
		return false
	}
}
