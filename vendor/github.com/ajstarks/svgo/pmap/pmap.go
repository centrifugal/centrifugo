// pmap percentage maps
// +build !appengine

package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ajstarks/svgo"
)

// Pmap defines a porportional map
type Pmap struct {
	Top   int     `xml:"top,attr"`
	Left  int     `xml:"left,attr"`
	Title string  `xml:"title,attr"`
	Pdata []Pdata `xml:"pdata"`
}

// Pdata defines data with a portpotional map
type Pdata struct {
	Legend    string `xml:"legend,attr"`
	Stagger   string `xml:"stagger,attr"`
	Alternate string `xml:"alternate,attr"`
	Item      []Item `xml:"item"`
}

// Item defines an item with porpotional map data
type Item struct {
	Name  string  `xml:",chardata"`
	Value float64 `xml:"value,attr"`
}

var (
	width, height, fontsize, fontscale, round, gutter, pred, pgreen, pblue, oflen int
	bgcolor, olcolor, colorspec, title                                            string
	showpercent, showdata, alternate, showtitle, stagger, showlegend, showtotal   bool
	ofpct                                                                         float64
	leftmargin                                                                    = 40
	topmargin                                                                     = 40
	canvas                                                                        = svg.New(os.Stdout)
)

const (
	globalfmt   = "stroke-width:1;font-family:Calibri,sans-serif;text-anchor:middle;font-size:%dpt"
	legendstyle = "text-anchor:start;font-size:150%"
	linefmt     = "stroke:%s"
)

func dopmap(location string) {
	var f *os.File
	var err error
	if len(location) > 0 {
		f, err = os.Open(location)
	} else {
		f = os.Stdin
	}
	if err == nil {
		readpmap(f)
		f.Close()
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

func readpmap(r io.Reader) {
	var pm Pmap
	if err := xml.NewDecoder(r).Decode(&pm); err == nil {
		drawpmap(pm)
	} else {
		fmt.Fprintf(os.Stderr, "Unable to parse pmap (%v)\n", err)
	}
}

func drawpmap(m Pmap) {
	fs := fontsize
	if m.Left > 0 {
		leftmargin = m.Left
	}
	if m.Top > 0 {
		topmargin = m.Top
	} else {
		topmargin = fs * fontscale
	}
	x := leftmargin
	y := topmargin
	if len(m.Title) > 0 {
		title = m.Title
	}
	canvas.Title(title)
	if showtitle {
		dotitle(title)
	}
	for _, p := range m.Pdata {
		pmap(x, y, fs, p)
		y += fs*fontscale + (gutter + fs*2)
	}
}

func pmap(x, y, fs int, m Pdata) {
	var tfill, vfmt, oc string
	var up bool
	h := fs * fontscale
	fw := fs * 80
	slen := fs + (fs / 2)
	up = false

	sum := 0.0
	for _, v := range m.Item {
		sum += v.Value
	}

	if len(olcolor) > 0 {
		oc = olcolor
	} else {
		oc = bgcolor
	}
	loffset := (fs * fontscale) + fs
	gline := fmt.Sprintf(linefmt, "gray")
	wline := fmt.Sprintf(linefmt, oc)
	if len(m.Legend) > 0 && showlegend {
		if showtotal {
			canvas.Text(x, y-fs, fmt.Sprintf("%s (total: "+floatfmt(sum)+")", m.Legend, sum), legendstyle)
		} else {
			canvas.Text(x, y-fs, m.Legend, legendstyle)
		}
	}
	for i, p := range m.Item {
		k := p.Name
		v := p.Value
		if v == 0.0 {
			continue
		}
		pct := v / sum
		pw := int(pct * float64(fw))
		xw := x + (pw / 2)
		yh := y + (h / 2)
		if pct >= .4 {
			tfill = "fill:white"
		} else {
			tfill = "fill:black"
		}
		if round > 0 {
			canvas.Roundrect(x, y, pw, h, round, round, canvas.RGBA(pred, pgreen, pblue, pct))
		} else {
			canvas.Rect(x, y, pw, h, canvas.RGBA(pred, pgreen, pblue, pct))
		}

		dy := yh + fs + (fs / 2)
		if pct <= ofpct || len(k) > oflen { // overflow label
			if up {
				dy -= loffset
				yh -= loffset
				canvas.Line(xw, y, xw, dy+(fs/2), gline)
			} else {
				dy += loffset
				yh += loffset
				canvas.Line(xw, y+h, xw, dy-(fs*3), gline)
			}
			if alternate {
				up = !up
				slen = fs * 2
			} else {
				slen = fs * 3
			}
			if stagger {
				loffset += slen
			}
			tfill = "fill:black"
		}
		canvas.Text(xw, yh, k, tfill)
		dpfmt := tfill + ";font-size:75%"
		vfmt = floatfmt(v)
		switch {
		case showpercent && !showdata:
			canvas.Text(xw, dy, fmt.Sprintf("%.1f%%", pct*100), dpfmt)
		case showpercent && showdata:
			canvas.Text(xw, dy, fmt.Sprintf(vfmt+", %.1f%%", v, pct*100), dpfmt)
		case showdata && !showpercent:
			canvas.Text(xw, dy, fmt.Sprintf(vfmt, v), dpfmt)
		}
		x += pw
		if i < len(m.Item) {
			canvas.Line(x, y, x, y+h, wline)
		}
	}
}

func floatfmt(v float64) string {
	var vfmt = "%.1f"
	if v-float64(int(v)) == 0.0 {
		vfmt = "%.0f"
	}
	return vfmt
}

func dotitle(s string) {
	offset := 40
	canvas.Text(leftmargin, height-offset, s, "text-anchor:start;font-size:250%")
}

func init() {
	flag.IntVar(&width, "w", 1024, "width")
	flag.IntVar(&height, "h", 768, "height")
	flag.IntVar(&fontsize, "f", 12, "font size (pt)")
	flag.IntVar(&fontscale, "s", 5, "font scaling factor")
	flag.IntVar(&round, "r", 0, "rounded corner size")
	flag.IntVar(&gutter, "g", 100, "gutter")
	flag.IntVar(&oflen, "ol", 20, "overflow length")
	flag.StringVar(&bgcolor, "bg", "white", "background color")
	flag.StringVar(&olcolor, "oc", "", "outline color")
	flag.StringVar(&colorspec, "c", "0,0,0", "color (r,g,b)")
	flag.StringVar(&title, "t", "Proportions", "title")
	flag.BoolVar(&showpercent, "p", false, "show percentage")
	flag.BoolVar(&showdata, "d", false, "show data")
	flag.BoolVar(&alternate, "a", false, "alternate overflow labels")
	flag.BoolVar(&stagger, "stagger", false, "stagger labels")
	flag.BoolVar(&showlegend, "showlegend", true, "show the legend")
	flag.BoolVar(&showtitle, "showtitle", false, "show the title")
	flag.BoolVar(&showtotal, "showtotal", false, "show totals in the legend")
	flag.Float64Var(&ofpct, "op", 0.05, "overflow percentage")
	flag.Parse()
	fmt.Sscanf(colorspec, "%d,%d,%d", &pred, &pgreen, &pblue)
}

func main() {
	canvas.Start(width, height)
	canvas.Rect(0, 0, width, height, "fill:"+bgcolor)
	canvas.Gstyle(fmt.Sprintf(globalfmt, fontsize))

	if len(flag.Args()) == 0 {
		dopmap("")
	} else {
		for _, f := range flag.Args() {
			dopmap(f)
		}
	}
	canvas.Gend()
	canvas.End()
}
