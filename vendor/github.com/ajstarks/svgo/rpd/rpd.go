package main

// Imports
import (
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/ajstarks/svgo"
	"io"
	"os"
)

// <thing top="100" left="100" sep="100">
//    <item width="50"  height="50"  name="Little" color="blue">This is small</item>
//    <item width="75"  height="100" name="Med"    color="green">This is medium</item>
//    <item width="100" height="200" name="Big"    color="red">This is large</item>
// </thing>

type Thing struct {
	Top  int    `xml:"top,attr"`
	Left int    `xml:"left,attr"`
	Sep  int    `xml:"sep,attr"`
	Item []item `xml:"item"`
}

type item struct {
	Width  int    `xml:"width,attr"`
	Height int    `xml:"height,attr"`
	Name   string `xml:"name,attr"`
	Color  string `xml:"color,attr"`
	Text   string `xml:",chardata"`
}

var (
	width  = flag.Int("w", 1024, "width")
	height = flag.Int("h", 768, "height")
	canvas = svg.New(os.Stdout)
)

// Open the file
func dothing(location string) {
	f, err := os.Open(location)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}
	defer f.Close()
	readthing(f)
}

// Read the file, loading the defined structure
func readthing(r io.Reader) {
	var t Thing
	if err := xml.NewDecoder(r).Decode(&t); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse components (%v)\n", err)
		return
	}
	drawthing(t)
}

// use the items of "thing" to make the picture
func drawthing(t Thing) {
	x := t.Left
	y := t.Top
	for _, v := range t.Item {
		style := fmt.Sprintf("font-size:%dpx;fill:%s", v.Width/2, v.Color)
		canvas.Circle(x, y, v.Height/4, "fill:"+v.Color)
		canvas.Text(x+t.Sep, y, v.Name+":"+v.Text+"/"+v.Color, style)
		y += v.Height
	}
}

func main() {
	flag.Parse()
	for _, f := range flag.Args() {
		canvas.Start(*width, *height)
		dothing(f)
		canvas.End()
	}
}
