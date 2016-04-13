// vismem visualizes memory locations
// +build !appengine

package main

import (
	"os"

	"github.com/ajstarks/svgo"
)

var canvas = svg.New(os.Stdout)

func main() {
	width := 512
	height := 512
	n := 1024
	rowsize := 32
	diameter := 16
	var value int
	var source string

	if len(os.Args) > 1 {
		source = os.Args[1]
	} else {
		source = "/dev/urandom"
	}

	f, _ := os.Open(source)
	mem := make([]byte, n)
	f.Read(mem)
	f.Close()

	canvas.Start(width, height)
	canvas.Title("Visualize Files")
	canvas.Rect(0, 0, width, height, "fill:white")
	dx := diameter / 2
	dy := diameter / 2
	canvas.Gstyle("fill-opacity:1.0")
	for i := 0; i < n; i++ {
		value = int(mem[i])
		if i%rowsize == 0 && i != 0 {
			dx = diameter / 2
			dy += diameter
		}
		canvas.Circle(dx, dy, diameter/2, canvas.RGB(value, value, value))
		dx += diameter
	}
	canvas.Gend()
	canvas.End()
}
