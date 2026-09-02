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
	return Save(s)
}
