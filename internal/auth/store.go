package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// fileStore persists tokens by forge hostname under the config dir.
type fileStore struct {
	path string
	mu   sync.Mutex
}

type storeFile struct {
	Tokens map[string]string `json:"tokens"` // hostname → token
}

func defaultStorePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func configDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "prui"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "prui"), nil
}

// LoadToken returns a stored token for hostname, if any.
func LoadToken(hostname string) (string, error) {
	hostname = normalizeHostname(hostname)
	if hostname == "" {
		return "", nil
	}
	path, err := defaultStorePath()
	if err != nil {
		return "", err
	}
	s := &fileStore{path: path}
	return s.get(hostname)
}

// SaveToken stores a token for hostname (0600 file).
func SaveToken(hostname, token string) error {
	hostname = normalizeHostname(hostname)
	token = strings.TrimSpace(token)
	if hostname == "" || token == "" {
		return fmt.Errorf("hostname and token are required")
	}
	path, err := defaultStorePath()
	if err != nil {
		return err
	}
	s := &fileStore{path: path}
	return s.set(hostname, token)
}

// DeleteToken removes a stored token for hostname.
func DeleteToken(hostname string) error {
	hostname = normalizeHostname(hostname)
	if hostname == "" {
		return nil
	}
	path, err := defaultStorePath()
	if err != nil {
		return err
	}
	s := &fileStore{path: path}
	return s.delete(hostname)
}

func (s *fileStore) get(hostname string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(f.Tokens[hostname]), nil
}

func (s *fileStore) set(hostname, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return err
	}
	if f.Tokens == nil {
		f.Tokens = map[string]string{}
	}
	f.Tokens[hostname] = token
	return s.write(f)
}

func (s *fileStore) delete(hostname string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return err
	}
	if f.Tokens == nil {
		return nil
	}
	delete(f.Tokens, hostname)
	return s.write(f)
}

func (s *fileStore) read() (storeFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return storeFile{Tokens: map[string]string{}}, nil
		}
		return storeFile{}, err
	}
	var f storeFile
	if err := json.Unmarshal(data, &f); err != nil {
		return storeFile{}, fmt.Errorf("read credentials: %w", err)
	}
	if f.Tokens == nil {
		f.Tokens = map[string]string{}
	}
	return f, nil
}

func (s *fileStore) write(f storeFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func normalizeHostname(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	if i := strings.IndexByte(h, ':'); i >= 0 {
		port := h[i+1:]
		if port != "" && isAllDigits(port) {
			h = h[:i]
		}
	}
	return h
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// HostnameOf extracts a hostname from a base or API URL.
func HostnameOf(raw string) string {
	return normalizeHostname(raw)
}
