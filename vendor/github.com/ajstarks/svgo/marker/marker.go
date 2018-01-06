// marker test
// +build !appengine

package main

import (
	"github.com/ajstarks/svgo"
	"os"
)

func main() {
	canvas := svg.New(os.Stdout)
	canvas.Start(500, 500)
	canvas.Title("Marker")

	canvas.Def()
	canvas.Marker("dot", 5, 5, 8, 8)
	canvas.Circle(5, 5, 3, "fill:black")
	canvas.MarkerEnd()

	canvas.Marker("box", 5, 5, 8, 8)
	canvas.CenterRect(5, 5, 6, 6, "fill:green")
	canvas.MarkerEnd()

	canvas.Marker("arrow", 2, 6, 13, 13)
	canvas.Path("M2,2 L2,11 L10,6 L2,2", "fill:blue")
	canvas.MarkerEnd()
	canvas.DefEnd()

	x := []int{100, 250, 100}
	y := []int{100, 250, 400}
	canvas.Polyline(x, y,
		`fill="none"`,
		`stroke="red"`,
		`marker-start="url(#dot)"`,
		`marker-mid="url(#arrow)"`,
		`marker-end="url(#box)"`)
	canvas.End()
}
