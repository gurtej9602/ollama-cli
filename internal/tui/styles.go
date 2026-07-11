package tui

import (
	"sort"

	"github.com/charmbracelet/lipgloss"
)

// ── Theme palette ─────────────────────────────────────────────────────────────

// ThemeColors defines all colors for a visual theme.
type ThemeColors struct {
	Primary    lipgloss.Color
	Accent     lipgloss.Color
	Success    lipgloss.Color
	Warning    lipgloss.Color
	Error      lipgloss.Color
	Muted      lipgloss.Color
	Text       lipgloss.Color
	Bg         lipgloss.Color
	Border     lipgloss.Color
	UserBubble lipgloss.Color
}

// Built-in themes
var themes = map[string]ThemeColors{
	"dark": {
		Primary:    "#7C3AED",
		Accent:     "#06B6D4",
		Success:    "#10B981",
		Warning:    "#F59E0B",
		Error:      "#EF4444",
		Muted:      "#6B7280",
		Text:       "#F9FAFB",
		Bg:         "#111827",
		Border:     "#374151",
		UserBubble: "#1E1B4B",
	},
	"dracula": {
		Primary:    "#BD93F9",
		Accent:     "#50FA7B",
		Success:    "#50FA7B",
		Warning:    "#FFB86C",
		Error:      "#FF5555",
		Muted:      "#6272A4",
		Text:       "#F8F8F2",
		Bg:         "#282A36",
		Border:     "#44475A",
		UserBubble: "#3D3F4F",
	},
	"nord": {
		Primary:    "#81A1C1",
		Accent:     "#88C0D0",
		Success:    "#A3BE8C",
		Warning:    "#EBCB8B",
		Error:      "#BF616A",
		Muted:      "#4C566A",
		Text:       "#ECEFF4",
		Bg:         "#2E3440",
		Border:     "#3B4252",
		UserBubble: "#3B4252",
	},
	"monokai": {
		Primary:    "#F92672",
		Accent:     "#A6E22E",
		Success:    "#A6E22E",
		Warning:    "#FD971F",
		Error:      "#F92672",
		Muted:      "#75715E",
		Text:       "#F8F8F2",
		Bg:         "#272822",
		Border:     "#3E3D32",
		UserBubble: "#3E3D32",
	},
	"ocean": {
		Primary:    "#0EA5E9",
		Accent:     "#22D3EE",
		Success:    "#34D399",
		Warning:    "#FBBF24",
		Error:      "#F87171",
		Muted:      "#64748B",
		Text:       "#E2E8F0",
		Bg:         "#0F172A",
		Border:     "#1E3A5F",
		UserBubble: "#1E3A5F",
	},
	"forest": {
		Primary:    "#22C55E",
		Accent:     "#86EFAC",
		Success:    "#4ADE80",
		Warning:    "#FCD34D",
		Error:      "#F87171",
		Muted:      "#6B7280",
		Text:       "#F0FDF4",
		Bg:         "#052E16",
		Border:     "#14532D",
		UserBubble: "#14532D",
	},
	"sunset": {
		Primary:    "#F97316",
		Accent:     "#FBBF24",
		Success:    "#34D399",
		Warning:    "#F59E0B",
		Error:      "#EF4444",
		Muted:      "#78716C",
		Text:       "#FEF3C7",
		Bg:         "#1C1917",
		Border:     "#44403C",
		UserBubble: "#292524",
	},
}

var currentTheme = themes["dark"]
var currentThemeName = "dark"

// ListThemes returns sorted theme names.
func ListThemes() []string {
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SetTheme switches the active palette and recomputes all styles.
// Returns false if the name is unknown.
func SetTheme(name string) bool {
	t, ok := themes[name]
	if !ok {
		return false
	}
	currentTheme = t
	currentThemeName = name
	recomputeStyles()
	return true
}

// ── Style variables (recomputed on theme change) ───────────────────────────────

var (
	colorPrimary    lipgloss.Color
	colorAccent     lipgloss.Color
	colorSuccess    lipgloss.Color
	colorWarning    lipgloss.Color
	colorError      lipgloss.Color
	colorMuted      lipgloss.Color
	colorText       lipgloss.Color
	colorBg         lipgloss.Color
	colorBorder     lipgloss.Color
	colorUserBubble lipgloss.Color

	baseStyle           lipgloss.Style
	headerStyle         lipgloss.Style
	statusBarStyle      lipgloss.Style
	modelBadgeStyle     lipgloss.Style
	userMsgStyle        lipgloss.Style
	userLabelStyle      lipgloss.Style
	assistantLabelStyle lipgloss.Style
	errorStyle          lipgloss.Style
	hintStyle           lipgloss.Style
	toolStyle           lipgloss.Style
	successStyle        lipgloss.Style
	inputStyle          lipgloss.Style
	inputFocusedStyle   lipgloss.Style
	timestampStyle      lipgloss.Style
	themeBadgeStyle     lipgloss.Style
	searchHlStyle       lipgloss.Style
)

func init() { recomputeStyles() }

func recomputeStyles() {
	colorPrimary    = currentTheme.Primary
	colorAccent     = currentTheme.Accent
	colorSuccess    = currentTheme.Success
	colorWarning    = currentTheme.Warning
	colorError      = currentTheme.Error
	colorMuted      = currentTheme.Muted
	colorText       = currentTheme.Text
	colorBg         = currentTheme.Bg
	colorBorder     = currentTheme.Border
	colorUserBubble = currentTheme.UserBubble

	baseStyle = lipgloss.NewStyle().Foreground(colorText)

	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorPrimary).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 2).
		Align(lipgloss.Center)

	statusBarStyle = lipgloss.NewStyle().
		Foreground(colorMuted).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1)

	modelBadgeStyle = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		PaddingLeft(1)

	userMsgStyle = lipgloss.NewStyle().
		Foreground(colorText).
		Background(colorUserBubble).
		Padding(0, 1).
		MarginTop(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary)

	userLabelStyle = lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true)

	assistantLabelStyle = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true)

	errorStyle = lipgloss.NewStyle().
		Foreground(colorError).
		Bold(true)

	hintStyle = lipgloss.NewStyle().
		Foreground(colorMuted).
		Italic(true)

	toolStyle = lipgloss.NewStyle().
		Foreground(colorWarning).
		Bold(true)

	successStyle = lipgloss.NewStyle().
		Foreground(colorSuccess).
		Bold(true)

	inputStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1)

	inputFocusedStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(0, 1)

	timestampStyle = lipgloss.NewStyle().
		Foreground(colorMuted).
		Faint(true)

	themeBadgeStyle = lipgloss.NewStyle().
		Foreground(colorWarning).
		Bold(true).
		PaddingLeft(1)

	searchHlStyle = lipgloss.NewStyle().
		Foreground(colorBg).
		Background(colorWarning).
		Bold(true)
}

// runPromptStyleCmd renders the "press Y to run" banner.
func runPromptStyleCmd(cmd, workDir string) string {
	display := cmd
	if len(display) > 90 {
		display = display[:87] + "..."
	}
	line1 := toolStyle.Render("▶ Run project?") +
		"  " + lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("$ "+display)
	line2 := hintStyle.Render("   in " + workDir)
	line3 := hintStyle.Render("   Press Y to run  •  N or Esc to skip")
	return line1 + "\n" + line2 + "\n" + line3 + "\n"
}
