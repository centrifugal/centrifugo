// colortab -- make a color/code placemat
// +build !appengine

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ajstarks/svgo"
)

func main() {
	var (
		canvas                 = svg.New(os.Stdout)
		filename               = flag.String("f", "svgcolors.txt", "input file")
		fontname               = flag.String("font", "Calibri,sans-serif", "fontname")
		outline                = flag.Bool("o", false, "outline")
		neg                    = flag.Bool("n", false, "negative")
		showrgb                = flag.Bool("rgb", false, "show RGB")
		showcode               = flag.Bool("showcode", true, "only show colors")
		circsw                 = flag.Bool("circle", true, "circle swatch")
		fontsize               = flag.Int("fs", 12, "fontsize")
		width                  = flag.Int("w", 1600, "width")
		height                 = flag.Int("h", 900, "height")
		rowsize                = flag.Int("r", 32, "rowsize")
		colw                   = flag.Int("c", 320, "column size")
		swatch                 = flag.Int("s", 16, "swatch size")
		gutter                 = flag.Int("g", 11, "gutter")
		err                    error
		colorfmt, tcolor, line string
	)

	flag.Parse()
	f, oerr := os.Open(*filename)
	if oerr != nil {
		fmt.Fprintf(os.Stderr, "%v\n", oerr)
		return
	}
	canvas.Start(*width, *height)
	canvas.Title("SVG Color Table")
	if *neg {
		canvas.Rect(0, 0, *width, *height, "fill:black")
		tcolor = "white"
	} else {
		canvas.Rect(0, 0, *width, *height, "fill:white")
		tcolor = "black"
	}
	top := 32
	left := 32
	in := bufio.NewReader(f)
	canvas.Gstyle(fmt.Sprintf("font-family:%s;font-size:%dpt;fill:%s",
		*fontname, *fontsize, tcolor))
	for x, y, nr := left, top, 0; err == nil; nr++ {
		line, err = in.ReadString('\n')
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if nr%*rowsize == 0 && nr > 0 {
			x += *colw
			y = top
		}
		if len(fields) == 3 {
			colorfmt = "fill:" + fields[1]
			if *outline {
				colorfmt = colorfmt + ";stroke-width:1;stroke:" + tcolor
			}
			if *circsw {
				canvas.Circle(x, y, *swatch/2, colorfmt)
			} else {
				canvas.CenterRect(x, y, *swatch, *swatch, colorfmt)
			}
			canvas.Text(x+*swatch+*fontsize/2, y+(*swatch/4), fields[0], "stroke:none")
			var label string
			if *showcode {
				if *showrgb {
					label = fields[1]
				} else {
					label = fields[2]
				}
				canvas.Text(x+((*colw*4)/5), y+(*swatch/4), label, "text-anchor:end;fill:gray")
			}
		}
		y += (*swatch + *gutter)
	}
	canvas.Gend()
	canvas.End()
}
