// pattern: test the pattern function
package main

import (
	"fmt"
	"github.com/ajstarks/svgo"
	"os"
)

func main() {
	canvas := svg.New(os.Stdout)
	w, h := 500, 500
	pct := 5
	pw, ph := (w*pct)/100, (h*pct)/100
	canvas.Start(w, h)

	// define the pattern
	canvas.Def()
	canvas.Pattern("hatch", 0, 0, pw, ph, "user")
	canvas.Gstyle("fill:none;stroke-width:1")
	canvas.Path(fmt.Sprintf("M0,0 l%d,%d", pw, ph), "stroke:red")
	canvas.Path(fmt.Sprintf("M%d,0 l-%d,%d", pw, pw, ph), "stroke:blue")
	canvas.Gend()
	canvas.PatternEnd()
	canvas.DefEnd()

	// use the pattern
	canvas.Gstyle("stroke:black; stroke-width:2")
	canvas.Circle(w/2, h/2, h/8, "fill:url(#hatch)")
	canvas.CenterRect((w*4)/5, h/2, h/4, h/4, "fill:url(#hatch)")
	canvas.Gend()
	canvas.End()
}
