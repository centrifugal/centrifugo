// benchviz: visualize benchmark data from benchcmp
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/ajstarks/svgo"
)

// geometry defines the layout of the visualization
type geometry struct {
	top, left, width, height, vwidth, vp, barHeight int
	dolines, coldata                                bool
	title, rcolor, scolor, style                    string
	deltamax, speedupmax                            float64
}

// process reads the input and calls the visualization function
func process(canvas *svg.SVG, filename string, g geometry) int {
	if filename == "" {
		return g.visualize(canvas, filename, os.Stdin)
	}
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 0
	}
	defer f.Close()
	return g.visualize(canvas, filename, f)
}

// vmap maps world to canvas coordinates
func vmap(value, low1, high1, low2, high2 float64) float64 {
	return low2 + (high2-low2)*(value-low1)/(high1-low1)
}

// visualize performs the visualization of the input, reading a line a time
func (g *geometry) visualize(canvas *svg.SVG, filename string, f io.Reader) int {
	var (
		err               error
		line, vs, bmtitle string
		dmin, dmax        float64
	)

	bh := g.barHeight
	vizwidth := g.vwidth
	vspacing := g.barHeight + (g.barHeight / 3) // vertical spacing
	bmtype := "delta"

	in := bufio.NewReader(f)
	canvas.Gstyle(fmt.Sprintf("font-size:%dpx;font-family:sans-serif", bh))
	if g.title == "" {
		bmtitle = filename
	} else {
		bmtitle = g.title
	}
	canvas.Text(g.left, g.top, bmtitle, "font-size:150%")

	height := 0
	for x, y, nr := g.left+g.vp, g.top+vspacing, 0; err == nil; nr++ {
		line, err = in.ReadString('\n')
		fields := strings.Split(strings.TrimSpace(line), ` `)

		if len(fields) <= 1 || len(line) < 2 {
			continue
		}
		name := fields[0]
		value := fields[len(fields)-1]
		if len(value) > 2 {
			vs = value[:len(value)-1]
		}
		v, _ := strconv.ParseFloat(vs, 64)
		av := math.Abs(v)

		switch {
		case strings.HasPrefix(value, "delt"):
			bmtype = "delta"
			dmin = 0.0
			dmax = g.deltamax // 100.0
			y += vspacing * 2
			continue

		case strings.HasPrefix(value, "speed"):
			bmtype = "speedup"
			dmin = 0.0
			dmax = g.speedupmax // 10.0
			y += vspacing * 2
			continue

		case strings.HasPrefix(name, "#"):
			y += vspacing
			canvas.Text(g.left, y, line[1:], "font-style:italic;fill:gray")
			continue
		}

		bw := int(vmap(av, dmin, dmax, 0, float64(vizwidth)))
		switch g.style {
		case "bar":
			g.bars(canvas, x, y, bw, bh, vspacing/2, bmtype, name, value, v)
		case "inline":
			g.inline(canvas, g.left, y, bw, bh, bmtype, name, value, v)
		default:
			g.bars(canvas, x, y, bw, bh, vspacing/2, bmtype, name, value, v)
		}
		y += vspacing
		height = y
	}
	canvas.Gend()
	return height
}

// inline makes the inline style pf visualization
func (g *geometry) inline(canvas *svg.SVG, x, y, w, h int, bmtype, name, value string, v float64) {
	var color string
	switch bmtype {
	case "delta":
		if v > 0 {
			color = g.rcolor
		} else {
			color = g.scolor
		}
	case "speedup":
		if v < 1.0 {
			color = g.rcolor
		} else {
			color = g.scolor
		}
	}
	canvas.Text(x-10, y, value, "text-anchor:end")
	canvas.Text(x, y, name)
	canvas.Rect(x, y-h, w, h, "fill-opacity:0.3;fill:"+color)
}

// bars creates barchart style visualization
func (g *geometry) bars(canvas *svg.SVG, x, y, w, h, vs int, bmtype, name, value string, v float64) {
	canvas.Gstyle("font-style:italic;font-size:75%")
	toffset := h / 4
	var tx int
	var tstyle string
	switch bmtype {
	case "delta":
		if v > 0 {
			canvas.Rect(x-w, y-h/2, w, h, "fill-opacity:0.3;fill:"+g.rcolor)
			tx = x - w - toffset
			tstyle = "text-anchor:end"
		} else {
			canvas.Rect(x, y-h/2, w, h, "fill-opacity:0.3;fill:"+g.scolor)
			tx = x + w + toffset
			tstyle = "text-anchor:start"
		}
	case "speedup":
		if v < 1.0 {
			canvas.Rect(x-w, y-h/2, w, h, "fill-opacity:0.3;fill:"+g.rcolor)
			tx = x - w - toffset
			tstyle = "text-anchor:end"
		} else {
			canvas.Rect(x, y-h/2, w, h, "fill-opacity:0.3;fill:"+g.scolor)
			tx = x + w + toffset
			tstyle = "text-anchor:start"
		}
	}
	if g.coldata {
		canvas.Text(x-toffset, y+toffset, value, "text-anchor:end")
	} else {
		canvas.Text(tx, y+toffset, value, tstyle)
	}
	canvas.Gend()
	canvas.Text(g.left, y+(h/2), name, "text-anchor:start")
	if g.dolines {
		canvas.Line(g.left, y+vs, g.left+(g.width-g.left), y+vs, "stroke:lightgray;stroke-width:1")
	}
}

func main() {
	var (
		width        = flag.Int("w", 1024, "width")
		top          = flag.Int("top", 50, "top")
		left         = flag.Int("left", 100, "left margin")
		vp           = flag.Int("vp", 512, "visualization point")
		vw           = flag.Int("vw", 300, "visual area width")
		bh           = flag.Int("bh", 20, "bar height")
		smax         = flag.Float64("sm", 10, "maximum speedup")
		dmax         = flag.Float64("dm", 100, "maximum delta")
		title        = flag.String("title", "", "title")
		speedcolor   = flag.String("scolor", "green", "speedup color")
		regresscolor = flag.String("rcolor", "red", "regression color")
		style        = flag.String("style", "bar", "set the style (bar or inline)")
		lines        = flag.Bool("line", false, "show lines between entries")
		coldata      = flag.Bool("col", false, "show data in a single column")
	)
	flag.Parse()

	g := geometry{
		width:      *width,
		top:        *top,
		left:       *left,
		vp:         *vp,
		vwidth:     *vw,
		barHeight:  *bh,
		title:      *title,
		scolor:     *speedcolor,
		rcolor:     *regresscolor,
		style:      *style,
		dolines:    *lines,
		coldata:    *coldata,
		speedupmax: *smax,
		deltamax:   *dmax,
	}

	// For every named file or stdin, render the SVG in memory, accumulating the height.
	var b bytes.Buffer
	canvas := svg.New(&b)
	height := 0
	if len(flag.Args()) > 0 {
		for _, f := range flag.Args() {
			height = process(canvas, f, g)
			g.top = height + 50
		}
	} else {
		height = process(canvas, "", g)
	}
	g.height = height + 15

	// Write the rendered SVG to stdout
	out := svg.New(os.Stdout)
	out.Start(g.width, g.height)
	out.Rect(0, 0, g.width, g.height, "fill:white;stroke-width:2px;stroke:lightgray")
	b.WriteTo(os.Stdout)
	out.End()
}
