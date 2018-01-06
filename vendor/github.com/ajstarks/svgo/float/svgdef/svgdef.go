// svgdef -  SVG Object Definition and Use
// +build !appengine

package main

import (
	"math"
	"os"

	"github.com/ajstarks/svgo/float"
)

const (
	textsize    = 24
	coordsize   = 4
	objcolor    = "rgb(0,0,127)"
	objstyle    = "fill:none; stroke-width:2;stroke:" + objcolor
	fobjstyle   = "fill-opacity:0.25;fill:" + objcolor
	legendstyle = "fill:gray; text-anchor:middle"
	titlestyle  = "fill:black; text-anchor:middle;font-size:24px"
	linestyle   = "stroke:black; stroke-width:1"
	gtextstyle  = "font-family:Calibri,sans; text-anchor:middle; font-size:24px"
	coordstring = "x, y"
	tpathstring = `It's "fine" & "dandy" to draw text along a path`
)

var (
	canvas   = svg.New(os.Stdout)
	grayfill = canvas.RGB(220, 220, 220)
	oc1      = svg.Offcolor{Offset: 0, Color: "white", Opacity: 1.0}
	oc2      = svg.Offcolor{Offset: 25, Color: "lightblue", Opacity: 1.0}
	oc3      = svg.Offcolor{Offset: 75, Color: "blue", Opacity: 1.0}
	oc4      = svg.Offcolor{Offset: 100, Color: objcolor, Opacity: 1.0}
	ga       = []svg.Offcolor{oc1, oc2, oc3, oc4}
)

// defcoodstr defines coordinate strings: (x,y)
func defcoordstr(x float64, y float64, s string) {
	canvas.Circle(x, y, coordsize, grayfill)
	canvas.Text(x, y-textsize, s, legendstyle)
}

// defcoord defines a coordinate
func defcoord(x, y, n float64) {
	canvas.Circle(x, y, coordsize, grayfill)
	canvas.Text(x, y+n, coordstring, legendstyle)
}

// deflegend makes object legends
func deflegend(x float64, y float64, size float64, legend string) {
	canvas.Text(x, y+size+textsize, legend, titlestyle)
}

// defcircle defines the circle object for arbitrary placement and size
func defcircle(id string, w, h float64, legend string) {
	canvas.Gid(id)
	canvas.Translate(w, h)
	defcoord(0, 0, -textsize)
	canvas.Circle(0, 0, h, objstyle)
	canvas.Line(0, 0, h, 0, linestyle)
	canvas.Text(h/2, textsize, "r", legendstyle)
	deflegend(0, 0, h, legend)
	canvas.Gend()
	canvas.Gend()
}

// defellipse defines the ellipse object for arbitrary placement and size
func defellipse(id string, w float64, h float64, legend string) {
	canvas.Gid(id)
	canvas.Translate(w, h)
	defcoord(0, 0, -textsize)
	canvas.Ellipse(0, 0, w, h, objstyle)
	canvas.Line(0, 0, w, 0, linestyle)
	canvas.Line(0, 0, 0, h, linestyle)
	canvas.Text(w/2, textsize, "rx", legendstyle)
	canvas.Text(-textsize, (h / 2), "ry", legendstyle)
	deflegend(0, 0, h, legend)
	canvas.Gend()
	canvas.Gend()
}

// defrect defines the rectangle object for arbitrary placement and size
func defrect(id string, w float64, h float64, legend string) {
	canvas.Gid(id)
	defcoord(0, 0, -textsize)
	canvas.Rect(0, 0, w, h, objstyle)
	canvas.Text(-textsize, (h / 2), "h", legendstyle)
	canvas.Text((w / 2), -textsize, "w", legendstyle)
	deflegend((w / 2), 0, h, legend)
	canvas.Gend()
}

// defcrect defines the centered rectangle object for arbitrary placement and size
func defcrect(id string, w float64, h float64, legend string) {
	canvas.Gid(id)
	defcoord(w/2, h/2, -textsize)
	canvas.Rect(0, 0, w, h, objstyle)
	canvas.Text(-textsize, (h / 2), "h", legendstyle)
	canvas.Text((w / 2), -textsize, "w", legendstyle)
	deflegend((w / 2), 0, h, legend)
	canvas.Gend()
}

// defsquare defines the square object for arbitrary placement and size
func defsquare(id string, w float64, legend string) {
	canvas.Gid(id)
	defcoord(0, 0, -textsize)
	canvas.Square(0, 0, w, objstyle)
	canvas.Text((w / 2), -textsize, "w", legendstyle)
	deflegend((w / 2), 0, w, legend)
	canvas.Gend()
}

// defimage defines the image object for arbitrary placement and size
func defimage(id string, w float64, h float64, s string, legend string) {
	canvas.Gid(id)
	defcoord(0, 0, -textsize)
	canvas.Rect(0, 0, w, h, objstyle)
	canvas.Text(-textsize, (h / 2), "h", legendstyle)
	canvas.Text((w / 2), -textsize, "w", legendstyle)
	canvas.Image(0, 0, int(w), int(h), s)
	deflegend(w/2, h, 0, legend)
	canvas.Gend()
}

// defline defines the line object for arbitrary placement and size
func defline(id string, w float64, h float64, legend string) {
	canvas.Gid(id)
	defcoordstr(0, 0, "x1, y1")
	defcoordstr(w, 0, "x2, y2")
	canvas.Line(0, 0, w, 0, objstyle)
	deflegend(w/2, h, 0, legend)
	canvas.Gend()
}

// defarc defines the arc object for arbitrary placement and size
func defarc(id string, w float64, h float64, legend string) {
	canvas.Gid(id)
	defcoordstr(0, 0, "sx, sy")
	defcoordstr(w*2, 0, "ex, ey")
	canvas.Arc(0, 0, h, h, 0, false, true, w*2, 0, objstyle)
	deflegend(w, h, h, legend)
	canvas.Gend()
}

// defbez defines the cublic bezier object for arbitrary placement and size
func defbez(id string, x float64, y float64, h float64, legend string) {
	sx, sy := 0.0, 0.0
	cx, cy := x, -y
	px, py := x, y
	ex, ey := x*2, 0.0
	canvas.Gid(id)
	defcoordstr(sx, sy, "sx, sy")
	defcoordstr(cx, cy, "cx, cy")
	defcoordstr(px, py, "px, py")
	defcoordstr(ex, ey, "ex, ey")
	canvas.Bezier(sx, sy, cx, cy, px, py, ex, ey, objstyle)
	deflegend(px, h, 0, legend)
	canvas.Gend()
}

// defqbez defines the quadratic bezier object for arbitrary placement and size
func defqbez(id string, px float64, py float64, h float64, legend string) {
	sx, sy := 0.0, 0.0
	ex, ey := px*2, 0.0
	cx, cy := (ex-px)/3, -py-(py/2)
	canvas.Gid(id)
	defcoordstr(sx, sy, "sx, sy")
	defcoordstr(cx, cy, "cx, cy")
	defcoordstr(ex, ey, "ex, ey")
	canvas.Qbez(sx, sy, cx, cy, ex, ey, objstyle)
	deflegend(px, h, 0, legend)
	canvas.Gend()
}

// defroundrect defines the roundrect object for arbitrary placement and size
func defroundrect(id string, w float64, h float64, rx float64, ry float64, legend string) {
	canvas.Gid(id)
	defcoord(0, 0, -textsize)
	canvas.Roundrect(0, 0, w, h, rx, ry, objstyle)
	canvas.Text(-textsize, (h / 2), "h", legendstyle)
	canvas.Text((w / 2), -textsize, "w", legendstyle)
	canvas.Line(rx, 0, rx, ry, linestyle)
	canvas.Line(0, ry, rx, ry, linestyle)
	canvas.Text(rx+textsize, ry-(ry/2), "ry", legendstyle)
	canvas.Text((rx / 2), ry+textsize, "rx", legendstyle)
	deflegend((w / 2), 0, h, legend)
	canvas.Gend()
}

// defpolygon defines the polygon object for arbitrary placement and size
func defpolygon(id string, w float64, h float64, legend string) {
	var x = []float64{0, w / 2, w, w, w / 2, 0}
	var y = []float64{0, -h / 4, 0, (h * 3) / 4, h / 2, (h * 3) / 4}
	canvas.Gid(id)
	for i := 0; i < len(x); i++ {
		defcoord(x[i], y[i], -textsize)
	}
	canvas.Polygon(x, y, objstyle)
	deflegend(w/2, h, 0, legend)
	canvas.Gend()
}

// defpolyline defines the polyline object for arbitrary placement and size
func defpolyline(id string, w float64, h float64, legend string) {
	var x = []float64{0, w / 3, (w * 3) / 4, w}
	var y = []float64{0, -(h / 2), -(h / 3), -h}
	canvas.Gid(id)
	for i := 0; i < len(x); i++ {
		defcoord(x[i], y[i], -textsize)
	}
	canvas.Polyline(x, y, objstyle)
	deflegend(w/2, h, 0, legend)
	canvas.Gend()
}

// defpath defines the path object for arbitrary placement and size
func defpath(id string, x, y float64, legend string) {
	var w3path = `M36,5l12,41l12-41h33v4l-13,21c30,10,2,69-21,28l7-2c15,27,33,-22,3,-19v-4l12-20h-15l-17,59h-1l-13-42l-12,42h-1l-20-67h9l12,41l8-28l-4-13h9`
	var cpath = `M94,53c15,32,30,14,35,7l-1-7c-16,26-32,3-34,0M122,16c-10-21-34,0-21,30c-5-30 16,-38 23,-21l5-10l-2-9`
	canvas.Gid(id)
	canvas.Path(w3path, `fill="`+objcolor+`"`)
	canvas.Path(cpath, canvas.RGBA(0, 0, 0, 0.5))
	defcoord(0, 0, -textsize)
	deflegend(x/2, y+50, textsize, legend)
	canvas.Gend()
}

// deflg defines the linear gradient object for arbitrary placement and size
func deflg(id string, w float64, h float64, legend string) {
	canvas.Gid(id)
	canvas.Rect(0, 0, w, h, "fill:url(#linear)")
	defcoordstr(0, 0, "x1%, y1%")
	defcoordstr(w, 0, "x2%, y2%")
	deflegend((w / 2), 0, h, legend)
	canvas.Gend()
}

// defrg defines the radial gradient object for arbitrary placement and size
func defrg(id string, w float64, h float64, legend string) {
	canvas.Gid(id)
	canvas.Rect(0, 0, w, h, "fill:url(#radial)")
	defcoordstr(0, 0, "cx%, cy%")
	defcoordstr(w/2, h/2, "fx%, fy%")
	deflegend((w / 2), 0, h, legend)
	canvas.Gend()
}

// deftrans defines the trans object for arbitrary placement and size
func deftrans(id string, w, h float64, legend string) {
	tx := w / 3
	canvas.Gid(id)
	defcoordstr(0, 0, "0, 0")
	defcoordstr(w-tx, 0, "x, y")
	deflegend(w/2, 0, h, legend)
	canvas.Rect(0, 0, tx, h, objstyle)
	canvas.Translate(w-tx, 0)
	canvas.Rect(0, 0, tx, h, fobjstyle)
	canvas.Gend()
	canvas.Gend()
}

// defgrid defines the grid object for arbitrary placement and size
func defgrid(id string, w, h float64, legend string) {
	n := h / 4
	canvas.Gid(id)
	defcoord(0, 0, -textsize)
	canvas.Text(-textsize, (h / 2), "h", legendstyle)
	canvas.Text((w / 2), -textsize, "w", legendstyle)
	canvas.Text(n+textsize, n/2, "n", legendstyle)
	canvas.Grid(0, 0, w, h, n, "stroke:"+objcolor)
	deflegend((w / 2), 0, h, legend)
	canvas.Gend()
}

// deftext defines the text object for arbitrary placement and size
func deftext(id string, w, h float64, text string, legend string) {
	canvas.Gid(id)
	defcoord(0, h/2, textsize)
	canvas.Text(0, h/2, text, "text-anchor:start;font-size:32pt")
	deflegend(w/2, 0, h, legend)
	canvas.Gend()
}

// deftextpath defines the textpath object for arbitrary placement and size
func deftextpath(id string, pathid string, s string, w, h float64, legend string) {
	canvas.Gid(id)
	canvas.Textpath(s, pathid, `fill="`+objcolor+`"`, `text-anchor="start"`, `font-size="16pt"`)
	deflegend(w/2, 0, h, legend)
	canvas.Gend()
}

// defscale defines the scale object for arbitrary placement and size
func defscale(id string, w, h float64, n float64, legend string) {
	canvas.Gid(id)
	defcoordstr(0, 0, "0, 0")
	canvas.Rect(0, 0, w, h, objstyle)
	canvas.Scale(n)
	canvas.Rect(0, 0, w, h, fobjstyle)
	canvas.Gend()
	deflegend(w/2, 0, h, legend)
	canvas.Gend()
}

// defscaleXY defines the scaleXY object for arbitrary placement and size
func defscaleXY(id string, w, h float64, dx, dy float64, legend string) {
	canvas.Gid(id)
	defcoordstr(0, 0, "0, 0")
	canvas.Rect(0, 0, w, h, objstyle)
	canvas.ScaleXY(dx, dy)
	canvas.Rect(0, 0, w, h, fobjstyle)
	canvas.Gend()
	deflegend(w/2, 0, h, legend)
	canvas.Gend()
}

// defskewX defines the skewX object for arbitrary placement and size
func defskewX(id string, w, h float64, angle float64, legend string) {
	canvas.Gid(id)
	defcoordstr(0, 0, "0, 0")
	canvas.Rect(0, 0, w, h, objstyle)
	canvas.SkewX(angle)
	canvas.Rect(0, 0, w, h, fobjstyle)
	canvas.Gend()
	deflegend(w/2, 0, h, legend)
	canvas.Gend()
}

// defskewY defines the skewY object for arbitrary placement and size
func defskewY(id string, w, h float64, angle float64, legend string) {
	canvas.Gid(id)
	defcoordstr(0, 0, "0, 0")
	canvas.Rect(0, 0, w, h, objstyle)
	canvas.SkewY(angle)
	canvas.Rect(0, 0, w, h, fobjstyle)
	canvas.Gend()
	deflegend(w/2, 0, h, legend)
	canvas.Gend()
}

// defskewXY defines the skewXY object for arbitrary placement and size
func defskewXY(id string, w, h float64, ax, ay float64, legend string) {
	canvas.Gid(id)
	defcoordstr(0, 0, "0, 0")
	canvas.Rect(0, 0, w, h, objstyle)
	canvas.SkewXY(ax, ay)
	canvas.Rect(0, 0, w, h, fobjstyle)
	canvas.Gend()
	deflegend(w/2, 0, h, legend)
	canvas.Gend()
}

// defrotate defines the rotate object for arbitrary placement and size
func defrotate(id string, w, h float64, deg float64, legend string) {
	t := deg * (math.Pi / 180.0)
	r := float64(w / 2)
	rx := r * math.Cos(t)
	ry := r * math.Sin(t)
	canvas.Gid(id)
	defcoordstr(0, 0, "0, 0")
	deflegend(w/2, 0, h, legend)
	canvas.Rect(0, 0, w, h, objstyle)
	canvas.Qbez(w/2, 0, (w/2)+10, ry/2, rx, ry, "fill:none;stroke:gray")
	canvas.Text(w/4, textsize, "r", legendstyle)
	canvas.Rotate(deg)
	canvas.Rect(0, 0, w, h, fobjstyle)
	canvas.Gend()
	canvas.Gend()
}

// defmeta defines the metadata objects
func defmeta(id string, w float64, name, desc []string, legend string) {
	canvas.Gid(id)
	canvas.Textlines(0, textsize, name, 24.0, 28.0, "black", "start")
	canvas.Textlines(w+150, textsize, desc, 24.0, 28.0, "rgb(127,127,127)", "start")
	deflegend(w, 0, 30*float64(len(name)), legend)
	canvas.Gend()
}

// defrgb defines the rgb object for arbitrary placement and size
func defrgb(id string, w, h float64, r, g, b int, opacity float64, legend string) {
	size := h / 8
	canvas.Gid(id)
	canvas.Gstyle(legendstyle)
	colordot(w/4, 0.0, size, r, 0, 0, 1.0)
	colordot(w/2, 0.0, size, 0, g, 0, 1.0)
	colordot(w*3/4, 0.0, size, 0, 0, b, 1.0)
	colordot(w, 0.0, size, r, g, b, opacity)
	if opacity < 1.0 {
		colordot(w+10.0, 0.0, size, r, g, b, opacity)
		canvas.Text(w, h/2, "alpha")
	}
	canvas.Text(w/4, h/2, "r")
	canvas.Text(w/2, h/2, "g")
	canvas.Text(w*3/4, h/2, "b")
	canvas.Text(w-(w/8), size-size/2, "->")
	canvas.Gend()
	deflegend(w/2, 0, h, legend)
	canvas.Gend()
}

// defobjects defines a set of objects with the specified dimensions,
// once defined, the objects are referenced for placement
func defobjects(w, h float64) {
	var (
		metatext = []string{
			"New(w io Writer)",
			"Start(w, h float64, options ...string)/End()",
			"Startview(w, h, minx, miny, vw, vh float64)",
			"Group(s ...string)/Gend()",
			"Gstyle(s string)/Gend()",
			"Gtransform(s string)/Gend()",
			"Gid(id string)/Gend()",
			"ClipPath(s ...string)/ClipEnd()",
			"Def()/DefEnd()",
			"Marker()/MarkerEnd()",
			"Pattern()/PatternEnd()",
			"Desc(s string)",
			"Title(s string)",
			"Script(type, data ...string)",
			"Mask(id string, x,y,w,h float64, style ...string)/MaskEnd()",
			"Link(href string, title string)/LinkEnd()",
			"Use(x float64, y float64, link string, style ...string)",
		}
		metadesc = []string{
			"specify destination",
			"begin/end the document",
			"begin/end the document with viewport",
			"begin/end group with attributes",
			"begin/end group style",
			"begin/end group transform",
			"begin/end group id",
			"begin/end clip path",
			"begin/end a defintion block",
			"begin/end markers",
			"begin/end pattern",
			"set the description element",
			"set the title element",
			"define a script",
			"begin/end mask element",
			"begin/end link to href, with a title",
			"use defined objects",
		}
	)
	h2 := h / 2
	canvas.Desc("Object Definitions")
	canvas.Def()
	canvas.LinearGradient("linear", 0, 0, 100, 0, ga)
	canvas.RadialGradient("radial", 0, 0, 100, 50, 50, ga)
	canvas.Path("M 0,0 A62,62 0 0 1 250,0", `id="tpath"`)
	defsquare("square", h, "Square(x, y, w float64, style ...string)")
	defrect("rect", w, h, "Rect(x, y, w, h float64, style ...string)")
	defcrect("crect", w, h, "CenterRect(x, y, w, h float64, style ...string)")
	defroundrect("roundrect", w, h, 25, 25, "Roundrect(x, y, w, h, rx, ry float64, style ...string)")
	defpolygon("polygon", w, h, "Polygon(x, y []float64, style ...string)")
	defcircle("circle", h, h2, "Circle(x, y, r float64, style ...string)")
	defellipse("ellipse", h, h2, "Ellipse(x, y, rx, ry float64, style ...string)")
	defline("line", w, h, "Line(x1, y1, x2, y2 float64, style ...string)")
	defpolyline("polyline", w, h, "Polyline(x, y []float64, style ...string)")
	defarc("arc", h, h2, "Arc(sx, sy, ax, ay, r float64, lflag, sflag bool, ex, ey float64, style ...string)")
	defpath("path", h, h2, "Path(s string, style ...string)")
	defqbez("qbez", h, h2, h, "Qbez(sx, sy, cx, cy, ex, ey float64, style ...string)")
	defbez("bezier", h, h2, h, "Bezier(sx, sy, cx, cy, px, py, ex, ey float64, style ...string)")
	defimage("image", 128, 128, "gophercolor128x128.png", "Image(x, y float64, w, h int,  path string, style ...string)")
	deflg("lgrad", w, h, "LinearGradient(s string, x1, y1, x2, y2 uint8, oc []Offcolor)")
	defrg("rgrad", w, h, "RadialGradient(s string, cx, cy, r, fx, fy uint8, oc []Offcolor)")
	deftrans("trans", w, h, "Translate(x, y float64)")
	defgrid("grid", w, h, "Grid(x, y, w, h, n float64, style ...string)")
	deftext("text", w, h, "hello, this is SVG", "Text(x, y float64, s string, style ...string)")
	defscale("scale", w, h, 0.5, "Scale(n float64)")
	defscaleXY("scalexy", w, h, 0.5, 0.75, "ScaleXY(x, y float64)")
	defskewX("skewx", w, h, 30, "SkewX(a float64)")
	defskewY("skewy", w, h, 10, "SkewY(a float64)")
	defskewXY("skewxy", w, h, 10, 10, "SkewXY(x, y float64)")
	defrotate("rotate", w, h, 30, "Rotate(r float64)")
	deftextpath("textpath", "#tpath", tpathstring, w, h, "Textpath(s, pathid string, style ...string)")
	defmeta("meta", w*2, metatext, metadesc, "Textlines(x, y float64, s []string, size, spacing float64, fill, align string)")
	defrgb("rgb", w, h, 44, 77, 232, 1.0, "RGB(r, g, b int)")
	defrgb("rgba", w, h, 44, 77, 232, 0.33, "RGBA(r, g, b int, opacity float64)")
	canvas.DefEnd()
}

// colordot makes a colored dot, with opacity
func colordot(x, y, r float64, red, green, blue int, a float64) {
	// canvas.Circle(x,y,r+textsize/6,"fill:none;stroke:"+objcolor)
	if a == 1.0 {
		canvas.Circle(x, y, r, canvas.RGB(red, green, blue))
	} else {
		canvas.Circle(x, y, r, canvas.RGBA(red, green, blue, a))
	}
}

// placerow is a helper for placeobjects, placing to previously
// defined objects row-wise
func placerow(w float64, s []string) {
	for x, name := range s {
		canvas.Use(float64(x)*w, 0, "#"+name)
	}
}

// placeobjects places a grid of objects on the canvas as specified
// by a string array.
func placeobjects(x, y, w, h float64, data [][]string) {
	canvas.Desc("Object Usage")
	for _, object := range data {
		canvas.Translate(x, y)
		placerow(w, object)
		canvas.Gend()
		y += h
	}
}

var roworder = [][]string{
	{"rect", "crect", "roundrect", "square", "line", "polyline"},
	{"polygon", "circle", "ellipse", "arc", "qbez", "bezier"},
	{"trans", "scale", "scalexy", "skewx", "skewy", "skewxy"},
	{"rotate", "text", "textpath", "path", "image", "grid"},
	{"lgrad", "rgrad", "rgb", "rgba", "meta"},
}

func main() {
	width := 4500.0
	height := (width * 3) / 4
	canvas.Decimals = 0
	canvas.Start(width, height)
	defobjects(250, 125)
	canvas.Title("SVG Go Library Description")
	canvas.Rect(0, 0, width, height, "fill:white;stroke:black;stroke-width:2")
	canvas.Gstyle(gtextstyle)
	canvas.Link("http://github.com/ajstarks/svgo", "SVGo Library")
	canvas.Text(width/2, 150, "SVG Go Library", "font-size:125px")
	canvas.Text(width/2, 200, "github.com/ajstarks/svgo", "font-size:50px;fill:gray")
	canvas.LinkEnd()
	placeobjects(400, 400, 700, 600, roworder)
	canvas.Gend()
	canvas.End()
}
