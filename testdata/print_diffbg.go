//go:build ignore

// print_diffbg.go renders candidate Diff +/- background colors as ANSI
// swatches so they can be eyeballed directly in the user's terminal.
//
// Why this is a separate script: the chat transcript strips ESC bytes,
// so any swatch printed via a tool result loses the actual colors. Run
// this locally to see the candidates with full 24-bit color.
//
// Usage:
//
//	go run testdata/print_diffbg.go
package main

import (
	"fmt"
	"strconv"
	"strings"
)

type pair struct {
	Name  string
	Green string // hex, e.g. "#0d3b13"
	Red   string
	Note  string
}

func main() {
	current := pair{Name: "0 (current)", Green: "#0d3b13", Red: "#3b0d0d", Note: "baseline"}
	candidates := []pair{
		{Name: "A", Green: "#102d15", Red: "#2d1013", Note: "slight desat + slight darker"},
		{Name: "B", Green: "#162a1a", Red: "#2a1618", Note: "moderate desat"},
		{Name: "C", Green: "#1c281f", Red: "#281c1f", Note: "heavy desat (near gray)"},
		{Name: "D", Green: "#0b2210", Red: "#220b0e", Note: "darker, similar sat"},
		{Name: "E", Green: "#142318", Red: "#23141a", Note: "mid-dark muted"},
		{Name: "F", Green: "#1a201c", Red: "#201a1c", Note: "very gray, only a tint"},
		{Name: "G", Green: "#0e1f12", Red: "#1f0e12", Note: "dark + slight desat"},
		{Name: "H", Green: "#172319", Red: "#23171a", Note: "mid muted"},
	}

	printHeading("Current (for reference)")
	printSwatch(current)
	printHeading("Candidates (彩度/明度を下げた案)")
	for _, p := range candidates {
		printSwatch(p)
	}

	fmt.Println()
	fmt.Println("色を選んだら 'A' 等の名前で教えてください。気に入る案がなければ追加で出します。")
}

func printHeading(s string) {
	fmt.Printf("\n\033[1m%s\033[0m\n", s)
}

func printSwatch(p pair) {
	gR, gG, gB := hexRGB(p.Green)
	rR, rG, rB := hexRGB(p.Red)
	const (
		plusFG  = "63;185;80"   // theme.DiffPlus  #3fb950
		minusFG = "248;81;73"   // theme.DiffMinus #f85149
		fgDef   = "212;212;212" // generic content
		fgKw    = "106;153;85"  // keyword-ish
		fgStr   = "206;145;120" // string-ish
		fgIdent = "156;220;254" // identifier-ish
		dimFG   = "117;109;89"  // line-number-ish
	)

	fmt.Printf("\n  \033[1m%-12s\033[0m  green=%s  red=%s   \033[2m%s\033[0m\n", p.Name, p.Green, p.Red, p.Note)
	// + row: leading line-number gutter (no bg) + bg-green cell with bold marker + tokens.
	fmt.Printf(
		"  \033[38;2;%sm   1\033[0m \033[48;2;%d;%d;%dm\033[1;38;2;%sm+\033[22;38;2;%sm \033[38;2;%sm\treturn\033[38;2;%sm \033[38;2;%sm\"hello, world\"\033[38;2;%sm                                              \033[0m\n",
		dimFG, gR, gG, gB, plusFG, fgDef, fgKw, fgDef, fgStr, fgDef,
	)
	// - row.
	fmt.Printf(
		"  \033[38;2;%sm   1\033[0m \033[48;2;%d;%d;%dm\033[1;38;2;%sm-\033[22;38;2;%sm \033[38;2;%sm\told := compute(\033[38;2;%smkey\033[38;2;%sm) \033[38;2;%sm// stale\033[38;2;%sm                              \033[0m\n",
		dimFG, rR, rG, rB, minusFG, fgDef, fgDef, fgIdent, fgDef, fgKw, fgDef,
	)
	// context row for adjacency, no bg.
	fmt.Printf(
		"  \033[38;2;%sm   2\033[0m  \033[38;2;%sm \tif true {                                              \033[0m\n",
		dimFG, fgDef,
	)
}

// hexRGB parses "#rrggbb" into three decimal channels. Panics on malformed
// input — it's a script, not production code.
func hexRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		panic("hex must be #rrggbb: " + hex)
	}
	r, err := strconv.ParseInt(hex[0:2], 16, 0)
	if err != nil {
		panic(err)
	}
	g, err := strconv.ParseInt(hex[2:4], 16, 0)
	if err != nil {
		panic(err)
	}
	b, err := strconv.ParseInt(hex[4:6], 16, 0)
	if err != nil {
		panic(err)
	}
	return int(r), int(g), int(b)
}
