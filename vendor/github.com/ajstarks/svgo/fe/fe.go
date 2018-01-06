// fe: SVG Filter Effect example from http://www.w3.org/TR/SVG/filters.html#AnExample
// +build !appengine

package main


import (
	"github.com/ajstarks/svgo"
	"os"
)

func main() {

	canvas := svg.New(os.Stdout)
	width := 410
	height := 120

	canvas.Start(width, height)
	canvas.Title(`SVGo Filter Example`)
	canvas.Desc(`Combines multiple filter primitives to produce a 3D lighting effect`)

	gfs := svg.Filterspec{In: "SourceAlpha", Result: "blur"}
	ofs := svg.Filterspec{In: "blur", Result: "offsetBlur"}
	sfs := svg.Filterspec{In: "blur", Result: "specOut"}
	cfs1 := svg.Filterspec{In: "specOut", In2: "SourceAlpha", Result: "specOut"}
	cfs2 := svg.Filterspec{In: "SourceGraphic", In2: "specOut", Result: "litPaint"}

	// define the filters
	canvas.Def()
	canvas.Filter("myFilter")
	canvas.FeGaussianBlur(gfs, 4, 4)
	canvas.FeOffset(ofs, 4, 4)
	canvas.FeSpecularLighting(sfs, 5, .75, 20, "#bbbbbb")
	canvas.FePointLight(-5000, -10000, 20000)
	canvas.FeSpecEnd()
	canvas.FeComposite(cfs1, "in", 0, 0, 0, 0)
	canvas.FeComposite(cfs2, "arithmetic", 0, 1, 1, 0)
	canvas.FeMerge([]string{ofs.Result, cfs2.Result})
	canvas.Fend()
	canvas.DefEnd()

	// specify the graphic
	canvas.Gid("SVG")
	canvas.Path("M50,90 C0,90 0,30 50,30 L150,30 C200,30 200,90 150,90 z", "fill:none;stroke:#D90000;stroke-width:10")
	canvas.Path("M60,80 C30,80 30,40 60,40 L140,40 C170,40 170,80 140,80 z", "fill:#D90000")
	canvas.Text(52, 76, "SVG", "fill:white;stroke:black;font-size:45;font-family:Verdana")
	canvas.Gend()

	canvas.Rect(0, 0, width, height, "stroke:black;fill:white")
	canvas.Use(0, 0, "#SVG")                              // plain graphic
	canvas.Use(200, 0, "#SVG", `filter="url(#myFilter)"`) // filter applied
	canvas.End()
}
