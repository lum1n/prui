package provider

import (
	"context"

	"github.com/vegard/prui/internal/domain"
)

// Host is the forge adapter contract.
type Host interface {
	Kind() domain.HostKind
	ListPullRequests(ctx context.Context, ref domain.RepoRef, opts domain.ListOpts) ([]domain.PullRequest, error)
	GetPullRequest(ctx context.Context, ref domain.PRRef) (*domain.PullRequest, error)
	ListFiles(ctx context.Context, ref domain.PRRef) ([]domain.FileChange, error)
	GetFileDiff(ctx context.Context, ref domain.PRRef, path string) (*domain.FileDiff, error)
	ListComments(ctx context.Context, ref domain.PRRef) ([]domain.Comment, error)
	StartReview(ctx context.Context, ref domain.PRRef) (*domain.DraftReview, error)
	SubmitReview(ctx context.Context, ref domain.PRRef, draft domain.DraftReview) error
	Approve(ctx context.Context, ref domain.PRRef) error
	Unapprove(ctx context.Context, ref domain.PRRef) error
}

// SupportsRequestChanges reports whether the host has a first-class request-changes review event.
func SupportsRequestChanges(kind domain.HostKind) bool {
	return kind == domain.HostGitHub
}
