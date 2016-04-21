// svgopher - Go mascot remix
// +build !appengine

package main

import (
	"os"

	"github.com/ajstarks/svgo"
)

var (
	width  = 500
	height = 300
	canvas = svg.New(os.Stdout)
)

func background(v int) { canvas.Rect(0, 0, width, height, canvas.RGB(v, v, v)) }

func gordon(x, y, w, h int) {

	w10 := w / 10
	w12 := w / 12
	w2 := w / 2
	w3 := w / 3
	w8 := w / 8
	w6 := w / 6
	xw := x + w
	h23 := (h * 2) / 3

	blf := "fill:black"
	wf := "fill:white"
	nf := "fill:brown"
	brf := "fill:brown; fill-opacity:0.2"
	brb := "fill:rgb(210,161,161)"

	canvas.Gstyle("fill:none; stroke:none")
	canvas.Bezier(x, y+h, x, y+h, x+w2, y-h, x+w, y+h, brb)
	canvas.Roundrect(x, y+(h-1), w, h, 10, 10, brb)
	canvas.Circle(x, y+h, w12, brf) // left ear
	canvas.Circle(x, y+h, w12-10, nf)

	canvas.Circle(x+w, y+h, w12, brf) // right ear
	canvas.Circle(x+w, y+h, w12-10, nf)

	canvas.Circle(x+w3, y+h23, w/9, wf) // left eye
	canvas.Circle(x+w3+10, y+h23, w10-10, blf)
	canvas.Circle(x+w3+15, y+h23, 5, wf)

	canvas.Circle(xw-w3, y+h23, w/9, wf) // right eye
	canvas.Circle(xw-w3+10, y+h23, w10-10, blf)
	canvas.Circle(xw-(w3)+15, y+h23, 5, wf)

	canvas.Roundrect(x+w2-w8, y+h+30, w8, w6, 5, 5, wf) // left tooth
	canvas.Roundrect(x+w2, y+h+30, w8, w6, 5, 5, wf)    // right tooth

	canvas.Ellipse(x+(w2), y+h+30, w6, w12, nf)   // snout
	canvas.Ellipse(x+(w2), y+h+10, w10, w12, blf) // nose

	canvas.Gend()
}

func main() {
	canvas.Start(width, height)
	canvas.Title("SVG Gopher")
	background(255)
	canvas.Gtransform("translate(100, 100)")
	canvas.Gtransform("rotate(-30)")
	gordon(48, 48, 240, 72)
	canvas.Gend()
	canvas.Gend()
	canvas.Link("svgdef.svg", "SVG Spec & Usage")
	canvas.Text(90, 145, "SVG", "font-family:Calibri,sans-serif;font-size:80;fill:brown")
	canvas.LinkEnd()
	canvas.End()
}
