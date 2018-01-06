// turbulence example from http://www.w3.org/TR/2003/REC-SVG11-20030114/filters.html#feTurbulence
// +build !appengine

package main

import (
	"fmt"
	"github.com/ajstarks/svgo"
	"os"
)

var (
	canvas = svg.New(os.Stdout)
	width  = 500
	height = 500
)

type perlin struct {
	id        string
	ftype     string
	basefreqx float64
	basefreqy float64
	octave    int
	seed      int64
	tile      bool
}

func (p perlin) defturbulence() {
	x := svg.Filterspec{}
	canvas.Filter(p.id)
	canvas.FeTurbulence(x, p.ftype, p.basefreqx, p.basefreqy, p.octave, p.seed, p.tile)
	canvas.Fend()
}

func (p perlin) frect(x, y, w, h int) {
	bot := y + h
	canvas.Rect(x, y, w, h, fmt.Sprintf(`filter="url(#%s)"`, p.id))
	canvas.Text(x+w/2, bot+25, fmt.Sprintf("type=%s", p.ftype))
	canvas.Text(x+w/2, bot+40, fmt.Sprintf("baseFrequency=%.2f", p.basefreqx))
	canvas.Text(x+w/2, bot+55, fmt.Sprintf("numOctaves=%d", p.octave))
}

func main() {
	var t1, t2, t3, t4, t5, t6 perlin

	t1 = perlin{"Turb1", "t", 0.05, 0.05, 2, 0, false}
	t2 = perlin{"Turb2", "t", 0.10, 0.10, 2, 0, false}
	t3 = perlin{"Turb3", "t", 0.05, 0.05, 8, 0, false}
	t4 = perlin{"Turb4", "f", 0.10, 0.10, 4, 0, false}
	t5 = perlin{"Turb5", "f", 0.40, 0.40, 4, 0, false}
	t6 = perlin{"Turb6", "f", 0.10, 0.10, 1, 0, false}

	canvas.Start(width, height)
	canvas.Title("Example of feTurbulence")
	canvas.Def()
	t1.defturbulence()
	t2.defturbulence()
	t3.defturbulence()
	t4.defturbulence()
	t5.defturbulence()
	t6.defturbulence()
	canvas.DefEnd()

	canvas.Gstyle("font-size:10;font-family:Verdana;text-anchor:middle")
	t1.frect(25, 25, 100, 75)
	t2.frect(175, 25, 100, 75)
	t3.frect(325, 25, 100, 75)
	t4.frect(25, 180, 100, 75)
	t5.frect(175, 180, 100, 75)
	t6.frect(325, 180, 100, 75)
	canvas.Gend()
	canvas.End()
}
