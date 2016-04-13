// flower - draw random flowers, inspired by Evelyn Eastmond's DesignBlocks gererated "grain2"
// +build !appengine

package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/ajstarks/svgo"
)

var (
	canvas    = svg.New(os.Stdout)
	niter     = flag.Int("n", 200, "number of iterations")
	width     = flag.Int("w", 500, "width")
	height    = flag.Int("h", 500, "height")
	thickness = flag.Int("t", 10, "max petal thinkness")
	np        = flag.Int("p", 15, "max number of petals")
	psize     = flag.Int("s", 30, "max length of petals")
	opacity   = flag.Int("o", 50, "max opacity (10-100)")
)

const flowerfmt = `stroke:rgb(%d,%d,%d); stroke-opacity:%.2f; stroke-width:%d`

func radial(xp int, yp int, n int, l int, style ...string) {
	var x, y, r, t, limit float64
	limit = 2.0 * math.Pi
	r = float64(l)
	canvas.Gstyle(style[0])
	for t = 0.0; t < limit; t += limit / float64(n) {
		x = r * math.Cos(t)
		y = r * math.Sin(t)
		canvas.Line(xp, yp, xp+int(x), yp+int(y))
	}
	canvas.Gend()
}

func random(howsmall, howbig int) int {
	if howsmall >= howbig {
		return howsmall
	}
	return rand.Intn(howbig-howsmall) + howsmall
}

func randrad(w int, h int, n int) {
	var x, y, r, g, b, o, s, t, p int
	for i := 0; i < n; i++ {
		x = rand.Intn(w)
		y = rand.Intn(h)
		r = rand.Intn(255)
		g = rand.Intn(255)
		b = rand.Intn(255)
		o = random(10, *opacity)
		s = random(10, *psize)
		t = random(2, *thickness)
		p = random(10, *np)
		radial(x, y, p, s, fmt.Sprintf(flowerfmt, r, g, b, float64(o)/100.0, t))
	}
}

func background(v int) { canvas.Rect(0, 0, *width, *height, canvas.RGB(v, v, v)) }

func init() {
	flag.Parse()
	rand.Seed(int64(time.Now().Nanosecond()) % 1e9)
}

func main() {
	canvas.Start(*width, *height)
	canvas.Title("Random Flowers")
	background(255)
	randrad(*width, *height, *niter)
	canvas.End()
}
