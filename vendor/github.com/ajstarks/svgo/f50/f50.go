// f50 -- given a search term, display 10x5 image grid, sorted by interestingness
// +build !appengine

package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/ajstarks/svgo"
)

// FlickrResp defines the Flickr response
type FlickrResp struct {
	Stat   string `xml:"stat,attr"`
	Photos Photos `xml:"photos"`
}

// Photos defines a set of Flickr photos
type Photos struct {
	Page    string  `xml:"page,attr"`
	Pages   string  `xml:"pages,attr"`
	Perpage string  `xml:"perpage,attr"`
	Total   string  `xml:"total,attr"`
	Photo   []Photo `xml:"photo"`
}

// Photo defines a Flickr photo
type Photo struct {
	Id       string `xml:"id,attr"`
	Owner    string `xml:"owner,attr"`
	Secret   string `xml:"secret,attr"`
	Server   string `xml:"server,attr"`
	Farm     string `xml:"farm,attr"`
	Title    string `xml:"title,attr"`
	Ispublic string `xml:"ispublic,attr"`
	Isfriend string `xml:"isfriend,attr"`
	IsFamily string `xml:"isfamily,attr"`
}

var (
	width  = 805
	height = 500
	canvas = svg.New(os.Stdout)
)

const (
	apifmt      = "https://api.flickr.com/services/rest/?method=%s&api_key=%s&%s=%s&per_page=50&sort=interestingness-desc"
	urifmt      = "http://farm%s.static.flickr.com/%s/%s.jpg"
	apiKey      = "YOURKEY"
	textStyle   = "font-family:Calibri,sans-serif; font-size:48px; fill:white; text-anchor:start"
	imageWidth  = 75
	imageHeight = 75
)

// FlickrAPI calls the API given a method with single name/value pair
func flickrAPI(method, name, value string) string {
	return fmt.Sprintf(apifmt, method, apiKey, name, value)
}

// makeURI converts the elements of a photo into a Flickr photo URI
func makeURI(p Photo, imsize string) string {
	im := p.Id + "_" + p.Secret

	if len(imsize) > 0 {
		im += "_" + imsize
	}
	return fmt.Sprintf(urifmt, p.Farm, p.Server, im)
}

// imageGrid reads the response from Flickr, and creates a grid of images
func imageGrid(f FlickrResp, x, y, cols, gutter int, imgsize string) {
	if f.Stat != "ok" {
		fmt.Fprintf(os.Stderr, "Status: %v\n", f.Stat)
		return
	}
	xpos := x
	for i, p := range f.Photos.Photo {
		if i%cols == 0 && i > 0 {
			xpos = x
			y += (imageHeight + gutter)
		}
		canvas.Link(makeURI(p, ""), p.Title)
		canvas.Image(xpos, y, imageWidth, imageHeight, makeURI(p, "s"))
		canvas.LinkEnd()
		xpos += (imageWidth + gutter)
	}
}

// fs calls the Flickr API to perform a photo search
func fs(s string) {
	var f FlickrResp
	r, weberr := http.Get(flickrAPI("flickr.photos.search", "text", s))
	if weberr != nil {
		fmt.Fprintf(os.Stderr, "%v\n", weberr)
		return
	}
	defer r.Body.Close()
	xmlerr := xml.NewDecoder(r.Body).Decode(&f)
	if xmlerr != nil || r.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "%v (status=%d)\n", xmlerr, r.StatusCode)
		return
	}
	canvas.Title(s)
	imageGrid(f, 5, 5, 10, 5, "s")
	canvas.Text(20, height-40, s, textStyle)
}

// for each search term on the commandline, create a photo grid
func main() {
	for i := 1; i < len(os.Args); i++ {
		canvas.Start(width, height)
		canvas.Rect(0, 0, width, height, "fill:black")
		fs(url.QueryEscape(os.Args[i]))
		canvas.End()
	}
}
