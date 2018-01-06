// randcomp visualizes random number generators
// +build !appengine

package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/ajstarks/svgo"
)

var canvas = svg.New(os.Stdout)

func main() {
	width := 512
	height := 256
	var n = 256
	var rx, ry int

	if len(os.Args) > 1 {
		n, _ = strconv.Atoi(os.Args[1])
	}

	f, _ := os.Open("/dev/urandom")
	x := make([]byte, n)
	y := make([]byte, n)
	f.Read(x)
	f.Read(y)
	f.Close()

	rand.Seed(int64(time.Now().Nanosecond()) % 1e9)
	canvas.Start(600, 400)
	canvas.Title("Random Integer Comparison")
	canvas.Desc("Comparison of Random integers: the random device & the Go rand package")
	canvas.Rect(0, 0, width/2, height, "fill:white; stroke:gray")
	canvas.Rect(width/2, 0, width/2, height, "fill:white; stroke:gray")

	canvas.Desc("Left: Go rand package (red), Right: /dev/urandom")
	canvas.Gstyle("stroke:none; fill-opacity:0.5")
	for i := 0; i < n; i++ {
		rx = rand.Intn(255)
		ry = rand.Intn(255)
		canvas.Circle(rx, ry, 5, canvas.RGB(127, 0, 0))
		canvas.Circle(int(x[i])+255, int(y[i]), 5, "fill:black")
	}
	canvas.Gend()

	canvas.Desc("Legends")
	canvas.Gstyle("text-anchor:middle; font-size:18; font-family:Calibri")
	canvas.Text(128, 280, "Go rand package", "")
	canvas.Text(384, 280, "/dev/urandom")
	canvas.Text(256, 280, fmt.Sprintf("n=%d", n), "font-size:12")
	canvas.Gend()
	canvas.End()
}
