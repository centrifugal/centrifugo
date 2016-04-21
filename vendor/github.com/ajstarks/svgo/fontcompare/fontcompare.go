// fontcompare: compare two fonts
// +build !appengine

package main

import (
	"fmt"
	"os"

	"github.com/ajstarks/svgo"
)

var (
	canvas = svg.New(os.Stdout)
	width  = 1000
	height = 600
	chars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789(){}[],.:;-+*/\\&_^%$#@!~`'\"<>"
	gstyle = "font-family:%s;font-size:%dpt;text-anchor:middle;fill:%s;fill-opacity:%.2f"
)

func letters(top, left int, font, color string, opacity float32) {
	rows := 7
	cols := 13
	glyph := 0
	fontsize := 32
	spacing := fontsize * 2
	x := left
	y := top
	canvas.Gstyle(fmt.Sprintf(gstyle, font, fontsize, color, opacity))
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			canvas.Text(x, y, chars[glyph:glyph+1])
			glyph++
			x += spacing
		}
		x = left
		y += spacing
	}
	canvas.Gend()
}

func main() {
	if len(os.Args) > 2 {
		canvas.Start(width, height)
		canvas.Rect(0, 0, width, height, "fill:white")
		canvas.Text(80, 540, os.Args[1], "font-size:14pt; fill:blue; font-family:"+os.Args[1])
		canvas.Text(80, 560, os.Args[2], "font-size:14pt; fill:red;  font-family:"+os.Args[2])
		letters(100, 100, os.Args[1], "blue", 0.5)
		letters(100, 100, os.Args[2], "red", 0.5)
		canvas.End()
	}
}
