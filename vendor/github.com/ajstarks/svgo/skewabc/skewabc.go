// skewabc - exercise the skew functions
// +build !appengine

package main

import (
	"fmt"
	"os"

	"github.com/ajstarks/svgo"
)

var (
	g      = svg.New(os.Stdout)
	width  = 500
	height = 500
)

func sky(x, y, w, h int, a float64, s string) {
	g.Gstyle(fmt.Sprintf("font-family:sans-serif;font-size:%dpx;text-anchor:middle", w/2))
	g.SkewY(a)
	g.Rect(x, y, w, h, `fill:black; fill-opacity:0.3`)
	g.Text(x+w/2, y+h/2, s, `fill:white;baseline-shift:-33%`)
	g.Gend()
	g.Gend()
}

func main() {
	g.Start(width, height)
	g.Title("Skew")
	g.Rect(0, 0, width, height, "fill:white")
	g.Grid(0, 0, width, height, 50, "stroke:lightblue")
	sky(100, 100, 100, 100, 30, "A")
	sky(200, 332, 100, 100, -30, "B")
	sky(300, -15, 100, 100, 30, "C")
	g.End()
}
