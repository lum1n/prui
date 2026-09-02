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
