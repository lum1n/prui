package provider

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/vegard/prui/internal/auth"
	"github.com/vegard/prui/internal/domain"
	"github.com/vegard/prui/internal/provider/bitbucketcloud"
	"github.com/vegard/prui/internal/provider/bitbucketdc"
	ghprov "github.com/vegard/prui/internal/provider/github"
)

// New creates a Host adapter for the given domain host config.
func New(host domain.Host) (Host, error) {
	cred, err := auth.Resolve(host)
	if err != nil {
		return nil, err
	}
	client, err := HTTPClient(host)
	if err != nil {
		return nil, err
	}

	switch host.Kind {
	case domain.HostGitHub:
		return ghprov.New(host, cred, client)
	case domain.HostBitbucketCloud:
		return bitbucketcloud.New(host, cred, client)
	case domain.HostBitbucketDC:
		return bitbucketdc.New(host, cred, client)
	default:
		return nil, fmt.Errorf("unsupported host kind %q", host.Kind)
	}
}

// HTTPClient builds an HTTP client with optional custom CA.
func HTTPClient(host domain.Host) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if host.CACert != "" {
		pem, err := os.ReadFile(host.CACert)
		if err != nil {
			return nil, fmt.Errorf("read ca_cert: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("failed to parse ca_cert %s", host.CACert)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &http.Client{Timeout: 60 * time.Second, Transport: transport}, nil
}
