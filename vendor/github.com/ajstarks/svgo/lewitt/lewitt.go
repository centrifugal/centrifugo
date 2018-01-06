// lewitt: inspired by by Sol LeWitt's Wall Drawing 91:
// +build !appengine

package main

//
// A six-inch (15 cm) grid covering the wall.
// Within each square, not straight lines from side to side, using
// red, yellow and blue pencils.  Each square contains at least
//  one line of each color.
//
// This version violates the original instructions in that straight lines
// as well as arcs are used

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/ajstarks/svgo"
)

var canvas = svg.New(os.Stdout)

const tilestyle = `stroke-width:1; stroke:rgb(128,128,128); stroke-opacity:0.5; fill:white`
const penstyle = `stroke:rgb%s; fill:none; stroke-opacity:%.2f; stroke-width:%d`

var width = 720
var height = 720

var nlines = flag.Int("n", 20, "number of lines/square")
var nw = flag.Int("w", 3, "maximum pencil width")
var pencils = []string{"(250, 13, 44)", "(247, 212, 70)", "(52, 114, 245)"}

func background(v int) { canvas.Rect(0, 0, width, height, canvas.RGB(v, v, v)) }

func lewitt(x int, y int, gsize int, n int, w int) {
	var x1, x2, y1, y2 int
	var op float64
	canvas.Rect(x, y, gsize, gsize, tilestyle)
	for i := 0; i < n; i++ {
		choice := rand.Intn(len(pencils))
		op = float64(random(1, 10)) / 10.0
		x1 = random(x, x+gsize)
		y1 = random(y, y+gsize)
		x2 = random(x, x+gsize)
		y2 = random(y, y+gsize)
		if random(0, 100) > 50 {
			canvas.Line(x1, y1, x2, y2, fmt.Sprintf(penstyle, pencils[choice], op, random(1, w)))
		} else {
			canvas.Arc(x1, y1, gsize, gsize, 0, false, true, x2, y2, fmt.Sprintf(penstyle, pencils[choice], op, random(1, w)))
		}
	}
}

func random(howsmall, howbig int) int {
	if howsmall >= howbig {
		return howsmall
	}
	return rand.Intn(howbig-howsmall) + howsmall
}

func init() {
	flag.Parse()
	rand.Seed(int64(time.Now().Nanosecond()) % 1e9)
}

func main() {

	canvas.Start(width, height)
	canvas.Title("Sol Lewitt's Wall Drawing 91")
	background(255)
	gsize := 120
	nc := width / gsize
	nr := height / gsize
	for cols := 0; cols < nc; cols++ {
		for rows := 0; rows < nr; rows++ {
			lewitt(cols*gsize, rows*gsize, gsize, *nlines, *nw)
		}
	}
	canvas.End()
}
