// websvg draws SVG in a web server
// +build !appengine

package main

import (
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/ajstarks/svgo"
)

const defaultstyle = "fill:rgb(127,0,0)"

var port = flag.String("port", ":2003", "http service address")

func main() {
	flag.Parse()
	http.Handle("/circle/", http.HandlerFunc(circle))
	http.Handle("/rect/", http.HandlerFunc(rect))
	http.Handle("/arc/", http.HandlerFunc(arc))
	http.Handle("/text/", http.HandlerFunc(text))
	err := http.ListenAndServe(*port, nil)
	if err != nil {
		log.Println("ListenAndServe:", err)
	}
}

func shapestyle(path string) string {
	i := strings.LastIndex(path, "/") + 1
	if i > 0 && len(path[i:]) > 0 {
		return "fill:" + path[i:]
	}
	return defaultstyle
}

func circle(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	s := svg.New(w)
	s.Start(500, 500)
	s.Title("Circle")
	s.Circle(250, 250, 125, shapestyle(req.URL.Path))
	s.End()
}

func rect(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	s := svg.New(w)
	s.Start(500, 500)
	s.Title("Rectangle")
	s.Rect(250, 250, 100, 200, shapestyle(req.URL.Path))
	s.End()
}

func arc(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	s := svg.New(w)
	s.Start(500, 500)
	s.Title("Arc")
	s.Arc(250, 250, 100, 100, 0, false, false, 100, 125, shapestyle(req.URL.Path))
	s.End()
}

func text(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	s := svg.New(w)
	s.Start(500, 500)
	s.Title("Text")
	s.Text(250, 250, "Hello, world", "text-anchor:middle;font-size:32px;"+shapestyle(req.URL.Path))
	s.End()
}
