// compx: display components and connections on a grid, given a XML description
// +build !appengine

package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/ajstarks/svgo"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// Component XML structures
type Component struct {
	Top    int      `xml:"top,attr"`
	Left   int      `xml:"left,attr"`
	Gutter int      `xml:"gutter,attr"`
	Gw     int      `xml:"gw,attr"`
	Gh     int      `xml:"gh,attr"`
	Gc     string   `xml:"gc,attr"`
	Legend []legend `xml:"legend"`
	Note   []note   `xml:"note"`
	Group  []group  `xml:"group"`
	Comp   []comp   `xml:"comp"`
}

type group struct {
	Brow    int     `xml:"brow,attr"`
	Bcol    int     `xml:"bcol,attr"`
	Erow    int     `xml:"erow,attr"`
	Ecol    int     `xml:"ecol,attr"`
	Width   int     `xml:"width,attr"`
	Height  int     `xml:"height,attr"`
	Label   string  `xml:"label,attr"`
	Color   string  `xml:"color,attr"`
	Opacity float64 `xml:"opacity,attr"`
}

type note struct {
	Row     int     `xml:"row,attr"`
	Col     int     `xml:"col,attr"`
	Width   int     `xml:"width,attr"`
	Height  int     `xml:"height,attr"`
	Size    int     `xml:"size,attr"`
	Spacing int     `xml:"spacing,attr"`
	Align   string  `xml:"align,attr"`
	Nitem   []nitem `xml:"nitem"`
}

type legend struct {
	Title  string  `xml:"title,attr"`
	Row    int     `xml:"row,attr"`
	Col    int     `xml:"col,attr"`
	Width  int     `xml:"width,attr"`
	Height int     `xml:"height,attr"`
	Size   int     `xml:"size,attr"`
	Litem  []litem `xml:"litem"`
}

type comp struct {
	Id      string    `xml:"id,attr"`
	Col     int       `xml:"col,attr"`
	Row     int       `xml:"row,attr"`
	Width   int       `xml:"width,attr"`
	Height  int       `xml:"height,attr"`
	Name    string    `xml:"name,attr"`
	Os      string    `xml:"os,attr"`
	Sw      string    `xml:"sw,attr"`
	Color   string    `xml:"color,attr"`
	Shape   string    `xml:"shape,attr"`
	Image   string    `xml:"image,attr"`
	Connect []Connect `xml:"connect"`
}

type litem struct {
	Color string `xml:"color,attr"`
	Type  string `xml:"type,attr"`
	Label string `xml:",chardata"`
}

type nitem struct {
	Color string `xml:"color,attr"`
	Align string `xml:"align,attr"`
	Text  string `xml:",chardata"`
}

// Connect defines connections
type Connect struct {
	Sloc  string `xml:"sloc,attr"`
	Dloc  string `xml:"dloc,attr"`
	Dest  string `xml:"dest,attr"`
	Mark  string `xml:"mark,attr"`
	Color string `xml:"color,attr"`
	Dir   string `xml:"dir,attr"`
	Label string `xml:",chardata"`
}

type gcomp struct {
	x, y, w, h int
}

var (
	width, height, fontscale                             int
	linesize, labelfs, notchsize, groupmargin            int
	showtitle, showtimestamp, roundbox, arc, italiclabel bool
	title, bgcolor, guide                                string
	gridw                                                = 0
	gridh                                                = 0
	globalcolor                                          = "black"
	canvas                                               = svg.New(os.Stdout)
)

const (
	lcolor      = "rgb(190,190,190)"
	boxradius   = 10
	lopacity    = "1.0"
	defcolor    = "black"
	linefmt     = "stroke:%s;fill:none"
	globalstyle = "font-family:Calibri,sans-serif;font-size:%dpx;fill:black;text-anchor:middle;stroke-linecap:round;stroke-width:%dpx;stroke-opacity:%s"
	ltstyle     = "text-anchor:%s;fill:black"
	legendstyle = "text-anchor:start;fill:black;font-size:%dpx"
	gridstyle   = "fill:none; stroke:gray; stroke-opacity:0.3"
	notefmt     = "font-size:%dpx"
	ntfmt       = "text-anchor:%s;fill:%s"
)

func background(fc string) { canvas.Rect(0, 0, width, height, "fill:"+fc) }

// docomp does XML file processing
func docomp(location string) {
	var f *os.File
	var err error
	if len(location) > 0 {
		f, err = os.Open(location)
	} else {
		f = os.Stdin
	}

	if err == nil {
		readcomp(f)
		f.Close()
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

// readcomp reads the XML into the component data structure
func readcomp(r io.Reader) {
	var c Component
	if err := xml.NewDecoder(r).Decode(&c); err == nil {
		drawc(c)
	} else {
		fmt.Fprintf(os.Stderr, "Unable to parse components (%v)\n", err)
	}
}

// drawc interprets the compoment data structure, and displays it
func drawc(c Component) {
	gridw = c.Gw
	gridh = c.Gh
	if len(c.Gc) == 0 {
		globalcolor = c.Gc
	} else {
		globalcolor = c.Gc
	}

	// Groups
	for _, group := range c.Group {
		dogroup(group, c.Top, c.Left, c.Gutter)
	}

	// Legends
	for _, leg := range c.Legend {
		dolegend(leg, c.Top, c.Left, c.Gutter, labelfs, labelfs+4)
	}

	// Notes
	for _, note := range c.Note {
		donote(note, c.Top, c.Left, c.Gutter)
	}

	// Components
	for _, x := range c.Comp {
		for _, y := range x.Connect {
			connect(gc(x, c.Top, c.Left, c.Gutter), y.Sloc,
				lookup(y.Dest, c.Comp, c.Top, c.Left, c.Gutter),
				y.Dloc, y.Label, y.Mark, y.Dir, y.Color)
		}
		display(x, c.Top, c.Left, c.Gutter)
	}

	if len(guide) > 0 {
		grid(c.Top, c.Left, c.Gutter)
	}
	if showtitle {
		dotitle(c.Left, 30, title)
	}
	if showtimestamp {
		timestamp(30)
	}
}

// lookup returns a graphic object given an id
func lookup(id string, c []comp, t, l, g int) gcomp {
	var x gcomp
	for _, v := range c {
		if id == v.Id {
			return gc(v, t, l, g)
		}
	}
	return x
}

// dotitle positions the title relative to the bottom of the drawing
func dotitle(left, offset int, t string) {
	canvas.Text(left, height-offset, t, "font-size:200%;text-anchor:start")
}

// timestamp draws a timestamp in the lower right of the drawing
func timestamp(offset int) {
	t := time.Now()
	canvas.Text(width-offset, height-offset, t.Format(time.ANSIC), "text-anchor:end")
}

// grid displays a grid overlay, useful for determining optimal positioning
func grid(top, left, gutter int) {
	gs := strings.SplitN(guide, `x`, 4)
	if len(gs) != 4 {
		return
	}
	w, _ := strconv.Atoi(gs[0])
	h, _ := strconv.Atoi(gs[1])
	nr, _ := strconv.Atoi(gs[2])
	nc, _ := strconv.Atoi(gs[3])
	canvas.Gstyle(gridstyle)
	y := top
	for r := 0; r < nr; r++ {
		x := left
		for c := 0; c < nc; c++ {
			canvas.Rect(x, y, w, h)
			canvas.Text(x+w/2, y+h/2, fmt.Sprintf("%d,%d", r, c),
				"font-size:150%;fill:lightgray;stroke:none")
			x += w + gutter
		}
		y += h + gutter
	}
	canvas.Gend()
}

// dogroup displays a colored rectangular area
func dogroup(g group, top, left, gutter int) {
	margin := groupmargin
	bx := colx(g.Bcol, g.Width, gutter, left)
	by := rowy(g.Brow, g.Height, gutter, top)
	ex := colx(g.Ecol, g.Width, gutter, left)
	ey := rowy(g.Erow, g.Height, gutter, top)
	gw := (ex + g.Width) - bx
	gh := (ey + g.Height) - by
	var gop float64
	if g.Opacity <= 0 {
		gop = 1.0
	} else {
		gop = g.Opacity
	}
	canvas.Rect(bx-margin, by-margin, gw+margin*2, gh+margin*2,
		fmt.Sprintf("fill-opacity:%.2f;fill:%s", gop, g.Color))
	if len(g.Label) > 0 {
		canvas.Text(bx+gw/2, by+gh/2, g.Label, "fill:gray")
	}
}

// dolegend displays the legend
func dolegend(leg legend, top, left, gutter, fs, ls int) {
	if leg.Size > 0 {
		fs = leg.Size
		ls = fs + 4
	}
	fsh := fs / 2
	x := colx(leg.Col, leg.Width, gutter, left)
	y := rowy(leg.Row, leg.Height, gutter, top)

	canvas.Gstyle(fmt.Sprintf(legendstyle, fs))
	for _, v := range leg.Litem {
		if v.Type == "line" {
			canvas.Rect(x, y+fs/4, fs, fs/4, "fill:"+v.Color)
		} else {
			canvas.Square(x, y, fs, "fill:"+v.Color)
		}
		canvas.Text(x+(fs*2), y+fsh, v.Label, "baseline-shift:-30%")
		y += ls
	}
	canvas.Gend()
}

// donote displays a note
func donote(n note, top, left, gutter int) {
	var align, color string

	size := n.Size
	ls := n.Spacing
	x := colx(n.Col, n.Width, gutter, left)
	y := rowy(n.Row, n.Height, gutter, top)
	if n.Align == "middle" {
		y += n.Height / 2
	}
	if size <= 0 {
		size = labelfs
	}
	if ls == 0 {
		ls = size + 2
	}
	xp := x
	canvas.Gstyle(fmt.Sprintf(notefmt, size))
	for _, v := range n.Nitem {
		switch v.Align {
		case "left", "start", "begin":
			align = "start"
			xp = x
		case "right", "end":
			align = "end"
			xp = x + n.Width
		case "middle", "mid", "center":
			align = "middle"
			xp = x + (n.Width / 2)
		default:
			align = "start"
			xp = x
		}
		if len(v.Color) == 0 {
			color = "black"
		} else {
			color = v.Color
		}
		canvas.Text(xp, y, v.Text, fmt.Sprintf(ntfmt, align, color))
		y += ls
	}
	canvas.Gend()
}

// gc computes the components coordinates
func gc(c comp, top, left, gutter int) gcomp {
	var g gcomp

	// the object and grid dimensions equal, unless explicitly overridden

	if gridw > 0 && gridh > 0 {
		g.x = colx(c.Col, gridw, gutter, left)
		g.y = rowy(c.Row, gridh, gutter, top)
	} else {
		g.x = colx(c.Col, c.Width, gutter, left)
		g.y = rowy(c.Row, c.Height, gutter, top)
	}
	g.w = c.Width
	g.h = c.Height
	return g
}

// display a component in the context of the grid
func display(c comp, top, left, gutter int) {
	g := gc(c, top, left, gutter)
	component(g, c)
}

// component positions and draws a components and its attributes
func component(g gcomp, c comp) {
	x := g.x
	y := g.y
	w := g.w
	h := g.h
	fs := w / fontscale
	fs2 := fs / 2
	w2 := w / 2
	h3 := h / 3

	var boxcolor string
	if len(c.Color) == 0 {
		boxcolor = globalcolor
	} else {
		boxcolor = c.Color
	}
	rectstyle := fmt.Sprintf("stroke:%s;stroke-width:1;fill:%s", boxcolor, boxcolor)

	if len(c.Image) > 0 {
		canvas.Image(x, y, w, h, c.Image)
		if len(c.Name) > 0 {
			canvas.Text(x+w2, y+h/3, c.Name,
				fmt.Sprintf("font-size:%dpx;fill:%s;baseline-shift:50%%", fs, boxcolor))
		}
		return
	}

	if strings.HasPrefix(c.Shape, "#") {
		uselibrary(x, y, w, h, c.Shape)
		return
	}

	switch c.Shape {

	case "mobile", "screen":
		screen(x, y, w, h, 10, boxcolor, bgcolor)
		canvas.Gstyle(fmt.Sprintf("font-size:%dpx", fs))
		canvas.Text(x+w/2, y+h/3, c.Name)
		if len(c.Os) > 0 {
			canvas.Text(x+w/2, y+h3+fs+2, c.Os, "font-size:60%")
		}
		if len(c.Sw) > 0 {
			canvas.Text(x+w/2, y+h3+fs*2, c.Sw, "font-size:75%")
		}
		canvas.Gend()

	case "server":
		server(x, y, w, h, 10, boxcolor, lcolor)
		canvas.Text(x+w/2, y+h-10, c.Name, fmt.Sprintf("font-size:%dpx;fill:white", fs))

	case "desktop":
		l := h / 20
		desktop(x, y, w, h, l, boxcolor, bgcolor)
		canvas.Gstyle(fmt.Sprintf("font-size:%dpx", fs))
		canvas.Text(x+w/2, y+h/3, c.Name)
		if len(c.Os) > 0 {
			canvas.Text(x+w/2, y+h3+fs+2, c.Os, "font-size:60%")
		}
		if len(c.Sw) > 0 {
			canvas.Text(x+w/2, y+h3+fs*2, c.Sw, "font-size:75%")
		}
		canvas.Gend()

	case "message":
		l := h / 20
		pmy := h / 8
		message(x, y, w, h, l, lcolor, boxcolor)
		canvas.Gstyle(fmt.Sprintf("font-size:%dpx;fill:white", fs))
		if len(c.Os) > 0 {
			canvas.Text(x+w2, y+pmy+fs, c.Os, "font-size:75%")
		}
		canvas.Text(x+w2, y+h-10, c.Name, fmt.Sprintf("font-size:%dpx;fill:%s", fs, bgcolor))
		canvas.Gend()

	case "cloud":
		r := w / 3
		xc := (x + w/2) + r/4
		yc := y + h/2
		cloud(xc, yc, r, boxcolor)
		canvas.Text(xc-(r/4), yc+r/2, c.Name, fmt.Sprintf("font-size:%dpx;fill:white", fs))

	case "db":
		cylinder(x, y+(h/4), w, h-(h/2), h/4, lcolor, boxcolor)
		canvas.Text(x+w2, y+h3, c.Name, fmt.Sprintf("font-size:%dpx;fill:white", fs))
		if len(c.Sw) > 0 {
			canvas.Text(x+w2, y+2*h3, c.Sw, "font-size:75%")
		}

	case "folder":
		l := w / 10
		folder(x, y, w, h, l, boxcolor, lcolor)
		yp := y + h/2
		canvas.Gstyle(fmt.Sprintf("font-size:%dpx;fill:white", fs))
		canvas.Text(x+w/2, yp, c.Name)
		if len(c.Os) > 0 {
			canvas.Text(x+w/2, yp+fs+2, c.Os, "font-size:60%")
		}
		if len(c.Sw) > 0 {
			canvas.Text(x+w/2, yp+fs*2, c.Sw, "font-size:75%")
		}
		canvas.Gend()

	case "face":
		fr := (w / 4) - (fs / 2) // (h*3)/8
		face(x+w/2, y+h/2, fr, linesize, boxcolor, bgcolor)
		canvas.Text(x+w/2, y+(h/2)+fr+15, c.Name, fmt.Sprintf("font-size:%dpx;fill:black", fs))

	case "role":
		role(x, y, w, h, boxcolor)
		canvas.Text(x+w/2, y+h-5, c.Name, fmt.Sprintf("font-size:%dpx;fill:%s", fs, bgcolor))

	case "eaec":
		l := w / 10
		eaec(x, y, w, h, l, boxcolor, bgcolor)
		canvas.Text(x+w/2, y+h-5, c.Name, fmt.Sprintf("font-size:%dpx;fill:%s", fs, bgcolor))

	case "plain":
		if roundbox {
			canvas.Roundrect(x, y, w, h, fs2, fs2, rectstyle)
		} else {
			canvas.Rect(x, y, w, h, rectstyle)
		}
		canvas.Text(x+w2, y+(h/2), c.Name, fmt.Sprintf("font-size:%dpx;fill:white;baseline-shift:-25%%", fs))

	default:
		canvas.Rect(x, y, w, h3, rectstyle)
		canvas.Rect(x, y+h3, w, h-h3, "fill:white;stroke-width:1;stroke:gray")
		canvas.Gstyle(fmt.Sprintf("font-size:%dpx", fs))
		canvas.Text(x+w2, y+h3, c.Name, "fill:white;baseline-shift:50%")
		canvas.Text(x+w2, y+h3+fs2, c.Os, "font-size:60%")
		wordstack(x+w2, (y+h)-fs2, fs, strings.Split(c.Sw, `\n`), "fill-opacity:0.75;font-size:75%")
		canvas.Gend()
	}
}

// uselibrary draws a previously defined object
func uselibrary(x, y, w, h int, name string) {
	canvas.Use(x, y, name, fmt.Sprintf(`width="%d"`, w), fmt.Sprintf(`height="%d"`, h))
}

// Object functions
func cylinder(x, y, w, h, eh int, fill, tfill string) {
	f := "fill:" + fill
	tf := "fill:" + tfill
	canvas.Rect(x, y, w, h, f)
	canvas.Ellipse(x+w/2, y+h, w/2, eh, f)
	canvas.Ellipse(x+w/2, y, w/2, eh, tf)
}

// folder object
func folder(x, y, w, h, l int, bcolor, color string) {
	nl := w / 10
	xl := x + nl
	xw := x + w
	yl := y + nl
	yh := y + h
	l2 := nl * 2
	l3 := nl * 3
	lh := nl / 2
	var (
		xn = []int{xl, xl + l2, xl + l3, xw, xw, xl}
		yn = []int{y, y, y + lh, y + lh, yh, yh}
		xf = []int{xw, xw - l, x, x + l}
		yf = []int{yh, yl + lh, yl + lh, yh}
	)
	canvas.Polygon(xn, yn, "fill:"+color)
	canvas.Polygon(xf, yf, "fill:"+bcolor)
}

// cloud object
func cloud(x, y, r int, style string) {
	small := r / 2
	medium := (r * 6) / 10
	canvas.Gstyle("fill:" + style)
	canvas.Circle(x, y, r)
	canvas.Circle(x+r, y+small, small)
	canvas.Circle(x-r-small, y+small, small)
	canvas.Circle(x-r, y, medium)
	canvas.Rect(x-r-small, y, r*2+small, r)
	canvas.Gend()
}

// message object
func message(x, y, w, h, l int, bcolor, scolor string) {
	et := h / 3
	w2 := w / 2
	px := w / 8
	py := h / 8
	e1x := []int{x, x, x + w, x + w, x + w2, x}
	e1y := []int{y + et, y + h, y + h, y + et, y + (et * 2), y + et}
	e2x := []int{x, x + w2, x + w, x + w2, x}
	e2y := []int{y + et, y, y + et, y + (et * 2), y + et}

	canvas.Polygon(e2x, e2y, "fill:"+bcolor)
	canvas.Polygon(e1x, e1y, "fill:"+scolor)
	canvas.Roundrect(x+px, y+py, w-(px*2), h-py, l, l, "fill:"+scolor)
	canvas.Line(x, y+et, x+w2, y+(et*2), "stroke-width:1;stroke:"+bcolor)
	canvas.Line(x+w, y+et, x+w2, y+(et*2), "stroke-width:1;stroke:"+bcolor)
}

// eaec person object
func eaec(x, y, w, h, l int, scolor, bcolor string) {
	wu := w / 8
	hu := h / 12
	wh := w / 2
	hh := h / 2
	sx := []int{x + wu*2, x + wu*6, x + wh}
	sy := []int{y + hu*6, y + hu*6, y + hu*11}
	tx := []int{x + wh, x + wh + wu, x + wh, x + wh - wu, x + wh}
	ty := []int{y + hu*6, y + hu*7, y + hu*10, y + hu*7, y + hu*6}

	canvas.Ellipse(x+wh, y+hu*4, wu+wu/2, hu*2, "fill:"+bcolor)
	canvas.Roundrect(x+wu, y+hh, w-wu*2, hu*6, l, l, "fill:"+bcolor)
	canvas.Polygon(sx, sy, "fill:"+scolor)
	canvas.Polygon(tx, ty, "fill:"+bcolor)
}

// screen object
func screen(x, y, w, h, l int, bcolor, color string) {
	canvas.Roundrect(x, y, w, h, l, l, "fill:"+bcolor)
	canvas.Rect(x+l, y+l, w-(l*2), h-(l*2), "fill:"+color)
}

// kb (keyboard) object
func kb(x, y, w, h, l int, color string) {
	var xp = []int{x + l, x, x + w, x + w - l}
	var yp = []int{y, y + h, y + h, y}
	canvas.Polygon(xp, yp, "fill:"+color)
}

// desktop object
func desktop(x, y, w, h, l int, bcolor, color string) {
	screen(x, y, w, h-l*3, l, bcolor, color)
	kb(x, y+h-(l*2), w+l, l*2, l*2, bcolor)
}

// face object
func face(x, y, r, l int, color, fcolor string) {
	fu := r / 10 // "face unit"
	ep := 3 * fu
	my := y + ep
	canvas.Circle(x, y, r, fmt.Sprintf("fill:%s;stroke-width:%dpx;stroke:%s", fcolor, l, color))
	canvas.Circle(x+ep, y-ep, r/10, "fill:"+color)
	canvas.Circle(x-ep, y-ep, r/10, "fill:"+color)
	canvas.Qbez(x+ep, my, x, y+ep*2, x-ep, my, fmt.Sprintf("fill:%s;stroke-width:%dpx;stroke:%s", color, l, color))
}

// server object
func server(x, y, w, h, l int, bcolor, color string) {
	var xp = []int{x + (l * 2), (x + w) - (l * 2), (x + w) - l, x + l}
	var yp = []int{y, y, y + l, y + l}
	canvas.Polygon(xp, yp, "fill:"+color)
	canvas.Roundrect(x, y+l, w, h-l, 5, 5, "fill:"+bcolor)
	canvas.Gstyle("stroke:" + color)
	yl := y + (l / 2) + h/4
	spacing := w / 5
	for r := 0; r < 2; r++ {
		xl := x + l
		for c := 0; c < 2; c++ {
			canvas.Line(xl, yl, xl+spacing, yl)
			xl += spacing + 10
		}
		yl += h / 4
	}
	canvas.Gend()
	canvas.Circle((x+w)-l, y+h/2, l/2, "fill:"+color)
}

// role object
func role(x, y, w, h int, color string) {
	hs := h / 20
	var xp = []int{x, x, x + w/3, x + w/2, x + (w / 2) + (w / 6), x + w, x + w}
	var yp = []int{y + h, y + (16 * hs), y + (12 * hs), y + (14 * hs), y + (12 * hs), y + (16 * hs), y + h}

	// var xp = []int{x, x, x + w/2, x + w, x + w}
	// var yp = []int{y + h, y + (h5 * 4), y + (h5 * 2), y + (h5 * 4), y + h}
	canvas.Gstyle("fill:" + color)
	canvas.Polygon(xp, yp)
	canvas.Ellipse(x+w/2, (y + h/3), w/5, h/3) //  "stroke:white;stroke-width:2")
	canvas.Gend()
}

// sloper computes the slope and r of a line
func sloper(x1, y1, x2, y2 int) (m, r float64) {
	dy := float64(y1 - y2)
	dx := float64(x1 - x2)
	m = dy / dx
	r = math.Atan2(dy, dx) * (180 / math.Pi)
	return m, r
}

// rowy computes the y position of a row
func rowy(n, h, g, t int) int { return t + (n * g) + (n * h) }

// colx computes the x position of a column
func colx(n, w, g, l int) int { return l + (n * g) + (n * w) }

// compass returns the coordinates of a compass point
func compass(g gcomp, point string) (cx, cy int, dir string) {
	switch point {
	case "nw":
		cx, cy, dir = g.x, g.y, "r"
	case "nnw":
		cx, cy, dir = g.x+g.w/4, g.y, "d"
	case "nne":
		cx, cy, dir = (g.x+g.w)-(g.w/4), g.y, "d"
	case "n":
		cx, cy, dir = g.x+(g.w/2), g.y, "d"
	case "ne":
		cx, cy, dir = g.x+g.w, g.y, "l"
	case "w":
		cx, cy, dir = g.x, g.y+g.h/2, "r"
	case "wnw":
		cx, cy, dir = g.x, g.y+g.h/4, "r"
	case "wsw":
		cx, cy, dir = g.x, (g.y+g.h)-(g.h/4), "r"
	case "ese":
		cx, cy, dir = g.x+g.w, (g.y+g.h)-(g.h/4), "l"
	case "ene":
		cx, cy, dir = g.x+g.w, g.y+(g.h/4), "l"
	case "c":
		cx, cy, dir = g.x+(g.w/2), g.y+(g.h/2), "n"
	case "e":
		cx, cy, dir = g.x+g.w, g.y+(g.h/2), "l"
	case "sw":
		cx, cy, dir = g.x, g.y+g.h, "r"
	case "ssw":
		cx, cy, dir = g.x+(g.w/4), g.y+g.h, "u"
	case "sse":
		cx, cy, dir = (g.x+g.w)-(g.w/4), g.y+g.h, "u"
	case "s":
		cx, cy, dir = g.x+(g.w/2), g.y+g.h, "u"
	case "se":
		cx, cy, dir = g.x+g.w, g.y+g.h, "l"
	}
	return cx, cy, dir
}

// connect two components
func connect(c1 gcomp, p1 string, c2 gcomp, p2 string, label string, mark string, dir string, color string) {
	x1, y1, d1 := compass(c1, p1)
	x2, y2, d2 := compass(c2, p2)
	linelabel(x1, y1, x2, y2, label, mark, d1, d2, dir, color)
}

// linestyle returns the style for lines
func linestyle(color string) string {
	return fmt.Sprintf(linefmt, color)
}

// linelabel determines the connection and arrow geometry
func linelabel(x1, y1, x2, y2 int, label string, mark string, d1 string, d2 string, dir string, color string) {
	aw := linesize * 4
	ah := linesize * 3

	if len(color) == 0 {
		color = lcolor
	}
	switch mark {
	case "b":
		lx1, ly1 := arrow(x1, y1, aw, ah, d1, color)
		lx2, ly2 := arrow(x2, y2, aw, ah, d2, color)
		doline(lx1, ly1, lx2, ly2, linestyle(color), dir, label)

	case "s":
		lx1, ly1 := arrow(x1, y1, aw, ah, d1, color)
		doline(lx1, ly1, x2, y2, linestyle(color), dir, label)

	case "d":
		lx2, ly2 := arrow(x2, y2, aw, ah, d2, color)
		doline(x1, y1, lx2, ly2, linestyle(color), dir, label)

	default:
		doline(x1, y1, x2, y2, linestyle(color), dir, label)
	}
}

// doline draws a line between to coordinates
func doline(x1, y1, x2, y2 int, style, direction, label string) {
	var labelstyle string
	var upflag bool

	if italiclabel {
		labelstyle = "font-style:italic;"
	}

	tadjust := 6
	mx := (x2 - x1) / 2
	my := (y2 - y1) / 2
	lx := x1 + mx
	ly := y1 + my
	m, _ := sloper(x1, y1, x2, y2)
	hline := m == 0
	vline := m == math.Inf(-1) || m == math.Inf(1)
	straight := hline || vline

	switch {
	case m < 0: // upwards line
		upflag = true
		labelstyle += "text-anchor:end;"
		lx -= tadjust
	case hline: // horizontal line
		labelstyle += "text-anchor:middle;baseline-shift:20%;"
		ly -= tadjust
	case m > 0: // downwards line
		upflag = false
		labelstyle += "text-anchor:start;"
		lx += tadjust
	}
	if arc && !straight {
		cx, cy := x1, y2 // initial control points
		// fmt.Fprintf(os.Stderr, "%s slope = %.3f\n", label, m)
		if upflag {
			if direction == "ccw" {
				cx, cy = x2, y1
			} else {
				cx, cy = x1, y2
			}
		} else {
			if direction == "ccw" {
				cx, cy = x1, y2
			} else {
				cx, cy = x2, y1
			}
		}
		canvas.Qbez(x1, y1, cx, cy, x2, y2, style)
		labelstyle += "text-anchor:middle"
		canvas.Text(lx, ly, label, labelstyle)
	} else {
		canvas.Line(x1, y1, x2, y2, style)
		canvas.Text(lx, ly, label, labelstyle) // midpoint
	}
}

// wordstack displays text in a left-justified stack
func wordstack(x, y, fs int, s []string, style ...string) {
	ls := fs + 2
	for i := len(s); i > 0; i-- {
		canvas.Text(x, y, s[i-1], style...)
		y -= ls
	}
}

// arrow constructs line-ending arrows according to connecting points
func arrow(x, y, w, h int, dir string, color string) (xl, yl int) {
	var xp = []int{x, x, x, x}
	var yp = []int{y, y, y, y}

	n := notchsize
	switch dir {
	case "r":
		xp[1] = x - w
		yp[1] = y - h/2
		xp[2] = (x - w) + n
		yp[2] = y
		xp[3] = x - w
		yp[3] = y + h/2
		xl, yl = xp[2], y
	case "l":
		xp[1] = x + w
		yp[1] = y - h/2
		xp[2] = (x + w) - n
		yp[2] = y
		xp[3] = x + w
		yp[3] = y + h/2
		xl, yl = xp[2], y
	case "u":
		xp[1] = x - w/2
		yp[1] = y + h
		xp[2] = x
		yp[2] = (y + h) - n
		xp[3] = x + w/2
		yp[3] = y + h
		xl, yl = x, yp[2]
	case "d":
		xp[1] = x - w/2
		yp[1] = y - h
		xp[2] = x
		yp[2] = (y - h) + n
		xp[3] = x + w/2
		yp[3] = y - h
		xl, yl = x, yp[2]
	}
	canvas.Polygon(xp, yp, "fill:"+color+";fill-opacity:"+lopacity)
	return xl, yl
}

// init processes command line arguments
func init() {
	flag.IntVar(&width, "w", 1024, "width")
	flag.IntVar(&height, "h", 768, "height")
	flag.IntVar(&linesize, "l", 4, "line weight")
	flag.IntVar(&labelfs, "lf", 16, "label font size (px)")
	flag.IntVar(&fontscale, "f", 10, "font scaling factor")
	flag.IntVar(&notchsize, "n", 0, "arrow notch size")
	flag.IntVar(&groupmargin, "gm", 10, "group margin")
	flag.StringVar(&bgcolor, "bg", "white", "background color")
	flag.StringVar(&title, "t", "comp grid", "title")
	flag.BoolVar(&italiclabel, "il", false, "italic labels")
	flag.BoolVar(&showtitle, "showtitle", false, "Show the title")
	flag.BoolVar(&showtimestamp, "time", false, "Show a timestamp")
	flag.BoolVar(&roundbox, "roundbox", false, "make boxes round")
	flag.BoolVar(&arc, "arc", false, "use arcs to connect")
	flag.StringVar(&guide, "g", "", "grid guide: WxHxRxC")
	flag.Parse()
}

// for every file (or stdin) make a component diagram
func main() {
	canvas.Start(width, height)
	canvas.Title(title)
	background(bgcolor)
	canvas.Gstyle(fmt.Sprintf(globalstyle, labelfs, linesize, lopacity))

	if len(flag.Args()) == 0 {
		docomp("")
	} else {
		for _, f := range flag.Args() {
			docomp(f)
		}
	}
	canvas.Gend()
	canvas.End()
}
