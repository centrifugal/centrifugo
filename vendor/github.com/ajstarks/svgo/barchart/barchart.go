// barchart - bar chart
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
)

var (
	width, height, iscale, fontsize, barheight, gutter, cornerRadius, labelimit int
	bgcolor, barcolor, title, inbar, valformat                                  string
	showtitle, showdata, showgrid, showscale, endtitle, trace                   bool
)

const (
	gstyle      = "font-family:Calibri,sans-serif;font-size:%dpx"
	borderstyle = "stroke:lightgray;stroke-width:1px"
	scalestyle  = "text-anchor:middle;font-size:75%"
	btitlestyle = "font-style:italic;font-size:150%;text-anchor:"
	notestyle   = "font-style:italic;text-anchor:"
	datastyle   = "text-anchor:end;fill:"
	titlestyle  = "text-anchor:start;font-size:300%"
	labelstyle  = "fill:black;baseline-shift:-25%"
)

// a Barchart Defintion
// <barchart title="Bullet Graph" top="50" left="250" right="50">
//    <note>This is a note</note>
//    <note>More expository text</note>
//    <bdata title="Browser Market Share" scale="0,100,20" showdata="true" color="red" unit="%"/>
//    	<bitem name="Firefox"  value="22.5" color="green"/>
//    	<bitem name="Chrome" value="12.3"/>
//		<bitem name="IE8" value="63.5"/>
//	  <bdata>
// </barchart>

type Barchart struct {
	Top   int     `xml:"top,attr"`
	Left  int     `xml:"left,attr"`
	Right int     `xml:"right,attr"`
	Title string  `xml:"title,attr"`
	Bdata []bdata `xml:"bdata"`
	Note  []note  `xml:"note"`
}

type bdata struct {
	Title    string   `xml:"title,attr"`
	Scale    string   `xml:"scale,attr"`
	Color    string   `xml:"color,attr"`
	Unit     string   `xml:"unit,attr"`
	Showdata bool     `xml:"showdata,attr"`
	Showgrid bool     `xml:"showgrid,attr"`
	Samebar  bool     `xml:"samebar,attr"`
	Bitem    []bitem  `xml:"bitem"`
	Bstack   []bstack `xml:"bstack"`
	Note     []note   `xml:"note"`
}

type bitem struct {
	Name    string  `xml:"name,attr"`
	Value   float64 `xml:"value,attr"`
	Color   string  `xml:"color,attr"`
	Samebar bool    `xml:"samebar,attr"`
}

type bstack struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
	Color string `xml:"color,attr"`
}

type note struct {
	Text string `xml:",chardata"`
}

// dobc does file i/o
func dobc(location string, s *svg.SVG) {
	var f *os.File
	var err error
	if len(location) > 0 {
		f, err = os.Open(location)
	} else {
		f = os.Stdin
	}
	if err == nil {
		readbc(f, s)
		f.Close()
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

// readbc reads and parses the XML specification
func readbc(r io.Reader, s *svg.SVG) {
	var bc Barchart
	if err := xml.NewDecoder(r).Decode(&bc); err == nil {
		drawbc(bc, s)
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

// drawbc draws the bar chart
func drawbc(bg Barchart, canvas *svg.SVG) {

	if bg.Left == 0 {
		bg.Left = 250
	}
	if bg.Right == 0 {
		bg.Right = 50
	}
	if bg.Top == 0 {
		bg.Top = 50
	}
	if len(title) > 0 {
		bg.Title = title
	}
	labelimit = bg.Left / 8
	cr := cornerRadius
	maxwidth := width - (bg.Left + bg.Right)
	x := bg.Left
	y := bg.Top
	sep := 10
	color := barcolor
	scfmt := "%v"
	canvas.Title(bg.Title)

	// for each bdata element...
	for _, b := range bg.Bdata {
		if trace {
			fmt.Fprintf(os.Stderr, "# %s\n", b.Title)
		}
		// overide the color if specified
		if len(b.Color) > 0 {
			color = b.Color
		} else {
			color = barcolor
		}
		// extract the scale data from the XML attributes
		// if not specified, compute the scale factors
		sc := strings.Split(b.Scale, ",")
		var scalemin, scalemax, scaleincr float64

		if len(sc) != 3 {
			if len(b.Bitem) > 0 {
				scalemin, scalemax, scaleincr = scalevalues(b.Bitem)
			}

			if len(b.Bstack) > 0 {
				scalemin, scalemax, scaleincr = scalestack(b.Bstack)
			}
		} else {
			scalemin, _ = strconv.ParseFloat(sc[0], 64)
			scalemax, _ = strconv.ParseFloat(sc[1], 64)
			scaleincr, _ = strconv.ParseFloat(sc[2], 64)
		}
		// label the graph
		canvas.Text(x, y, b.Title, btitlestyle+anchor())

		y += sep * 2
		chartop := y

		// draw the data items
		canvas.Gstyle(datastyle + color)

		// stacked bars
		for _, stack := range b.Bstack {
			if trace {
				fmt.Fprintf(os.Stderr, "%s~%s\n", stack.Value, stack.Name)
			}
			stackdata := stackvalues(stack.Value)
			if len(stackdata) < 1 {
				continue
			}
			sx := x
			canvas.Text(x-sep, y+barheight/2, textlimit(stack.Name, labelimit), labelstyle)
			barop := colorange(1.0, 0.3, len(stackdata))
			for ns, sd := range stackdata {
				dw := vmap(sd, scalemin, scalemax, 0, float64(maxwidth))
				if len(stack.Color) > 0 {
					canvas.Roundrect(sx, y, int(dw), barheight, cr, cr, fmt.Sprintf("fill:%s;fill-opacity:%.2f", stack.Color, barop[ns]))
				} else {
					canvas.Roundrect(sx, y, int(dw), barheight, cr, cr, fmt.Sprintf("fill-opacity:%.2f", barop[ns]))
				}

				if (showdata || b.Showdata) && sd > 0 {
					var valuestyle = "fill-opacity:1;font-style:italic;font-size:75%;text-anchor:middle;baseline-shift:-25%;"
					var ditem string
					var datax int
					if len(b.Unit) > 0 {
						ditem = fmt.Sprintf(valformat+"%s", sd, b.Unit)
					} else {
						ditem = fmt.Sprintf(valformat, sd)
					}
					if len(inbar) > 0 {
						valuestyle += inbar
					} else {
						valuestyle += "fill:black"
					}
					datax = sx + int(dw)/2
					canvas.Text(datax, y+barheight/2, ditem, valuestyle)
				}
				sx += int(dw)
			}
			y += barheight + gutter
		}

		// plain bars
		for _, d := range b.Bitem {
			if trace {
				fmt.Fprintf(os.Stderr, "%.2f~%s\n", d.Value, d.Name)
			}
			canvas.Text(x-sep, y+barheight/2, textlimit(d.Name, labelimit), labelstyle)
			dw := vmap(d.Value, scalemin, scalemax, 0, float64(maxwidth))
			var barop float64
			if b.Samebar {
				barop = 0.3
			} else {
				barop = 1.0
			}
			if len(d.Color) > 0 {
				canvas.Roundrect(x, y, int(dw), barheight, cr, cr, fmt.Sprintf("fill:%s;fill-opacity:%.2f", d.Color, barop))
			} else {
				canvas.Roundrect(x, y, int(dw), barheight, cr, cr, fmt.Sprintf("fill-opacity:%.2f", barop))
			}
			if showdata || b.Showdata {
				var valuestyle = "fill-opacity:1;font-style:italic;font-size:75%;text-anchor:start;baseline-shift:-25%;"
				var ditem string
				var datax int
				if len(b.Unit) > 0 {
					ditem = fmt.Sprintf(valformat+"%s", d.Value, b.Unit)
				} else {
					ditem = fmt.Sprintf(valformat, d.Value)
				}
				if len(inbar) > 0 {
					valuestyle += inbar
					datax = x + fontsize/2
				} else {
					valuestyle += "fill:black"
					datax = x + int(dw) + fontsize/2
				}
				canvas.Text(datax, y+barheight/2, ditem, valuestyle)
			}
			if !d.Samebar {
				y += barheight + gutter
			}
		}
		canvas.Gend()

		// draw the scale and borders
		chartbot := y + gutter
		if showgrid || b.Showgrid {
			canvas.Line(x, chartop, x+maxwidth, chartop, borderstyle)                 // top border
			canvas.Line(x, chartbot-gutter, x+maxwidth, chartbot-gutter, borderstyle) // bottom border
		}
		if showscale {
			if scaleincr < 1 {
				scfmt = "%.1f"
			} else {
				scfmt = "%0.f"
			}
			canvas.Gstyle(scalestyle)
			for sc := scalemin; sc <= scalemax; sc += scaleincr {
				scx := vmap(sc, scalemin, scalemax, 0, float64(maxwidth))
				canvas.Text(x+int(scx), chartbot+fontsize, fmt.Sprintf(scfmt, sc))
				if showgrid || b.Showgrid {
					canvas.Line(x+int(scx), chartbot, x+int(scx), chartop, borderstyle) // grid line
				}
			}
			canvas.Gend()
		}

		// apply the note if present
		if len(b.Note) > 0 {
			canvas.Gstyle(notestyle + anchor())
			y += fontsize * 2
			leading := 3
			for _, note := range b.Note {
				canvas.Text(bg.Left, y, note.Text)
				y += fontsize + leading
			}
			canvas.Gend()
		}
		y += sep * 7 // advance vertically for the next chart
	}
	// if requested, place the title below the last chart
	if showtitle && len(bg.Title) > 0 {
		y += fontsize * 2
		canvas.Text(bg.Left, y, bg.Title, titlestyle)
	}
	// apply overall note if present
	if len(bg.Note) > 0 {
		canvas.Gstyle(notestyle + anchor())
		y += fontsize * 2
		leading := 3
		for _, note := range bg.Note {
			canvas.Text(bg.Left, y, note.Text)
			y += fontsize + leading
		}
		canvas.Gend()
	}
}

func anchor() string {
	if endtitle {
		return "end"
	}
	return "start"
}

// vmap maps one interval to another
func vmap(value float64, low1 float64, high1 float64, low2 float64, high2 float64) float64 {
	return low2 + (high2-low2)*(value-low1)/(high1-low1)
}

// maxitem finds the maxima is a collection of bar items
func maxitem(data []bitem) float64 {
	max := -math.SmallestNonzeroFloat64
	for _, d := range data {
		if d.Value > max {
			max = d.Value
		}
	}
	return max
}

// maxstack finds the maxima is a stack of bars
func maxstack(stacks []bstack) float64 {
	max := -math.SmallestNonzeroFloat64
	for _, s := range stacks {
		sv := stackvalues(s.Value)
		sum := 0.0
		for _, d := range sv {
			sum += d
		}
		if sum > max {
			max = sum
		}
	}
	return max
}

// scale values returns the min, max, increment from a set of bar items
func scalevalues(data []bitem) (float64, float64, float64) {
	var m, max, increment float64
	rui := 5
	m = maxitem(data)
	max = roundup(m, 100)
	if max > 2 {
		increment = roundup(max/float64(rui), 10)
	} else {
		increment = 0.4
	}
	return 0, max, increment
}

// scalestack returns the min, max, increment from a stack of bars
func scalestack(data []bstack) (float64, float64, float64) {
	var m, max, increment float64
	rui := 5
	m = maxstack(data)
	max = roundup(m, 100)
	if max > 2 {
		increment = roundup(max/float64(rui), 10)
	} else {
		increment = 0.4
	}
	return 0, max, increment
}

// roundup rouds a floating point number up
func roundup(n float64, m int) float64 {
	i := int(n)
	if i <= 2 {
		return 2
	}
	for ; i%m != 0; i++ {
	}
	return float64(i)
}

// stack value returns the values from the value string of a stack
func stackvalues(s string) []float64 {
	v := strings.Split(s, "/")
	if len(v) <= 0 {
		return nil
	}
	vals := make([]float64, len(v))
	for i, x := range v {
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			vals[i] = 0
		} else {
			vals[i] = f
		}
	}
	return vals
}

// colorange evenly distributes opacity across a range of values
func colorange(start, end float64, n int) []float64 {
	v := make([]float64, n)
	v[0] = start
	v[n-1] = end
	if n == 2 {
		return v
	}
	incr := (end - start) / float64(n-1)
	for i := 1; i < n-1; i++ {
		v[i] = v[i-1] + incr
	}
	return v
}

func textlimit(s string, n int) string {
	l := len(s)
	if l <= n {
		return s
	}

	return s[0:n-3] + "..."
}

// init sets up the command flags
func init() {
	flag.StringVar(&bgcolor, "bg", "white", "background color")
	flag.StringVar(&barcolor, "bc", "rgb(200,200,200)", "bar color")
	flag.StringVar(&valformat, "vfmt", "%v", "value format")
	flag.IntVar(&width, "w", 1024, "width")
	flag.IntVar(&height, "h", 800, "height")
	flag.IntVar(&barheight, "bh", 20, "bar height")
	flag.IntVar(&gutter, "g", 5, "gutter")
	flag.IntVar(&cornerRadius, "cr", 0, "corner radius")
	flag.IntVar(&fontsize, "f", 18, "fontsize (px)")
	flag.BoolVar(&showscale, "showscale", true, "show scale")
	flag.BoolVar(&showgrid, "showgrid", false, "show grid")
	flag.BoolVar(&showdata, "showdata", false, "show data values")
	flag.BoolVar(&showtitle, "showtitle", false, "show title")
	flag.BoolVar(&endtitle, "endtitle", false, "align title to the end")
	flag.BoolVar(&trace, "trace", false, "show name/value pairs")
	flag.StringVar(&inbar, "inbar", "", "data in bar format")
	flag.StringVar(&title, "t", "", "title")
}

// for every input file (or stdin), draw a bar graph
// as specified by command flags
func main() {
	flag.Parse()
	canvas := svg.New(os.Stdout)
	canvas.Start(width, height)
	canvas.Rect(0, 0, width, height, "fill:"+bgcolor)
	canvas.Gstyle(fmt.Sprintf(gstyle, fontsize))
	if len(flag.Args()) == 0 {
		dobc("", canvas)
	} else {
		for _, f := range flag.Args() {
			dobc(f, canvas)
		}
	}
	canvas.Gend()
	canvas.End()
}
