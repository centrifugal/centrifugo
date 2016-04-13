// tsg -- twitter search grid
// +build !appengine

package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/ajstarks/svgo"
)

var canvas = svg.New(os.Stdout)

// Feed is the Atom feed structure
type Feed struct {
	XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	Entry   []Entry  `xml:"entry"`
}

// Entry defines an entry within an Aton feed
type Entry struct {
	Link   []Link `xml:"link"`
	Title  string `xml:"title"`
	Author Person `xml:"author"`
}

// Link defines a link within an Atom feed
type Link struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

// Person defines a person responsible for the tweet
type Person struct {
	Name string `xml:"name"`
}

// Text defines the text of the tweet
type Text struct {
	Type string `xml:",attr"`
	Body string `xml:",chardata"`
}

var (
	nresults = flag.Int("n", 100, "Maximum results (up to 100)")
	since    = flag.String("d", "", "Search since this date (YYYY-MM-DD)")
)

const (
	queryURI = "http://search.twitter.com/search.atom?q=%s&since=%s&rpp=%d"
	textfmt  = "font-family:Calibri,Lucida,sans;fill:gray;text-anchor:middle;font-size:48px"
	imw      = 48
	imh      = 48
)

// ts dereferences the twitter search URL and reads the XML (Atom) response
func ts(s string, date string, n int) {

	r, err := http.Get(fmt.Sprintf(queryURI, url.QueryEscape(s), date, n))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}
	if r.StatusCode == http.StatusOK {
		readatom(r.Body)
	} else {
		fmt.Fprintf(os.Stderr,
			"Twitter is unable to search for %s (%s)\n", s, r.Status)
	}
	r.Body.Close()
}

// readatom unmarshals the twitter search response and formats the results into a grid
func readatom(r io.Reader) {
	var twitter Feed
	err := xml.NewDecoder(r).Decode(&twitter)
	if err == nil {
		tgrid(twitter, 25, 25, 50, 50, 10)
	} else {
		fmt.Fprintf(os.Stderr, "Unable to parse the Atom feed (%v)\n", err)
	}
}

// tgrid makes a clickable grid of tweets from the Atom feed
func tgrid(t Feed, x, y, w, h, nc int) {
	var slink, imlink string
	xp := x
	for i, entry := range t.Entry {
		for _, link := range entry.Link {
			switch link.Rel {
			case "alternate":
				slink = link.Href
			case "image":
				imlink = link.Href
			}
		}
		if i%nc == 0 && i > 0 {
			xp = x
			y += h
		}
		canvas.Link(slink, slink)
		canvas.Image(xp, y, imw, imh, imlink)
		canvas.LinkEnd()
		xp += w
	}
}

// for every non-flag argument, make a twitter search grid
func main() {
	flag.Parse()
	width := 550
	height := 700
	canvas.Start(width, height)
	for _, s := range flag.Args() {
		canvas.Title("Twitter search for " + s)
		ts(s, *since, *nresults)
		canvas.Text(width/2, height-50, s, textfmt)
	}
	canvas.End()
}
