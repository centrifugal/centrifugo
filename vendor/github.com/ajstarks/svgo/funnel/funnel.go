// funnel draws a funnel-like shape
// +build !appengine

package main

import (
	"os"

	"github.com/ajstarks/svgo"
)

var canvas = svg.New(os.Stdout)
var width = 320
var height = 480

func funnel(bg int, fg int, grid int, dim int) {
	h := dim / 2
	canvas.Rect(0, 0, width, height, canvas.RGB(bg, bg, bg))
	for size := grid; size < width; size += grid {
		canvas.Ellipse(h, size, size/2, size/2, canvas.RGBA(fg, fg, fg, 0.2))
	}
}

func main() {
	canvas.Start(width, height)
	canvas.Title("Funnel")
	funnel(0, 255, 25, width)
	canvas.End()
}
