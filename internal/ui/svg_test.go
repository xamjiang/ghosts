package ui

import (
	"crypto/sha1"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// A tiny ANSI-to-SVG renderer for the README screenshots. It understands the
// SGR codes our views emit (bold, 24-bit colours, reset) and lays text out on
// a fixed cell grid. Each run of text is stretched to its exact width with
// textLength, so columns stay aligned in whatever monospace font the viewer
// has, and the images need no embedded font.

const (
	cellW    = 8.4
	cellH    = 19.0
	fontSize = 14.0
	padX     = 22.0
	padY     = 14.0
	barH     = 36.0 // window title bar
	margin   = 24.0 // room for the shadow
)

type cell struct {
	ch       string
	fg, bg   string
	bold     bool
	occupied bool
}

type style struct {
	fg, bg string
	bold   bool
}

// parseANSI turns rendered output into a grid of cells, one row per line.
func parseANSI(s string) [][]cell {
	var grid [][]cell
	for _, ln := range strings.Split(s, "\n") {
		var row []cell
		var st style
		for len(ln) > 0 {
			if strings.HasPrefix(ln, "\x1b[") {
				end := strings.IndexByte(ln, 'm')
				if end < 0 {
					break
				}
				st = applySGR(st, ln[2:end])
				ln = ln[end+1:]
				continue
			}
			g, w := ansi.FirstGraphemeCluster(ln, ansi.GraphemeWidth)
			ln = ln[len(g):]
			row = append(row, cell{ch: g, fg: st.fg, bg: st.bg, bold: st.bold, occupied: true})
			for range w - 1 {
				row = append(row, cell{fg: st.fg, bg: st.bg})
			}
		}
		grid = append(grid, row)
	}
	return grid
}

func applySGR(st style, params string) style {
	if params == "" {
		return style{}
	}
	ps := strings.Split(params, ";")
	rgb := func(i int) string {
		r, _ := strconv.Atoi(ps[i])
		g, _ := strconv.Atoi(ps[i+1])
		b, _ := strconv.Atoi(ps[i+2])
		return fmt.Sprintf("#%02X%02X%02X", r, g, b)
	}
	for i := 0; i < len(ps); i++ {
		switch ps[i] {
		case "0":
			st = style{}
		case "1":
			st.bold = true
		case "22":
			st.bold = false
		case "39":
			st.fg = ""
		case "49":
			st.bg = ""
		case "38", "48":
			if i+4 < len(ps) && ps[i+1] == "2" {
				if ps[i] == "38" {
					st.fg = rgb(i + 2)
				} else {
					st.bg = rgb(i + 2)
				}
				i += 4
			}
		}
	}
	return st
}

// svgLine renders columns [x0, x1) of one row, positioned at the origin.
func svgLine(row []cell, x0, x1 int, defFG string) string {
	var b strings.Builder
	at := func(c int) cell {
		if c < len(row) {
			return row[c]
		}
		return cell{ch: " ", occupied: true}
	}
	// Backgrounds first, merged into runs.
	for c := x0; c < x1; {
		bg := at(c).bg
		n := 1
		for c+n < x1 && at(c+n).bg == bg {
			n++
		}
		if bg != "" {
			fmt.Fprintf(&b, `<rect x="%.1f" width="%.1f" height="%.0f" fill="%s"/>`, float64(c-x0)*cellW, float64(n)*cellW, cellH, bg)
		}
		c += n
	}
	// Then text, one element per style run; blank runs are skipped.
	for c := x0; c < x1; {
		first := at(c)
		n, text := 0, ""
		for c+n < x1 {
			k := at(c + n)
			if k.fg != first.fg || k.bold != first.bold {
				break
			}
			if k.occupied {
				text += k.ch
			}
			n++
		}
		if t := strings.TrimRight(text, " "); strings.TrimSpace(t) != "" {
			lead := len(t) - len(strings.TrimLeft(t, " "))
			t = strings.TrimLeft(t, " ")
			w := ansi.StringWidth(t)
			fg := first.fg
			if fg == "" {
				fg = defFG
			}
			attr := ""
			if first.bold {
				attr = ` font-weight="600"`
			}
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s"%s textLength="%.1f" lengthAdjust="spacingAndGlyphs">%s</text>`,
				float64(c-x0+lead)*cellW, cellH*0.72, fg, attr, float64(w)*cellW, html.EscapeString(t))
		}
		c += n
	}
	return b.String()
}

type svgFrame struct {
	grid [][]cell
	hold time.Duration
}

// crop is a rectangle of cells: columns [X0, X1), rows [Y0, Y1).
type crop struct{ X0, Y0, X1, Y1 int }

// windowSVG draws one or more frames inside a macOS-style terminal window.
// With several frames it loops through them with CSS animations, holding
// each for its duration; identical lines are stored once.
func windowSVG(frames []svgFrame, r crop, title, bg, defFG string) string {
	cols, rows := r.X1-r.X0, r.Y1-r.Y0
	winW := padX*2 + float64(cols)*cellW
	winH := barH + padY*2 + float64(rows)*cellH
	W, H := winW+margin*2, winH+margin*2

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, W, H, W, H)
	b.WriteString(`<style>text{font-family:"SF Mono",SFMono-Regular,ui-monospace,Menlo,"JetBrains Mono","Cascadia Mono",Consolas,"Liberation Mono",monospace;font-size:14px;white-space:pre}`)

	var total time.Duration
	for _, f := range frames {
		total += f.hold
	}
	if len(frames) > 1 {
		fmt.Fprintf(&b, `.f{visibility:hidden;animation:%.2fs step-end infinite}`, total.Seconds())
		var t time.Duration
		for i, f := range frames {
			start := 100 * t.Seconds() / total.Seconds()
			end := 100 * (t + f.hold).Seconds() / total.Seconds()
			fmt.Fprintf(&b, `.f%d{animation-name:k%d}`, i, i)
			if i == 0 {
				fmt.Fprintf(&b, `@keyframes k0{0%%{visibility:visible}%.3f%%{visibility:hidden}}`, end)
			} else {
				fmt.Fprintf(&b, `@keyframes k%d{0%%{visibility:hidden}%.3f%%{visibility:visible}%.3f%%{visibility:hidden}}`, i, start, end)
			}
			t += f.hold
		}
		b.WriteString(`@media (prefers-reduced-motion:reduce){.f{animation:none}.f0{visibility:visible}}`)
	}
	b.WriteString(`</style>`)

	// Window, shadow and title bar.
	fmt.Fprintf(&b, `<defs><filter id="s" x="-10%%" y="-10%%" width="120%%" height="130%%"><feDropShadow dx="0" dy="8" stdDeviation="10" flood-color="#000" flood-opacity=".35"/></filter><clipPath id="w"><rect x="%.0f" y="%.0f" width="%.1f" height="%.1f" rx="12"/></clipPath>`, margin, margin, winW, winH)

	// Unique lines.
	ids := map[string]string{}
	var defs strings.Builder
	lineID := func(svg string) string {
		sum := sha1.Sum([]byte(svg))
		key := fmt.Sprintf("%x", sum[:6])
		if _, ok := ids[key]; !ok {
			ids[key] = "l" + strconv.Itoa(len(ids))
			fmt.Fprintf(&defs, `<g id="%s">%s</g>`, ids[key], svg)
		}
		return ids[key]
	}
	var bodies []string
	for _, f := range frames {
		var fb strings.Builder
		for y := r.Y0; y < r.Y1 && y < len(f.grid); y++ {
			svg := svgLine(f.grid[y], r.X0, r.X1, defFG)
			if svg == "" {
				continue
			}
			fmt.Fprintf(&fb, `<use href="#%s" y="%.0f"/>`, lineID(svg), float64(y-r.Y0)*cellH)
		}
		bodies = append(bodies, fb.String())
	}
	b.WriteString(defs.String())
	b.WriteString(`</defs>`)

	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.1f" height="%.1f" rx="12" fill="%s" filter="url(#s)"/>`, margin, margin, winW, winH, bg)
	fmt.Fprintf(&b, `<g clip-path="url(#w)"><rect x="%.0f" y="%.0f" width="%.1f" height="%.0f" fill="#2A2A2D"/>`, margin, margin, winW, barH)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.1f" height="1" fill="#000" opacity=".5"/></g>`, margin, margin+barH, winW)
	for i, c := range []string{"#FF5F57", "#FEBC2E", "#28C840"} {
		fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="6" fill="%s"/>`, margin+20+float64(i)*20, margin+barH/2, c)
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="#98989D" text-anchor="middle" style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:13px">%s</text>`, W/2, margin+barH/2+4.5, html.EscapeString(title))
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.1f" height="%.1f" rx="12" fill="none" stroke="#FFFFFF" stroke-opacity=".12"/>`, margin+.5, margin+.5, winW-1, winH-1)

	fmt.Fprintf(&b, `<g transform="translate(%.1f %.1f)">`, margin+padX, margin+barH+padY)
	for i, body := range bodies {
		if len(frames) > 1 {
			fmt.Fprintf(&b, `<g class="f f%d">%s</g>`, i, body)
		} else {
			b.WriteString(body)
		}
	}
	b.WriteString(`</g></svg>`)
	return b.String()
}
