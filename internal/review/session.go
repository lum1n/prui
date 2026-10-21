package review

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lum1n/prui/internal/config"
	"github.com/lum1n/prui/internal/domain"
)

// Session is a locally persisted draft review for one PR.
type Session struct {
	HostName string             `json:"host_name"`
	Repo     domain.RepoRef     `json:"repo"`
	Number   int                `json:"number"`
	Draft    domain.DraftReview `json:"draft"`
	AIReview *domain.AIReview   `json:"ai_review,omitempty"`
	Updated  time.Time          `json:"updated"`
}

func key(hostName string, ref domain.PRRef) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s/%s/%s/%d", hostName, ref.Repo.Owner, ref.Repo.Name, ref.Number)))
	return hex.EncodeToString(h[:8])
}

func pathFor(hostName string, ref domain.PRRef) (string, error) {
	dir, err := config.DraftsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key(hostName, ref)+".json"), nil
}

// Load reads a draft session from disk, or returns an empty one.
func Load(hostName string, ref domain.PRRef) (*Session, error) {
	p, err := pathFor(hostName, ref)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Session{
				HostName: hostName,
				Repo:     ref.Repo,
				Number:   ref.Number,
				Draft:    domain.DraftReview{Action: domain.ActionComment},
			}, nil
		}
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save persists the session.
func Save(s *Session) error {
	p, err := pathFor(s.HostName, domain.PRRef{Repo: s.Repo, Number: s.Number})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	s.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// Clear deletes a persisted draft.
func Clear(hostName string, ref domain.PRRef) error {
	p, err := pathFor(hostName, ref)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// AddComment appends a draft comment and saves.
func (s *Session) AddComment(c domain.DraftComment) error {
	if c.ID == "" {
		c.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	s.Draft.Comments = append(s.Draft.Comments, c)
	return Save(s)
}

// UpdateComment replaces the body of a draft by id.
func (s *Session) UpdateComment(id, body string) error {
	for i := range s.Draft.Comments {
		if s.Draft.Comments[i].ID == id {
			s.Draft.Comments[i].Body = body
			return Save(s)
		}
	}
	return fmt.Errorf("draft comment %s not found", id)
}

// RemoveComment deletes a draft by id.
func (s *Session) RemoveComment(id string) error {
	out := s.Draft.Comments[:0]
	for _, c := range s.Draft.Comments {
		if c.ID != id {
			out = append(out, c)
		}
	}
	s.Draft.Comments = out
	// An accepted AI suggestion whose draft is gone goes back to pending.
	if s.AIReview != nil {
		for i := range s.AIReview.Suggestions {
			if s.AIReview.Suggestions[i].DraftID == id {
				s.AIReview.Suggestions[i].DraftID = ""
				s.AIReview.Suggestions[i].Status = domain.SuggestionPending
			}
		}
	}
	return Save(s)
}

// SetAIReview stores a new AI review. Accepted suggestions from an earlier
// run are kept; pending ones are replaced. The review summary is prefilled
// when the draft has none.
func (s *Session) SetAIReview(r domain.AIReview) error {
	var kept []domain.AISuggestion
	if s.AIReview != nil {
		for _, sg := range s.AIReview.Suggestions {
			if sg.Status == domain.SuggestionAccepted {
				kept = append(kept, sg)
			}
		}
	}
	for i := range r.Suggestions {
		if r.Suggestions[i].ID == "" {
			r.Suggestions[i].ID = fmt.Sprintf("ai-%d-%d", time.Now().UnixNano(), i)
		}
		if r.Suggestions[i].Status == "" {
			r.Suggestions[i].Status = domain.SuggestionPending
		}
	}
	r.Suggestions = append(kept, r.Suggestions...)
	s.AIReview = &r
	if s.Draft.Summary == "" {
		s.Draft.Summary = r.Summary
	}
	return Save(s)
}

// Suggestion returns the AI suggestion with id, or nil.
func (s *Session) Suggestion(id string) *domain.AISuggestion {
	if s.AIReview == nil {
		return nil
	}
	for i := range s.AIReview.Suggestions {
		if s.AIReview.Suggestions[i].ID == id {
			return &s.AIReview.Suggestions[i]
		}
	}
	return nil
}

// AcceptSuggestion turns a pending suggestion into a draft comment with body
// (the suggestion body when empty) and saves.
func (s *Session) AcceptSuggestion(id, body string) error {
	sg := s.Suggestion(id)
	if sg == nil {
		return fmt.Errorf("ai suggestion %s not found", id)
	}
	if sg.Status == domain.SuggestionAccepted {
		return nil
	}
	if body == "" {
		body = sg.Body
	}
	sg.Body = body
	dc := domain.DraftComment{
		ID:     fmt.Sprintf("%d", time.Now().UnixNano()),
		Body:   body,
		Path:   sg.Path,
		Anchor: sg.Anchor,
	}
	sg.Status = domain.SuggestionAccepted
	sg.DraftID = dc.ID
	return s.AddComment(dc)
}

// UpdateSuggestion edits a suggestion body; for accepted ones the linked
// draft comment is updated too.
func (s *Session) UpdateSuggestion(id, body string) error {
	sg := s.Suggestion(id)
	if sg == nil {
		return fmt.Errorf("ai suggestion %s not found", id)
	}
	sg.Body = body
	if sg.Status == domain.SuggestionAccepted && sg.DraftID != "" {
		for i := range s.Draft.Comments {
			if s.Draft.Comments[i].ID == sg.DraftID {
				s.Draft.Comments[i].Body = body
			}
		}
	}
	return Save(s)
}

// RemoveSuggestion drops a suggestion. A draft created from it is removed too.
func (s *Session) RemoveSuggestion(id string) error {
	sg := s.Suggestion(id)
	if sg == nil {
		return fmt.Errorf("ai suggestion %s not found", id)
	}
	draftID := sg.DraftID
	out := s.AIReview.Suggestions[:0]
	for _, x := range s.AIReview.Suggestions {
		if x.ID != id {
			out = append(out, x)
		}
	}
	s.AIReview.Suggestions = out
	if draftID != "" {
		return s.RemoveComment(draftID)
	}
	return Save(s)
}

// PendingSuggestions counts suggestions not yet accepted.
func (s *Session) PendingSuggestions() int {
	if s == nil || s.AIReview == nil {
		return 0
	}
	n := 0
	for _, sg := range s.AIReview.Suggestions {
		if sg.Status != domain.SuggestionAccepted {
			n++
		}
	}
	return n
}
