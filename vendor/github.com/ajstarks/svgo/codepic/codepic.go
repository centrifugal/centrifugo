// codepic -- produce code+output sample suitable for slides
// +build !appengine

package main

import (
	"bufio"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ajstarks/svgo"
)

var (
	canvas                                                    = svg.New(os.Stdout)
	font                                                      string
	codeframe, picframe, syntax                               bool
	linespacing, fontsize, top, left, boxwidth, width, height int
)

const (
	framestyle = "stroke:gray;stroke-dasharray:1,1;fill:none"
	codefmt    = "font-family:%s;font-size:%dpx"
	labelfmt   = "text-anchor:middle;" + codefmt
	kwfmt      = `<tspan %s>%s</tspan>`
	commentfmt = `<tspan font-style="italic" fill="rgb(127,127,127)">%s</tspan>`
	textfmt    = "<text x=\"%d\" y=\"%d\" xml:space=\"preserve\">%s</text>\n"
	svgofmt    = `font-weight="bold"  fill="rgb(0,0,127)"`
	gokwfmt    = `font-style="italic" fill="rgb(127,0,0)"`
)

// SVG is the incoming SVG file, capture everything into between <svg..> and </svg>
// in the Doc string.  This code will be translated to form the "picture" portion
type SVG struct {
	Width  int    `xml:"width,attr"`
	Height int    `xml:"height,attr"`
	Doc    string `xml:",innerxml"`
}

var gokw = []string{
	"defer ", "go ", "range ", "chan ", " continue", "if ", "for ", "func ",
	"uint8", "uint", "uint16", "uint32", "complex64", "complex128", " byte", "int8", "int16", "int32",
	"int64", " int", "float64", "float32", " string", "import ", "const ",
	"package ", "return", "var ", "type ", "switch ", "case ", "default:",
}

var svgokw = []string{
	".Start", ".Startview,", ".End", ".Script", ".Gstyle", ".Gtransform", ".Scale", ".Offcolor",
	".ScaleXY", ".SkewX", ".SkewY", ".SkewXY,", ".Rotate", ".TranslateRotate", ".RotateTranslate", ".Translate",
	".Group", ".Gid", ".Gend", ".ClipPath", ".ClipEnd", ".DefEnd", ".Def", ".Desc", ".Title", ".Linkf",
	".LinkEnd", ".Use", ".Mask", ".MaskEnd", ".Circle", ".Ellipse", ".Polygon", ".Rect", ".CenterRect",
	".Roundrect", ".Square", ".Path", ".Arc", ".Bezier", ".Qbez", ".Qbezier", ".Line", ".Polyline", ".Image",
	".Textpath", ".Textlines,", ".Text", ".RGBA", ".RGB", ".LinearGradient", ".RadialGradient", ".Grid",
}

// codepic makes a code+picture SVG file, given a go source file
// and conventionally named output -- given <name>.go, <name>.svg
func codepic(filename string) {
	var basename string

	bn := strings.Split(filename, ".")
	if len(bn) > 0 {
		basename = bn[0]
	} else {
		fmt.Fprintf(os.Stderr, "cannot get the basename for %s\n", filename)
		return
	}
	canvas.Start(width, height)
	canvas.Title(basename)
	canvas.Rect(0, 0, width, height, "fill:white")
	placepic(width/2, top, basename)
	canvas.Gstyle(fmt.Sprintf(codefmt, font, fontsize))
	placecode(left+fontsize, top+fontsize*2, filename)
	canvas.Gend()
	canvas.End()
}

// placecode places the code section on the left
func placecode(x, y int, filename string) {
	var rerr error
	var line string
	var ic bool
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}
	defer f.Close()
	in := bufio.NewReader(f)

	for xp := left + fontsize; rerr == nil; y += linespacing {
		line, rerr = in.ReadString('\n')
		if len(line) > 0 {
			line, ic = svgtext(xp, y, line[0:len(line)-1])
			if !ic && syntax {
				line = keyword(line, gokwfmt, gokw)
				line = keyword(line, svgofmt, svgokw)
			}
			io.WriteString(canvas.Writer, line)
		}
	}
	if codeframe {
		canvas.Rect(top, left, left+boxwidth, y, framestyle)
	}
}

// keyword styles keywords in a line of code
func keyword(line string, style string, kw []string) string {
	for _, k := range kw {
		line = strings.Replace(line, k, fmt.Sprintf(kwfmt, style, k), 1)
	}
	return line
}

// svgtext
func svgtext(x, y int, s string) (string, bool) {
	var iscomment = false
	s = strings.Replace(s, "&", "&amp;", -1)
	s = strings.Replace(s, "<", "&lt;", -1)
	s = strings.Replace(s, ">", "&gt;", -1)
	s = strings.Replace(s, "\t", "    ", -1)

	if syntax {
		i := strings.Index(s, "// ")
		if i >= 0 {
			iscomment = true
			s = strings.Replace(s, s[i:], fmt.Sprintf(commentfmt, s[i:]), 1)
		}
	}
	return fmt.Sprintf(textfmt, x, y, s), iscomment
}

// placepic places the picture on the right
func placepic(x, y int, basename string) {
	var s SVG
	f, err := os.Open(basename + ".svg")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}
	defer f.Close()
	if err := xml.NewDecoder(f).Decode(&s); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse (%v)\n", err)
		return
	}
	canvas.Text(x, height-10, basename+".go", fmt.Sprintf(labelfmt, font, fontsize*2))
	canvas.Group(`clip-path="url(#pic)"`, fmt.Sprintf(`transform="translate(%d,%d)"`, x, y))
	canvas.ClipPath(`id="pic"`)
	canvas.Rect(0, 0, s.Width, s.Height)
	canvas.ClipEnd()
	io.WriteString(canvas.Writer, s.Doc)
	canvas.Gend()
	if picframe {
		canvas.Rect(x, y, s.Width, s.Height, framestyle)
	}
}

// init initializes flags
func init() {
	flag.BoolVar(&codeframe, "codeframe", false, "frame the code")
	flag.BoolVar(&picframe, "picframe", false, "frame the picture")
	flag.BoolVar(&syntax, "syntax", false, "syntax coloring")
	flag.IntVar(&width, "w", 1024, "width")
	flag.IntVar(&height, "h", 768, "height")
	flag.IntVar(&linespacing, "ls", 16, "linespacing")
	flag.IntVar(&fontsize, "fs", 14, "fontsize")
	flag.IntVar(&top, "top", 20, "top")
	flag.IntVar(&left, "left", 20, "left")
	flag.IntVar(&boxwidth, "boxwidth", 450, "boxwidth")
	flag.StringVar(&font, "font", "Inconsolata", "font name")
	flag.Parse()
}

// for every file, make a code+pic SVG file
func main() {
	for _, f := range flag.Args() {
		codepic(f)
	}
}
