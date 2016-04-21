// paths draws the W3C logo as a paths
// +build !appengine

package main

import (
	"fmt"
	"os"

	"github.com/ajstarks/svgo"
)

var canvas = svg.New(os.Stdout)

func w3c() {
	w3path := `M36,5l12,41l12-41h33v4l-13,21c30,10,2,69-21,28l7-2c15,27,33,-22,3,-19v-4l12-20h-15l-17,59h-1l-13-42l-12,42h-1l-20-67h9l12,41l8-28l-4-13h9`
	cpath := `M94,53c15,32,30,14,35,7l-1-7c-16,26-32,3-34,0M122,16c-10-21-34,0-21,30c-5-30 16,-38 23,-21l5-10l-2-9`
	canvas.Path(w3path, "fill:#005A9C")
	canvas.Path(cpath)
}

func main() {
	canvas.Startview(700, 200, 0, 0, 700, 200)
	canvas.Title("Paths")
	for i := 0; i < 5; i++ {
		canvas.Gtransform(fmt.Sprintf("translate(%d,0)", i*130))
		w3c()
		canvas.Gend()
	}
	canvas.End()
}
