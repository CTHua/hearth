package tui

import (
	"image/color"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/CTHua/hearth/internal/checkout"
	"github.com/CTHua/hearth/internal/core"
	"github.com/CTHua/hearth/internal/skill"
	"github.com/CTHua/hearth/internal/subagent"
)

// Color tokens. lipgloss downgrades them to what the terminal supports and
// drops them under NO_COLOR, so every state also carries a symbol and a word.
// colorSelected is a neutral gray on purpose: a blue-gray such as #2B3140
// downgrades to teal at 256 colors and cyan at 16, which tmux shows by
// default. colorHover, the row under the mouse, is fainter than the cursor
// row and stays gray (235) at 256 colors for the same reason.
var (
	colorAccent   = lipgloss.Color("#E8A33D")
	colorOK       = lipgloss.Color("#8BC47E")
	colorWarn     = lipgloss.Color("#E5C07B")
	colorErr      = lipgloss.Color("#E06C75")
	colorMuted    = lipgloss.Color("#868B96")
	colorDim      = lipgloss.Color("#4B505C")
	colorSelected = lipgloss.Color("#303030")
	colorHover    = lipgloss.Color("#262626")
)

// Status symbols: ok, missing or off, conflict or error, other warnings.
const (
	symbolOK   = "✓"
	symbolOff  = "○"
	symbolErr  = "✗"
	symbolWarn = "!"
)

// States the MCP rows of Mine show; skills and repos bring their own.
const (
	stateOK             = "ok"
	stateOff            = "off"
	stateStale          = "stale"
	stateEnvMissing     = "env-missing"
	stateCommandMissing = "command-missing"
)

var (
	accent      = lipgloss.NewStyle().Foreground(colorAccent)
	accentBold  = accent.Bold(true)
	muted       = lipgloss.NewStyle().Foreground(colorMuted)
	dim         = lipgloss.NewStyle().Foreground(colorDim)
	bold        = lipgloss.NewStyle().Bold(true)
	errText     = lipgloss.NewStyle().Foreground(colorErr)
	warnText    = lipgloss.NewStyle().Foreground(colorWarn)
	mineBadge   = lipgloss.NewStyle().Foreground(colorAccent).Reverse(true).Bold(true)
	teamBadge   = lipgloss.NewStyle().Foreground(colorWarn).Reverse(true).Bold(true)
	sgrSequence = regexp.MustCompile("\x1b\\[[0-9;:]*m")
)

// cell is one table cell; a nil color is the terminal's own.
type cell struct {
	text  string
	color color.Color
}

// statusCell is a state as symbol, word and color, never color alone.
func statusCell(state string) cell {
	switch state {
	case stateOK:
		return cell{symbolOK + " " + state, colorOK}
	case string(skill.Disabled), string(skill.NotInstalled), string(skill.LinkMissing), string(checkout.Missing), core.NotSelectedLabel, stateOff, string(subagent.NotSynced):
		return cell{symbolOff + " " + state, colorMuted}
	case string(skill.PathConflict), string(skill.LinkConflict), string(skill.SkillInvalid), string(subagent.Invalid):
		return cell{symbolErr + " " + state, colorErr}
	}
	return cell{symbolWarn + " " + state, colorWarn}
}

// plain drops the colors of rendered text, for the screen under a form.
func plain(text string) string {
	return sgrSequence.ReplaceAllString(text, "")
}

// messageStyle colors a result line by what it reports.
func messageStyle(line string) lipgloss.Style {
	switch {
	case strings.HasPrefix(line, problemError):
		return errText
	case strings.HasPrefix(line, problemWarn):
		return warnText
	}
	return lipgloss.NewStyle()
}
