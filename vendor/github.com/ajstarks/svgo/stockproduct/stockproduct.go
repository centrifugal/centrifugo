// stockproduct draws a bar chart comparing stock price to products
// +build !appengine

package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"os"

	"github.com/ajstarks/svgo"
)

// Parameters defines options
type Parameters struct {
	showline, showimage, showproduct, showprice, showdate, showgrid bool
	x, y, w, h, width, height, spacing, fontsize, dot               int
	minvalue, maxvalue, ginterval, opacity, rotatetext              float64
	barcolor                                                        string
}

// <stockproduct title="Apple Products and Stock Price">
//    <sdata price="7.38" date="2002-08" product="Jaguar" image="images/jaguar.png"/>
//    <sdata price="11.44" date="2003-10" product="Panther" image="images/panther.png"/>
//    <sdata price="41.67" date="2005-03" product="Tiger" image="images/tiger.png"/>
//    <sdata price="172.75" date="2007-10" product="Leopard" image="images/leopard.jpg"/>
//    <sdata price="170.05" date="2009-08" product="Snow Leopard" image="images/snowleopard.jpg"/>
//    <sdata price="399.68" date="2011-07" product="Lion" image="images/lion.png"/>
// </stockproduct>

// StockProduct is the top-level drawing
type StockProduct struct {
	Title string  `xml:"title,attr"`
	Sdata []Sdata `xml:"sdata"`
}

// Sdata defines stock data
type Sdata struct {
	Price   float64 `xml:"price,attr"`
	Date    string  `xml:"date,attr"`
	Product string  `xml:"product,attr"`
	Image   string  `xml:"image,attr"`
}

// vmap maps ranges
func vmap(value float64, low1 float64, high1 float64, low2 float64, high2 float64) float64 {
	return low2 + (high2-low2)*(value-low1)/(high1-low1)

}

// barchart draws a chart from data read at location, on a SVG canvas
// if the location is the empty string, read from standard input.
// Data items are scaled according to the width, with parameters controlling the visibility
// of lines, products, images, and dates
func (p *Parameters) barchart(location string, canvas *svg.SVG) {
	var (
		f   *os.File
		err error
		sp  StockProduct
	)
	if len(location) > 0 {
		f, err = os.Open(location)
	} else {
		f = os.Stdin
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer f.Close()
	if err := xml.NewDecoder(f).Decode(&sp); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	bottom := p.y + p.h
	interval := p.w / (len(sp.Sdata) - 1)
	bw := interval - p.spacing
	offset := 120
	halfoffset := offset / 2

	if bw < 2 {
		bw = 2
	}
	canvas.Text(p.x, p.y-halfoffset, sp.Title, "font-size:300%")
	if p.showgrid {
		canvas.Gstyle("stroke:lightgray;stroke-width:1px")
		gx := p.x - (bw / 2)
		for i := p.maxvalue; i >= p.minvalue; i -= p.ginterval {
			yp := int(vmap(i, p.minvalue, p.maxvalue, float64(p.y), float64(bottom)))
			by := p.y + (bottom - yp)
			canvas.Line(gx, by, p.x+p.w+(bw/2), by)
			canvas.Text(gx-halfoffset, by, fmt.Sprintf("%.0f", i), "fill:black;stroke:none")
		}
		canvas.Gend()
	}
	canvas.Gstyle(fmt.Sprintf("stroke-opacity:%.2f;stroke:%s;stroke-width:%d;text-anchor:middle", p.opacity, p.barcolor, bw))
	for _, d := range sp.Sdata {
		yp := int(vmap(d.Price, p.minvalue, p.maxvalue, float64(p.y), float64(bottom)))
		by := p.y + (bottom - yp)
		if p.showline {
			canvas.Line(p.x, bottom, p.x, by)
		}
		if p.dot > 0 {
			canvas.Circle(p.x, by, p.dot, fmt.Sprintf("stroke:none;fill-opacity:%.2f;fill:%s", p.opacity, p.barcolor))
		}
		if p.showimage {
			if len(d.Image) > 0 {
				canvas.Image(p.x-bw/2, by-offset-p.fontsize, bw, offset, d.Image)
			}
		}
		canvas.Gstyle("stroke:none;fill:black")
		if p.showproduct {
			if p.rotatetext != 0 {
				canvas.TranslateRotate(p.x, bottom+40, p.rotatetext)
				canvas.Text(0, 0, d.Product)
				canvas.Gend()
			} else {
				canvas.Text(p.x, bottom+40, d.Product)
			}
		}
		if p.showprice {
			canvas.Text(p.x, by+p.fontsize, fmt.Sprintf("%.2f", d.Price), "font-size:150%;font-weight:bold")
		}
		if p.showdate {
			canvas.Text(p.x, bottom+20, d.Date)
		}
		canvas.Gend()
		p.x += interval
	}
	canvas.Gend()
}

var param Parameters

// set parameters according to command flags
func init() {
	flag.BoolVar(&param.showline, "line", true, "show lines")
	flag.BoolVar(&param.showimage, "image", true, "show images")
	flag.BoolVar(&param.showproduct, "product", true, "show products")
	flag.BoolVar(&param.showprice, "price", true, "show prices")
	flag.BoolVar(&param.showdate, "date", true, "show dates")
	flag.BoolVar(&param.showgrid, "grid", true, "show grid")
	flag.IntVar(&param.width, "w", 1600, "overall width")
	flag.IntVar(&param.height, "h", 900, "overall height")
	flag.IntVar(&param.x, "left", 150, "left")
	flag.IntVar(&param.y, "top", 120, "top")
	flag.IntVar(&param.w, "gw", 1400, "graph width")
	flag.IntVar(&param.h, "gh", 700, "graph height")
	flag.IntVar(&param.dot, "dot", 0, "dotsize")
	flag.IntVar(&param.fontsize, "fs", 14, "font size (px)")
	flag.IntVar(&param.spacing, "spacing", 15, "bar spacing")
	flag.Float64Var(&param.maxvalue, "max", 400, "max value")
	flag.Float64Var(&param.minvalue, "min", 0, "max value")
	flag.Float64Var(&param.ginterval, "ginterval", 50, "max value")
	flag.Float64Var(&param.opacity, "opacity", 0.5, "bar opacity")
	flag.Float64Var(&param.rotatetext, "rt", 0, "rotate text")
	flag.StringVar(&param.barcolor, "color", "lightgray", "bar color")
	flag.Parse()
}

func main() {
	width := 1600
	height := 900
	canvas := svg.New(os.Stdout)
	canvas.Start(param.width, param.height)
	canvas.Rect(0, 0, width, height, canvas.RGB(255, 255, 255))
	canvas.Gstyle(fmt.Sprintf("font-family:Calibri,sans-serif;font-size:%dpx", param.fontsize))
	if len(flag.Args()) == 0 {
		param.barchart("", canvas)
	} else {
		for _, f := range flag.Args() {
			param.barchart(f, canvas)
		}
	}
	canvas.Gend()
	canvas.End()
}
