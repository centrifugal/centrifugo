// webfonts demo
// +build !appengine

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ajstarks/svgo"
)

var (
	canvas   = svg.New(os.Stdout)
	width    = 500
	height   = 1100
	fontlist = "Sue Ellen Francisco|Over the Rainbow|Pacifico|Inconsolata|Miltonian|Megrim|Monofett|Permanent Marker|Homemade Apple|Ultra"
)

const (
	gwfURI  = "http://fonts.googleapis.com/css?family="
	fontfmt = "<style type=\"text/css\">\n<![CDATA[\n%s]]>\n</style>\n"
	gfmt    = "fill:white;font-size:36pt;text-anchor:middle"
)

func googlefont(f string) []string {
	empty := []string{}
	r, err := http.Get(gwfURI + url.QueryEscape(f))
	if err != nil {
		return empty
	}
	defer r.Body.Close()
	b, rerr := ioutil.ReadAll(r.Body)
	if rerr != nil || r.StatusCode != http.StatusOK {
		return empty
	}
	canvas.Def()
	fmt.Fprintf(canvas.Writer, fontfmt, b)
	canvas.DefEnd()
	return strings.Split(fontlist, "|")
}

func main() {
	canvas.Start(width, height)
	canvas.Title("Webfonts")
	if len(os.Args) > 1 {
		fontlist = os.Args[1]
	}
	fl := googlefont(fontlist)
	canvas.Rect(0, 0, width, height)
	canvas.Ellipse(width/2, height+50, width/2, height/5, "fill:rgb(44,77,232)")
	canvas.Gstyle(gfmt)
	for i, f := range fl {
		canvas.Text(width/2, (i+1)*100, "Hello, World", "font-family:"+f)
	}
	canvas.Gend()
	canvas.End()
}
