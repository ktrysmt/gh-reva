package tui

import (
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// splashLayout selects which arrangement loadingView paints. Picked
// once at NewModel time (random, unless GH_REVA_SPLASH_LAYOUT pins a
// value) so the splash does not flicker through variants during the
// few seconds of PR load.
type splashLayout int

const (
	splashLayoutDome      splashLayout = 1 // dome only + "reva vX.Y.Z" + spinner
	splashLayoutAscii     splashLayout = 2 // ASCII REVA art + "vX.Y.Z" + spinner
	splashLayoutDomeAscii splashLayout = 3 // ASCII REVA art beside dome + "vX.Y.Z" + spinner
)

// revaArt holds the three ASCII REVA designs. Index is selected at
// startup (random, unless GH_REVA_SPLASH_ART pins a value).
//
// Source rows can be authored uneven (figlet output trails differ; some
// editors auto-trim trailing whitespace on save). init() right-pads
// every row to the variant's widest row so layout 3's horizontal join
// and layout 2's per-row centering both see equal-width rows.
var revaArt = []string{
	// 0: figlet "standard" — 5 rows, classic pipe-letter.
	` ____  _______     ___
|  _ \| ____\ \   / / \
| |_) |  _|  \ \ / / _ \
|  _ <| |___  \ V / ___ \
|_| \_\_____|  \_/_/   \_\`,
	// 1: ANSI Shadow — 6 rows, bold blocks with a 3D bevel via the
	// ╔╗╚╝═║ outline running below and right of each letter. The
	// largest variant; reads like a game-splash logo.
	`██████╗ ███████╗██╗   ██╗ █████╗
██╔══██╗██╔════╝██║   ██║██╔══██╗
██████╔╝█████╗  ██║   ██║███████║
██╔══██╗██╔══╝  ╚██╗ ██╔╝██╔══██║
██║  ██║███████╗ ╚████╔╝ ██║  ██║
╚═╝  ╚═╝╚══════╝  ╚═══╝  ╚═╝  ╚═╝`,
	// 2: half-block — 3 rows, condensed.
	`█▀█ █▀▀ █ █ ▄▀█
█▀▄ █▀  ▀▄▀ █▀█
▀ ▀ ▀▀▀  ▀  ▀ ▀`,
}

func init() {
	for i, art := range revaArt {
		rows := strings.Split(art, "\n")
		w := 0
		for _, r := range rows {
			if rw := lipgloss.Width(r); rw > w {
				w = rw
			}
		}
		for j, r := range rows {
			if pad := w - lipgloss.Width(r); pad > 0 {
				rows[j] = r + strings.Repeat(" ", pad)
			}
		}
		revaArt[i] = strings.Join(rows, "\n")
	}
}

// chooseSplashLayout returns the layout for this launch. Reads
// GH_REVA_SPLASH_LAYOUT (1/2/3) for deterministic e2e; falls back to
// uniform random over the three layouts.
func chooseSplashLayout() splashLayout {
	if v := os.Getenv("GH_REVA_SPLASH_LAYOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 3 {
			return splashLayout(n)
		}
	}
	return splashLayout(rand.Intn(3) + 1)
}

// chooseSplashArt returns the ascii-art index for this launch. Reads
// GH_REVA_SPLASH_ART (0/1/2) for deterministic e2e; falls back to
// uniform random.
func chooseSplashArt() int {
	if v := os.Getenv("GH_REVA_SPLASH_ART"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n < len(revaArt) {
			return n
		}
	}
	return rand.Intn(len(revaArt))
}

// renderRevaArt returns the selected REVA art with one shade applied
// uniformly. We keep it mono-colored so each variant reads as a single
// logo instead of competing with the dome's three-shade gradient.
func (m Model) renderRevaArt() string {
	art := revaArt[m.splashArtIdx]
	color := m.theme.LogoShade1
	rows := strings.Split(art, "\n")
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = fg(r, color)
	}
	return strings.Join(out, "\n")
}

// composeDomeAndAscii renders ASCII art on the left and the dome on
// the right, joined per-row with a 3-column gap. The shorter block is
// vertically centered against the taller one so the joined block has
// no awkward top/bottom hang.
//
// We use raw row widths (lipgloss.Width strips SGR) for the gap
// calculation, then concatenate the SGR-bearing rows so per-row colour
// stays intact through the join.
func (m Model) composeDomeAndAscii() []string {
	artRows := strings.Split(m.renderRevaArt(), "\n")
	domeRows := strings.Split(renderLogo(m.theme), "\n")
	h := len(domeRows)
	if len(artRows) > h {
		h = len(artRows)
	}
	// +1 top bias on the art shifts it one row below the dome's
	// vertical midline so the two blocks read as a paired logo
	// rather than the smaller block hovering above the larger one.
	artRows = padRowsVertically(artRows, h, 1)
	domeRows = padRowsVertically(domeRows, h, 0)
	artW := maxLineWidth(artRows)
	domeW := maxLineWidth(domeRows)
	const gap = "   " // 3 cols breathing room between the blocks
	out := make([]string, h)
	for i := 0; i < h; i++ {
		a := artRows[i]
		if pad := artW - lipgloss.Width(a); pad > 0 {
			a += strings.Repeat(" ", pad)
		}
		d := domeRows[i]
		if pad := domeW - lipgloss.Width(d); pad > 0 {
			d += strings.Repeat(" ", pad)
		}
		out[i] = a + gap + d
	}
	return out
}

// padRowsVertically centers `rows` inside an `h`-row block by adding
// blank rows above and below (split as evenly as possible; an extra
// blank goes to the bottom on odd remainder so the visible content
// hugs the top very slightly — matches how figlet output usually
// reads when paired with a tall logo).
//
// topBias adds extra blank rows above the content (and removes the
// same count from below), nudging the block downward without changing
// the total height. Used by composeDomeAndAscii to set the ASCII
// REVA art one row below the dome's vertical midline so it visually
// reads as nestled inside the dome's frame. Negative biases nudge up.
// Out-of-range biases are clamped so `top` stays in [0, diff].
func padRowsVertically(rows []string, h, topBias int) []string {
	if len(rows) >= h {
		return rows
	}
	diff := h - len(rows)
	top := diff/2 + topBias
	if top < 0 {
		top = 0
	}
	if top > diff {
		top = diff
	}
	bot := diff - top
	out := make([]string, 0, h)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	out = append(out, rows...)
	for i := 0; i < bot; i++ {
		out = append(out, "")
	}
	return out
}

func maxLineWidth(rows []string) int {
	w := 0
	for _, r := range rows {
		if rw := lipgloss.Width(r); rw > w {
			w = rw
		}
	}
	return w
}

// versionLineFor returns the version label as it appears between the
// splash and the spinner. Layout 1 prefixes "reva " because the dome
// alone is wordless; layouts 2 and 3 already render REVA as art so the
// label collapses to just the version string. Empty version → empty
// line (caller suppresses).
//
// goreleaser passes the bare semver via {{.Version}} (no leading `v`),
// so without the prepend below a 0.3.1 release would render as just
// `0.3.1` while every other surface (`git tag`, release page, install
// command) reads `v0.3.1`. Prepend the `v` only when the supplied
// version starts with a digit; already-prefixed values (test fixtures,
// future ldflags using {{.Tag}}) and non-semver strings like `dev`
// stay untouched.
func (m Model) versionLineFor(layout splashLayout) string {
	if m.version == "" {
		return ""
	}
	v := m.version
	if v[0] >= '0' && v[0] <= '9' {
		v = "v" + v
	}
	label := v
	if layout == splashLayoutDome {
		label = "reva " + v
	}
	return fg(label, m.theme.LoadingSpinner)
}
