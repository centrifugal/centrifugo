// structlayout-svg: generate SVG struct layouts
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ajstarks/svgo"
	"honnef.co/go/tools/structlayout"
)

var title, bgcolor, scolor, pcolor string

func init() {
	flag.StringVar(&title, "t", "Structure Layout", "title")
	flag.StringVar(&bgcolor, "bgcolor", "rgb(240,240,240)", "background color")
	flag.StringVar(&scolor, "scolor", "steelblue", "structure color")
	flag.StringVar(&pcolor, "pcolor", "maroon", "padding color")
}

func main() {
	log.SetFlags(0)
	flag.Parse()

	// Decode the JSON
	var fields []structlayout.Field
	if err := json.NewDecoder(os.Stdin).Decode(&fields); err != nil {
		log.Fatal(err)
	}
	if len(fields) == 0 {
		return
	}

	// layout variables
	top := 50
	structheight := 50
	structwidth := 100
	gutter := 8
	byteheight := 10
	fontsize := byteheight + byteheight/2
	width := 600

	// Determine the height of the canvas
	height := top + gutter
	for _, f := range fields {
		height += byteheight * int(f.Size)
		height += gutter
	}
	x := width / 10
	y := top

	// Set up the canvas
	canvas := svg.New(os.Stdout)
	canvas.Start(width, height)
	canvas.Rect(0, 0, width, height, "fill:"+bgcolor)
	canvas.Gstyle(fmt.Sprintf("font-size:%dpx;font-family:sans-serif", fontsize))
	canvas.Text(x+structwidth/2, top/2, title, "text-anchor:middle;font-size:120%")

	// For every field, draw a labled box
	pos := int64(0)
	var fillcolor string
	nudge := (fontsize * 2) / 3
	for _, f := range fields {
		name := f.Name + " " + f.Type
		if f.IsPadding {
			name = "padding"
			fillcolor = pcolor
		} else {
			fillcolor = scolor
		}
		structheight = byteheight * int(f.Size)
		canvas.Rect(x, y, structwidth, structheight, "fill:"+fillcolor)
		canvas.Text(x+structwidth+10, y+nudge, fmt.Sprintf("%d", pos))
		canvas.Text(x+structwidth+fontsize*4, y+nudge, fmt.Sprintf("%s (size %d, align %d)", name, f.Size, f.Align))

		if f.Size > 2 {
			canvas.Text(x-10, y+structheight, fmt.Sprintf("%d", pos+f.Size-1), "text-anchor:end")
		}
		pos += f.Size
		y += structheight + gutter
	}
	canvas.Gend()
	canvas.End()
}
