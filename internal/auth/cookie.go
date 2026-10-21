package auth

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// -H 'Cookie: …' / --header "cookie: …" from "Copy as cURL".
	curlHeaderRe = regexp.MustCompile(`(?is)(?:-H|--header)\s+(?:\$?'cookie:\s*((?:[^'\\]|\\.)*)'|"cookie:\s*((?:[^"\\]|\\.)*)")`)
	// -b '…' / --cookie "…" from "Copy as cURL" (newer Chrome).
	curlCookieRe = regexp.MustCompile(`(?is)(?:-b|--cookie)\s+(?:\$?'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)")`)
	// "cookie": "…" from "Copy as fetch" / HAR-like JSON.
	jsonCookieRe = regexp.MustCompile(`(?i)"cookie"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	// Cookie: … line in a pasted raw request header block.
	headerLineRe = regexp.MustCompile(`(?im)^\s*cookie:\s*(.+)$`)
)

// ParseCookieInput extracts a Cookie header value from pasted text. It accepts a raw
// value ("a=b; c=d"), a "Cookie: …" line, a raw request header block, or a browser
// DevTools "Copy as cURL" / "Copy as fetch" snippet.
func ParseCookieInput(in string) (string, error) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "", fmt.Errorf("empty input")
	}
	var v string
	for _, re := range []*regexp.Regexp{curlHeaderRe, curlCookieRe, jsonCookieRe} {
		if m := re.FindStringSubmatch(in); m != nil {
			v = firstNonEmptyStr(m[1:]...)
			if v != "" {
				break
			}
		}
	}
	if v == "" {
		if m := headerLineRe.FindStringSubmatch(in); m != nil {
			v = m[1]
		}
	}
	if v == "" && !strings.ContainsAny(in, "\n") && !strings.HasPrefix(strings.ToLower(in), "curl ") {
		v = in
	}
	v = strings.Join(strings.Fields(normalizeCookie(v)), " ")
	if v == "" || !strings.Contains(v, "=") {
		return "", fmt.Errorf("no cookie found; paste the Cookie header or a \"Copy as cURL\" command")
	}
	return v, nil
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
