package domain

import "strings"

// FormatAuthor builds a display label from login/slug and optional full name.
// Examples: "Ada Lovelace (alovelace)", "alovelace", "Ada Lovelace".
func FormatAuthor(login, displayName string) string {
	login = strings.TrimSpace(login)
	displayName = strings.TrimSpace(displayName)
	switch {
	case displayName != "" && login != "" && !strings.EqualFold(displayName, login):
		return displayName + " (" + login + ")"
	case displayName != "":
		return displayName
	default:
		return login
	}
}

// AuthorLogin extracts the login/slug from a FormatAuthor label when present.
func AuthorLogin(formatted string) string {
	formatted = strings.TrimSpace(formatted)
	if i := strings.LastIndex(formatted, "("); i >= 0 && strings.HasSuffix(formatted, ")") {
		inner := strings.TrimSpace(formatted[i+1 : len(formatted)-1])
		if inner != "" {
			return inner
		}
	}
	return formatted
}
