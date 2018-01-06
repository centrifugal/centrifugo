// radar roadmap (via Ernst and Young)
package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/ajstarks/svgo"
	"io"
	"math"
	"os"
	"strings"
)

var (
	width, height, iscale, fontsize, margin int
	bgcolor, itemcolor, title               string
	opacity                                 float64
	showtitle                               bool
	gstyle                                  = "font-family:Calibri;font-size:%dpx;text-anchor:middle"
	canvas                                  = svg.New(os.Stdout)
)

// Roadmap XML structure:
// a roadmap consists of sections, which contain items, which indicate maturity and impact
type Roadmap struct {
	Title    string    `xml:"title,attr"`
	Duration int       `xml:"duration,attr"`
	Unit     string    `xml:"unit,attr"`
	Section  []section `xml:"section"`
}

type section struct {
	Name    string  `xml:"name,attr"`
	Spacing float64 `xml:"spacing,attr"`
	Item    []item  `xml:"item"`
}

type item struct {
	Impact int     `xml:"impact,attr"`
	Effort int     `xml:"effort,attr"`
	Begin  string  `xml:"begin,attr"`
	Age    float64 `xml:"age,attr"`
	Name   string  `xml:",chardata"`
	Desc   desc    `xml:"desc"`
}

type desc struct {
	Description string `xml:",chardata"`
}

// dorr does file i/o
func dorr(location string) {
	var f *os.File
	var err error
	if len(location) > 0 {
		f, err = os.Open(location)
	} else {
		f = os.Stdin
	}
	if err == nil {
		readrr(f)
		f.Close()
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

// readrr reads and parses the XML specification
func readrr(r io.Reader) {
	var rm Roadmap
	if err := xml.NewDecoder(r).Decode(&rm); err == nil {
		drawrr(rm)
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

// drawrr draws the roadmap
func drawrr(rm Roadmap) {

	if len(rm.Title) > 0 {
		title = rm.Title
	}
	canvas.Title(title)
	canvas.Gstyle(fmt.Sprintf(gstyle, fontsize))
	canvas.Rect(0, 0, width, height, "fill:"+bgcolor)
	duration := rm.Duration
	if duration <= 0 {
		duration = 3
	}
	ns := len(rm.Section)
	cx := (width / 2)
	cy := (height / 2)
	r := ((width - margin) / duration) / 2
	sr := r
	midsize := width / 100

	// for each unit of time, draw cencentric circles
	for i := 0; i < duration; i++ {
		canvas.Circle(cx, cy, sr, "fill:none;stroke:lightgray;stroke-dasharray:7,7")
		canvas.Text(cx, (cy - sr), fmt.Sprintf("%s %d", rm.Unit, i+1), "font-size:150%;fill:gray")
		sr += r
	}
	// for each section, define its boundaries and draw its label
	angle := 360.0 / float64(ns)
	a := angle
	a2 := a / 2
	for _, s := range rm.Section {
		drawseclines(cx, cy, float64(r*duration), a, a2, s.Name)
		spacing := s.Spacing
		if spacing == 0 {
			spacing = (angle / float64(len(s.Item))) - 1
		}
		iangle := a + spacing
		// for each item in the section, place the marker and label
		for _, i := range s.Item {
			itemx, itemy := dpolar(cx, cy, i.Age*float64(r), iangle)
			drawitem(itemx, itemy, i.Impact*iscale, i.Effort, i.Name)
			iangle += spacing
		}
		a += angle
	}

	canvas.Circle(cx, cy, midsize, "fill:red")
	canvas.Text(cx, cy, "READY", "baseline-shift:-25%")
	if showtitle {
		dotitle(title)
	}
	canvas.Gend()
}

// radians converts degrees to radians
func radians(d float64) float64 {
	return d * (math.Pi / 180.0)
}

// dotitle places the title text
func dotitle(s string) {
	canvas.Text(width/2, height-10, s, "font-size:200%;text-anchor:middle")
}

// dpolar returns the cartesion coordinates given the center, size, and angle (in degrees)
func dpolar(cx, cy int, r, d float64) (int, int) {
	x := r * math.Cos(radians(d))
	y := r * math.Sin(radians(d))
	return cx + int(x), cy + int(y)
}

// drawseclines defines and labels the sections
func drawseclines(cx, cy int, size, a, h float64, s string) {
	fs := fontsize + (fontsize / 2)
	ix, iy := dpolar(cx, cy, size, a)
	ix2, iy2 := dpolar(cx, cy, size+50, a+h)
	canvas.Line(cx, cy, ix, iy, "stroke:lightgray")
	textlines(ix2, iy2, fs, fs+2, "middle", "black", strings.Split(s, "\\n"))
}

// drawitem draws a roadmap item
func drawitem(x, y, isize, ieffort int, s string) {
	var op float64
	if ieffort > 0 {
		op = opacity * (float64(ieffort) / 10.0)
	} else {
		op = opacity
	}
	style := fmt.Sprintf("fill:%s;fill-opacity:%.2f;stroke:white", itemcolor, op)
	canvas.Circle(x, y, isize/2, style)
	textlines(x-(isize/2)-2, y, fontsize, fontsize+2, "end", "black", strings.Split(s, "\\n"))
}

// textlines displays text at a specified size, leading, fill, and alignment
func textlines(x, y, fs, leading int, align, fill string, s []string) {
	canvas.Gstyle(fmt.Sprintf("font-size:%dpx;text-anchor:%s;fill:%s", fs, align, fill))
	for _, v := range s {
		canvas.Text(x, y, v)
		y += leading
	}
	canvas.Gend()
}

// init sets up the command flags
func init() {
	flag.StringVar(&bgcolor, "bg", "white", "background color")
	flag.StringVar(&itemcolor, "ic", "rgb(131,206,226)", "item color")
	flag.IntVar(&width, "w", 800, "width")
	flag.IntVar(&height, "h", 800, "height")
	flag.IntVar(&fontsize, "f", 12, "fontsize (px)")
	flag.IntVar(&iscale, "s", int(float64(width)*.009), "impact scale")
	flag.IntVar(&margin, "m", 150, "outside margin")
	flag.BoolVar(&showtitle, "showtitle", false, "show title")
	flag.StringVar(&title, "t", "Roadmap", "title")
	flag.Float64Var(&opacity, "o", 1.0, "opacity")
	flag.Parse()
}

// for every input file (or stdin), draw a roadmap as specified by command flags
func main() {
	canvas.Start(width, height)
	if len(flag.Args()) == 0 {
		dorr("")
	} else {
		for _, f := range flag.Args() {
			dorr(f)
		}
	}
	canvas.End()
}
