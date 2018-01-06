// cube: draw cubes
package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"os"

	"github.com/ajstarks/svgo"
)

var canvas = svg.New(os.Stdout)

// randcolor returns a random color
func randcolor() string {
	rgb := []byte{0, 0, 0} // read error returns black
	rand.Read(rgb)
	return fmt.Sprintf("fill:rgb(%d,%d,%d)", rgb[0], rgb[1], rgb[2])
}

// rcube makes a cube with three visible faces, each with a random color
func rcube(x, y, l int) {
	tx := []int{x, x + (l * 3), x, x - (l * 3), x}
	ty := []int{y, y + (l * 2), y + (l * 4), y + (l * 2), y}

	lx := []int{x - (l * 3), x, x, x - (l * 3), x - (l * 3)}
	ly := []int{y + (l * 2), y + (l * 4), y + (l * 8), y + (l * 6), y + (l * 2)}

	rx := []int{x + (l * 3), x + (l * 3), x, x, x + (l * 3)}
	ry := []int{y + (l * 2), y + (l * 6), y + (l * 8), y + (l * 4), y + (l * 2)}

	canvas.Polygon(tx, ty, randcolor())
	canvas.Polygon(lx, ly, randcolor())
	canvas.Polygon(rx, ry, randcolor())
}

// lattice draws a grid of cubes, n rows deep.
// The grid begins at (xp, yp), with hspace between cubes in a row, and vspace between rows.
func lattice(xp, yp, w, h, size, hspace, vspace, n int, bgcolor string) {
	if bgcolor == "" {
		canvas.Rect(0, 0, w, h, randcolor())
	} else {
		canvas.Rect(0, 0, w, h, "fill:"+bgcolor)
	}
	y := yp
	for r := 0; r < n; r++ {
		for x := xp; x < w; x += hspace {
			rcube(x, y, size)
		}
		y += vspace
	}
}

func main() {
	var (
		width  = flag.Int("w", 600, "canvas width")
		height = flag.Int("h", 600, "canvas height")
		x      = flag.Int("x", 60, "begin x location")
		y      = flag.Int("y", 60, "begin y location")
		size   = flag.Int("size", 20, "cube size")
		rows   = flag.Int("rows", 3, "rows")
		hs     = flag.Int("hs", 120, "horizontal spacing")
		vs     = flag.Int("vs", 160, "vertical spacing")
		bg     = flag.String("bg", "", "background")
	)
	flag.Parse()
	canvas.Start(*width, *height)
	lattice(*x, *y, *width, *height, *size, *hs, *vs, *rows, *bg)
	canvas.End()
}
