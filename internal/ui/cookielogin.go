package ui

import (
	"context"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lum1n/prui/internal/auth"
	"github.com/lum1n/prui/internal/provider"
)

// cookieLogin is the in-TUI session cookie login dialog (Bitbucket DC).
type cookieLogin struct {
	active      bool
	input       textinput.Model
	reason      string
	err         string
	validating  bool
	triedStored bool // already retried with the cookie saved on disk
}

type cookieValidatedMsg struct {
	cookie string
	user   string
	err    error
}

func newCookieInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "paste Cookie header or DevTools \"Copy as cURL\""
	ti.Prompt = "▎ "
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.CharLimit = 0
	ti.Cursor.SetMode(cursor.CursorStatic)
	return ti
}

func (m *Model) cookieSession() provider.CookieSession {
	if !auth.SupportsCookieLogin(m.opts.Host) {
		return nil
	}
	cs, _ := m.opts.Provider.(provider.CookieSession)
	return cs
}

// cookieAutoLogin reports whether auth failures should open the login dialog
// (cookie-based or unauthenticated sessions; never for token auth).
func (m *Model) cookieAutoLogin() bool {
	cs := m.cookieSession()
	return cs != nil && (cs.Cookie() != "" || m.opts.NeedLogin)
}

func (m *Model) openCookieLogin(reason string) {
	if m.login.input.Prompt == "" {
		m.login.input = newCookieInput()
	}
	m.login.active = true
	m.login.reason = reason
	m.login.err = ""
	m.login.validating = false
	m.login.input.SetValue("")
	m.login.input.Focus()
}

func (m *Model) closeCookieLogin() {
	m.login.active = false
	m.login.validating = false
	m.login.input.Blur()
	m.login.input.SetValue("")
}

func (m *Model) reloadCurrent() tea.Cmd {
	m.loading = true
	m.errMsg = ""
	if m.pr != nil {
		return m.loadReview(m.pr.Ref.Number)
	}
	if m.opts.PRNumber > 0 {
		return m.loadReview(m.opts.PRNumber)
	}
	return m.loadPRs()
}

// msgErr extracts the error carried by an async load/submit result.
func msgErr(msg tea.Msg) error {
	switch v := msg.(type) {
	case loadedPRsMsg:
		return v.err
	case loadedReviewMsg:
		return v.err
	case loadedDiffMsg:
		return v.err
	case loadedAllDiffsMsg:
		return v.err
	case submitDoneMsg:
		return v.err
	case loadedTasksMsg:
		return v.err
	case taskToggledMsg:
		return v.err
	case loadedReviewStatusMsg:
		return v.err
	case loadedGapContentMsg:
		return v.err
	}
	return nil
}

// onAuthExpired runs after a request failed with an auth error: first retry once
// with the cookie saved on disk (it may be newer than $BB_COOKIE), then prompt.
func (m Model) onAuthExpired(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	cs := m.cookieSession()
	if cs == nil {
		return m, cmd
	}
	if !m.login.triedStored {
		m.login.triedStored = true
		if stored, _ := auth.LoadCookie(auth.Hostname(m.opts.Host)); stored != "" && stored != cs.Cookie() {
			cs.SetCookie(stored)
			m.status = "Retrying with saved session…"
			return m, tea.Batch(cmd, m.reloadCurrent())
		}
	}
	m.loading = false
	if !m.login.active {
		m.openCookieLogin("Your " + m.opts.Host.Name + " session has expired.")
	}
	return m, cmd
}

func (m Model) submitCookie(raw string) (tea.Model, tea.Cmd) {
	cookie, err := auth.ParseCookieInput(raw)
	if err != nil {
		m.login.err = err.Error()
		return m, nil
	}
	cs := m.cookieSession()
	if cs == nil {
		m.login.err = "this host does not support cookie login"
		return m, nil
	}
	m.login.err = ""
	m.login.validating = true
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		user, err := cs.ValidateCookie(ctx, cookie)
		return cookieValidatedMsg{cookie: cookie, user: user, err: err}
	}
}

func (m Model) handleCookieValidated(msg cookieValidatedMsg) (tea.Model, tea.Cmd) {
	m.login.validating = false
	if msg.err != nil {
		m.login.err = msg.err.Error()
		return m, nil
	}
	cs := m.cookieSession()
	if cs == nil {
		return m, nil
	}
	cs.SetCookie(msg.cookie)
	m.closeCookieLogin()
	m.opts.NeedLogin = false
	m.login.triedStored = true
	who := ""
	if msg.user != "" {
		who = " as " + msg.user
	}
	if err := auth.SaveCookie(auth.Hostname(m.opts.Host), msg.cookie); err != nil {
		m.status = "Logged in" + who + " (not saved: " + err.Error() + ")"
	} else {
		m.status = "Logged in" + who + " · session saved"
	}
	return m, m.reloadCurrent()
}

func (m Model) updateCookieLogin(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.login.validating {
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.closeCookieLogin()
		if m.opts.NeedLogin {
			m.loading = false
			m.status = "Not logged in · press L to log in"
		}
		return m, nil
	case "ctrl+o":
		u := strings.TrimRight(m.opts.Host.BaseURL, "/")
		if err := openBrowser(u + "/dashboard"); err != nil {
			m.login.err = "open browser: " + err.Error()
		} else {
			m.login.err = ""
		}
		return m, nil
	case "ctrl+v":
		text, err := clipboard.ReadAll()
		if err != nil {
			m.login.err = "clipboard: " + err.Error()
			return m, nil
		}
		m.login.input.SetValue(strings.TrimSpace(text))
		return m.submitCookie(text)
	case "enter":
		return m.submitCookie(m.login.input.Value())
	}
	var cmd tea.Cmd
	m.login.input, cmd = m.login.input.Update(msg)
	return m, cmd
}

func (m Model) cookieLoginView(width, height int) string {
	w := width - 8
	if w > 76 {
		w = 76
	}
	if w < 30 {
		w = 30
	}
	m.login.input.Width = w - 6
	base := strings.TrimRight(m.opts.Host.BaseURL, "/")

	lines := []string{titleStyle.Render("Log in to " + m.opts.Host.Name)}
	if m.login.reason != "" {
		lines = append(lines, "", m.login.reason)
	}
	lines = append(lines,
		"",
		"1. ctrl+o  open "+base+" in your browser and sign in",
		"2. DevTools → Network → any request → copy the Cookie",
		"   header, or right-click → Copy → Copy as cURL",
		"3. ctrl+v  paste from clipboard (or paste below + enter)",
		"",
		m.login.input.View(),
		"",
	)
	switch {
	case m.login.validating:
		lines = append(lines, mutedStyle.Render("Checking session…"))
	case m.login.err != "":
		lines = append(lines, errorStyle.Render(m.login.err))
	default:
		lines = append(lines, mutedStyle.Render("Saved to ~/.config/prui/credentials.json (0600)"))
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 2).
		Width(w).
		Render(strings.Join(lines, "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func cookieLoginHelpLine() string {
	return "ctrl+o open browser · ctrl+v paste clipboard · enter submit · esc cancel"
}
