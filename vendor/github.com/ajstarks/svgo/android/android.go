// android draws bugdroid, the Android mascot
// +build !appengine

package main

import (
	"fmt"
	"os"

	"github.com/ajstarks/svgo"
)

var (
	width  = 500
	height = 500
	canvas = svg.New(os.Stdout)
)

const androidcolor = "rgb(164,198,57)"

func background(v int) { canvas.Rect(0, 0, width, height, canvas.RGB(v, v, v)) }

func android(x, y int, fill string, opacity float64) {
	var linestyle = []string{`stroke="` + fill + `"`, `stroke-linecap="round"`, `stroke-width="5"`}
	globalstyle := fmt.Sprintf("fill:%s;opacity:%.2f", fill, opacity)
	canvas.Gstyle(globalstyle)
	canvas.Arc(x+30, y+70, 35, 35, 0, false, true, x+130, y+70)                     // head
	canvas.Line(x+60, y+25, x+50, y+10, linestyle[0], linestyle[1], linestyle[2])   // left antenna
	canvas.Line(x+100, y+25, x+110, y+10, linestyle[0], linestyle[1], linestyle[2]) // right antenna
	canvas.Circle(x+60, y+45, 5, "fill:white")                                      // left eye
	canvas.Circle(x+100, y+45, 5, `fill="white"`)                                   // right eye
	canvas.Roundrect(x+30, y+75, 100, 90, 10, 10)                                   // body
	canvas.Rect(x+30, y+75, 100, 80)
	canvas.Roundrect(x+5, y+80, 20, 70, 10, 10)   // left arm
	canvas.Roundrect(x+135, y+80, 20, 70, 10, 10) // right arm
	canvas.Roundrect(x+50, y+150, 20, 50, 10, 10) // left leg
	canvas.Roundrect(x+90, y+150, 20, 50, 10, 10) // right leg
	canvas.Gend()
}

func main() {
	canvas.Start(width, height)
	canvas.Title("Android")
	background(255)

	android(100, 100, androidcolor, 1.0)
	canvas.Scale(3.0)
	android(50, 50, "gray", 0.5)
	canvas.Gend()

	canvas.Scale(0.5)
	android(100, 100, "red", 1.0)
	canvas.Gend()
	canvas.End()
}
