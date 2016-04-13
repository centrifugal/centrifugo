// imfade progressively fades the Go gopher image
// +build !appengine

package main

import (
	"fmt"
	"os"

	"github.com/ajstarks/svgo"
)

var canvas = svg.New(os.Stdout)

func main() {
	width := 768
	height := 128
	image := "gophercolor128x128.png"
	if len(os.Args) > 1 {
		image = os.Args[1]
	}
	canvas.Start(width, height)
	canvas.Title("Image Fade")
	opacity := 1.0
	for i := 0; i < width-128; i += 100 {
		canvas.Image(i, 0, 128, 128, image, fmt.Sprintf("opacity:%.2f", opacity))
		opacity -= 0.10
	}
	canvas.Grid(0, 0, width, height, 16, "stroke:gray; opacity:0.2")

	canvas.End()
}
