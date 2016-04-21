// picserv: serve pictures
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ajstarks/svgo"
)

var listen = flag.String("listen", ":1958", "http service address")

const (
	arcstyle  = "stroke:red;stroke-linecap:round;fill:none;stroke-width:10"
	rotextfmt = "fill:%s;font-family:%s;font-size:%dpt"
	flowerfmt = "stroke:rgb(%d,%d,%d); stroke-opacity:%.2f; stroke-width:%d"
	tilestyle = "stroke-width:1; stroke:rgb(128,128,128); stroke-opacity:0.5; fill:white"
	penstyle  = "stroke:rgb%s; fill:none; stroke-opacity:%.2f; stroke-width:%d"
	width     = 256
	height    = 256
)

// include index
//go:generate ih -v index -o index.go pic256.html

// init seeds the RNG
func init() {
	rand.Seed(time.Now().Unix() % 1e9)
}

// serve stuff
func main() {
	flag.Parse()
	http.Handle("/", http.HandlerFunc(picindex))
	http.Handle("/index/", http.HandlerFunc(picindex))
	http.Handle("/pic256.html", http.HandlerFunc(picindex))
	http.Handle("/rotext/", http.HandlerFunc(rotext))
	http.Handle("/rshape/", http.HandlerFunc(rshape))
	http.Handle("/face/", http.HandlerFunc(face))
	http.Handle("/flower/", http.HandlerFunc(flower))
	http.Handle("/cube/", http.HandlerFunc(cube))
	http.Handle("/lewitt/", http.HandlerFunc(lewitt))
	http.Handle("/mondrian/", http.HandlerFunc(mondrian))
	http.Handle("/funnel/", http.HandlerFunc(funnel))
	http.Handle("/clock/", http.HandlerFunc(clock))
	http.Handle("/pacman/", http.HandlerFunc(pacman))
	http.Handle("/ubuntu/", http.HandlerFunc(ubuntu))
	http.Handle("/tux/", http.HandlerFunc(tux))
	log.Printf("listen on %s", *listen)
	err := http.ListenAndServe(*listen, nil)
	if err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

// qstring returns the string value of the query string
func qstring(q url.Values, key, defval string, length int) string {
	var retval string
	p, ok := q[key]
	if ok {
		retval = p[0]
	} else {
		return defval
	}
	if len(retval) > length {
		return retval[:length]
	}
	return retval
}

// qfloat returns the float64 value of a query string, within limits
func qfloat(q url.Values, key string, defval float64, min, max float64) float64 {
	var retval float64
	var err error
	p, ok := q[key]
	if ok {
		retval, err = strconv.ParseFloat(p[0], 64)
		if err != nil {
			return defval
		}
	} else {
		return defval
	}
	if retval < min || retval > max {
		return defval
	}
	return retval
}

// qfint returns the integer value of a query string, within limits
func qint(q url.Values, key string, defval int, min, max int) int {
	var retval int
	var err error
	p, ok := q[key]
	if ok {
		retval, err = strconv.Atoi(p[0])
		if err != nil {
			return defval
		}
	} else {
		return defval
	}
	if retval < min || retval > max {
		return defval
	}
	return retval
}

// qbool returns the boolean value of a query string
func qbool(q url.Values, key string, defval bool) bool {
	p, ok := q[key]
	if ok {
		switch p[0] {
		case "t", "true", "T", "1", "on":
			return true
		case "f", "false", "F", "0", "off":
			return false
		default:
			return defval
		}
	} else {
		return defval
	}
}

func random(howsmall, howbig int) int {
	if howsmall >= howbig {
		return howsmall
	}
	return rand.Intn(howbig-howsmall) + howsmall
}

func randcolor() string {
	return fmt.Sprintf("fill:rgb(%d,%d,%d)", rand.Intn(255), rand.Intn(255), rand.Intn(255))
}

// picindex shows an HTML document that describes the service
// The "index" variable is a string that holds the document,
// made with go generate
func picindex(w http.ResponseWriter, req *http.Request) {
	log.Printf("index: %s %s %s", req.RemoteAddr, req.URL.Path, req.UserAgent())
	io.WriteString(w, index)
}

// rotext makes rotated and faded text
func rotext(w http.ResponseWriter, req *http.Request) {

	log.Printf("rotext: %s", req.RemoteAddr)
	query := req.URL.Query()

	rchar := qstring(query, "char", "a", 3)     // the string
	ti := qfloat(query, "ti", 10, 5, 360)       // angle interval
	bg := qstring(query, "bg", "black", 20)     // background color
	fg := qstring(query, "fg", "white", 20)     // text color
	font := qstring(query, "font", "serif", 50) // font name
	a, ai := 1.0, 0.03

	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Title("Rotated Text")
	canvas.Rect(0, 0, width, height, "fill:"+bg)
	canvas.Gstyle(fmt.Sprintf(rotextfmt, fg, font, width/(len(rchar)+1)))
	for t := 0.0; t <= 360.0; t += ti {
		canvas.TranslateRotate(width/2, height/2, t)
		canvas.Text(0, 0, rchar, fmt.Sprintf("fill-opacity:%.2f", a))
		canvas.Gend()
		a -= ai
	}
	canvas.Gend()
	canvas.End()
}

// face draws a face, with mood (happy, sad, neutral),
// and glance (up, down, left, right, middle)
func face(w http.ResponseWriter, req *http.Request) {

	log.Printf("face: %s", req.RemoteAddr)
	query := req.URL.Query()

	mood := qstring(query, "mood", "h", 10)
	glance := qstring(query, "glance", "m", 10)
	ex1 := width / 4       // left eye x 25% from the left
	ex2 := (width * 3) / 4 // right eye x 25% from the right
	ey := height / 3       // eye y one third from the bottom
	sy := (height * 2) / 3 // mouth y two-thirds from the bottom
	er := width / 12       // eye radius
	ax := height / 3       // mouth arc x
	ay := height / 3       // mounth arc y
	aflag := false
	pupilsize := er / 3
	xoffset := 0
	yoffset := 0

	// adjust mouth according to mood
	switch mood {
	case "n", "neutral":
		ay = 0
	case "s", "sad":
		sy = (height * 4) / 5
		aflag = true
	}

	// adjust pupils according to glance
	switch glance {
	case "l", "left":
		xoffset = -pupilsize
	case "r", "right":
		xoffset = pupilsize
	case "d", "down":
		yoffset = pupilsize
	case "u", "up":
		yoffset = -pupilsize
	}

	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Title("Face")
	canvas.Rect(0, 0, width, height, "fill:white")                  // background
	canvas.Circle(ex1, ey, er)                                      // lefteye
	canvas.Circle(ex2, ey, er)                                      // righteye
	canvas.Circle(ex1+xoffset, ey+yoffset, pupilsize, "fill:white") // left pupil
	canvas.Circle(ex2+xoffset, ey+yoffset, pupilsize, "fill:white") // right pupil
	canvas.Arc(ex1, sy, ax, ay, 0, false, aflag, ex2, sy, arcstyle) // mouth
	canvas.End()
}

// rshape draws random shapes
func rshape(w http.ResponseWriter, req *http.Request) {

	log.Printf("rshape: %s", req.RemoteAddr)
	query := req.URL.Query()

	n := qint(query, "n", 150, 5, 200)        // number of shapes
	shape := qstring(query, "shape", "c", 10) // type of shape
	bg := qstring(query, "bg", "white", 20)   // background color
	samesize := qbool(query, "same", false)   // regular or oblong
	canvas := svg.New(w)

	// draw rect, square, ellipse or circle according to the specified shape
	shapefunc := canvas.Ellipse
	switch shape {
	case "r", "box":
		shapefunc = canvas.Rect
		samesize = false
	case "s", "sq", "square":
		shapefunc = canvas.Rect
		samesize = true
	case "e", "ellipse":
		shapefunc = canvas.Ellipse
		samesize = false
	case "c", "circle", "dot":
		shapefunc = canvas.Ellipse
		samesize = true
	}

	w.Header().Set("Content-type", "image/svg+xml")
	var s1, s2 int
	canvas.Start(width, height)
	canvas.Title("Random Shapes")
	canvas.Rect(0, 0, width, height, "fill:"+bg)
	for i := 0; i < n; i++ {
		s1 = rand.Intn(width / 5)
		if samesize {
			s2 = s1
		} else {
			s2 = rand.Intn(height / 5)
		}
		shapefunc(rand.Intn(width), rand.Intn(height), s1, s2,
			fmt.Sprintf("fill-opacity:%.2f;fill:rgb(%d,%d,%d)",
				rand.Float64(), rand.Intn(256), rand.Intn(256), rand.Intn(256)))
	}
	canvas.End()
}

func flower(w http.ResponseWriter, req *http.Request) {

	log.Printf("flower: %s", req.RemoteAddr)
	query := req.URL.Query()

	n := qint(query, "n", 200, 10, 200)          // number of "flowers"
	np := qint(query, "petals", 15, 10, 60)      // number of "petals" per flower
	opacity := qint(query, "op", 50, 20, 100)    // opacity
	psize := qint(query, "size", 30, 5, 50)      // length of the petals
	thickness := qint(query, "thick", 10, 3, 20) // petal thickness

	limit := 2.0 * math.Pi

	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Title("Flowers")
	canvas.Rect(0, 0, width, height, "fill:white")

	for i := 0; i < n; i++ {
		x := rand.Intn(width)
		y := rand.Intn(height)
		r := float64(random(10, psize))

		canvas.Gstyle(fmt.Sprintf(flowerfmt, rand.Intn(255), rand.Intn(255), rand.Intn(255),
			float64(random(10, opacity))/100.0, random(2, thickness)))
		for theta := 0.0; theta < limit; theta += limit / float64(random(10, np)) {
			xr := r * math.Cos(theta)
			yr := r * math.Sin(theta)
			canvas.Line(x, y, x+int(xr), y+int(yr))
		}
		canvas.Gend()
	}
	canvas.End()
}

// rcube makes a cube with three visible faces, each with a random color
func rcube(canvas *svg.SVG, x, y, l int) {

	// top face
	tx := []int{x, x + (l * 3), x, x - (l * 3), x}
	ty := []int{y, y + (l * 2), y + (l * 4), y + (l * 2), y}

	// left face
	lx := []int{x - (l * 3), x, x, x - (l * 3), x - (l * 3)}
	ly := []int{y + (l * 2), y + (l * 4), y + (l * 8), y + (l * 6), y + (l * 2)}

	// right face
	rx := []int{x + (l * 3), x + (l * 3), x, x, x + (l * 3)}
	ry := []int{y + (l * 2), y + (l * 6), y + (l * 8), y + (l * 4), y + (l * 2)}

	canvas.Polygon(tx, ty, randcolor())
	canvas.Polygon(lx, ly, randcolor())
	canvas.Polygon(rx, ry, randcolor())
}

// cube draws a grid of cubes, n rows deep.
// The grid begins at (xp, yp), with hspace between cubes in a row, and vspace between rows.
func cube(w http.ResponseWriter, req *http.Request) {

	log.Printf("cube: %s", req.RemoteAddr)
	query := req.URL.Query()

	bgcolor := qstring(query, "bg", randcolor(), 30)  // background color
	n := qint(query, "row", 3, 1, 20)                 // number of rows
	hspace := qint(query, "hs", width/5, 0, width)    // horizontal space
	vspace := qint(query, "vs", height/4, 0, height)  // vertical space
	size := qint(query, "size", width/30, 2, width/4) // cube size
	xp := qint(query, "x", width/10, 0, width/2)      // initial x position
	yp := qint(query, "y", height/10, 0, height/2)    // initial y position

	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Title("Cubes")
	canvas.Rect(0, 0, width, height, bgcolor)
	y := yp
	for r := 0; r < n; r++ {
		for x := xp; x < width; x += hspace {
			rcube(canvas, x, y, size)
		}
		y += vspace
	}
	canvas.End()
}

var pencils = []string{"(250, 13, 44)", "(247, 212, 70)", "(52, 114, 245)"}

func lew(canvas *svg.SVG, x int, y int, gsize int, n int, w int) {
	var x1, x2, y1, y2 int
	var op float64
	canvas.Rect(x, y, gsize, gsize, tilestyle)
	for i := 0; i < n; i++ {
		choice := rand.Intn(len(pencils))
		op = float64(random(1, 10)) / 10.0
		x1 = random(x, x+gsize)
		y1 = random(y, y+gsize)
		x2 = random(x, x+gsize)
		y2 = random(y, y+gsize)
		if random(0, 100) > 50 {
			canvas.Line(x1, y1, x2, y2, fmt.Sprintf(penstyle, pencils[choice], op, random(1, w)))
		} else {
			canvas.Arc(x1, y1, gsize, gsize, 0, false, true, x2, y2, fmt.Sprintf(penstyle, pencils[choice], op, random(1, w)))
		}
	}
}

// lewitt simulates Sol Lewitt's Wall Drawing 91
func lewitt(w http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()
	log.Printf("lewitt: %s", req.RemoteAddr)

	nlines := qint(query, "lines", 20, 5, 100)
	nw := qint(query, "pen", 3, 1, 5)

	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Title("Sol Lewitt's Wall Drawing 91")
	canvas.Rect(0, 0, width, height, "fill:white")
	gsize := width / 6
	nc := width / gsize
	nr := height / gsize
	for cols := 0; cols < nc; cols++ {
		for rows := 0; rows < nr; rows++ {
			lew(canvas, cols*gsize, rows*gsize, gsize, nlines, nw)
		}
	}
	canvas.End()
}

// pmcolor returns a random color from Mondrian's set, or a specified standard color
func pmcolor(randcolor bool, standard string) string {
	moncolors := []string{"white", "red", "blue", "yellow"}
	if randcolor {
		return moncolors[rand.Intn(10000)%4]
	}
	return standard
}

// mondrian draws a view inspired by Piet Mondrian's Composition red, blue, white and yellow
func mondrian(w http.ResponseWriter, req *http.Request) {
	log.Printf("mondrian: %s", req.RemoteAddr)
	query := req.URL.Query()
	rc := qbool(query, "random", false)
	w3 := width / 3
	w6 := w3 / 2
	w23 := w3 * 2

	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Title("Mondrian Composition in red, blue, white and yellow")
	canvas.Gstyle("stroke:black;stroke-width:6")
	canvas.Rect(0, 0, w3, w3, "fill:"+pmcolor(rc, "white"))
	canvas.Rect(0, w3, w3, w3, "fill:"+pmcolor(rc, "white"))
	canvas.Rect(0, w23, w3, w3, "fill:"+pmcolor(rc, "blue"))
	canvas.Rect(w3, 0, w23, w23, "fill:"+pmcolor(rc, "red"))
	canvas.Rect(w3, w23, w23, w3, "fill:"+pmcolor(rc, "white"))
	canvas.Rect(width-w6, height-w3, w3-w6, w6, "fill:"+pmcolor(rc, "white"))
	canvas.Rect(width-w6, height-w6, w3-w6, w6, "fill:"+pmcolor(rc, "yellow"))
	canvas.Gend()
	canvas.Rect(0, 0, width, height, "fill:none;stroke:black;stroke-width:12")
	canvas.End()
}

// funnel makes a funnel from fading ellipses
func funnel(w http.ResponseWriter, req *http.Request) {
	log.Printf("funnel: %s", req.RemoteAddr)
	query := req.URL.Query()
	bg := qstring(query, "bg", "black", 20)
	fg := qstring(query, "fg", "white", 20)
	grid := qint(query, "step", 25, 10, height/3)
	h := width / 2

	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Title("Funnel")
	canvas.Rect(0, 0, width, height, "fill:"+bg)
	canvas.Gstyle("fill-opacity:0.2;fill:" + fg)
	for size := grid; size < width; size += grid {
		canvas.Ellipse(h, size, size/2, size/2)
	}
	canvas.Gend()
	canvas.End()
}

var (
	digits    = [12]string{"12", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}
	hrangles  = [12]float64{90, 60, 30, 0, 330, 300, 270, 240, 210, 180, 150, 120}
	minangles = [60]float64{
		90, 84, 78, 72, 66, 60, 54, 48, 42, 36, 30, 24, 18, 12, 6,
		0, 354, 348, 342, 336, 330, 324, 318, 312, 306,
		300, 294, 288, 282, 276, 270, 264, 258, 252, 246, 240, 234, 228, 222, 216,
		210, 204, 198, 192, 186, 180, 174, 168, 162, 156,
		150, 144, 138, 132, 126, 120, 114, 108, 102, 96,
	}
)

const (
	radians  = math.Pi / 180.0
	hrcolor  = "rgb(127,0,0)"
	secolor  = "rgb(0,0,255)"
	mincolor = "rgb(127,127,127)"
	bgcolor  = "rgb(140,140,140)"
	linefmt  = "stroke:%s;stroke-width:%d"
	digitfmt = "font-family:Helvetica,Calibri,sans-serif;text-anchor:middle;font-size:%dpx"
)

// clock draws an analog clock
func clock(w http.ResponseWriter, req *http.Request) {
	log.Printf("clock: %s", req.RemoteAddr)
	query := req.URL.Query()
	size := width / 3
	basesize := size / 12
	fs := (size * 2) + (size / 2)

	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	canvas.Start(width, height)

	// clock face
	cx, cy := width/2, height/2
	now := time.Now()
	hour, min, sec := now.Hour(), now.Minute(), now.Second()

	hour = qint(query, "hour", hour, 0, 23)
	min = qint(query, "min", min, 0, 59)
	sec = qint(query, "sec", sec, 0, 59)

	canvas.Rect(0, 0, width, height, "fill:black")
	canvas.Roundrect(cx-(fs/2), cy-(fs/2), fs, fs, basesize, basesize, "fill:"+bgcolor)
	canvas.Circle(cx, cy, size+(size/6), "fill:white")
	canvas.Gstyle(fmt.Sprintf(digitfmt, basesize*2))

	// draw the clock-face digits
	r := float64(size)
	rx := float64(cx)
	ry := float64(cy)
	for h := 12; h > 0; h-- {
		t := hrangles[h%12] * radians
		x := rx + r*math.Cos(t)
		y := ry + r*math.Sin(t)
		canvas.Text(int(x), height-int(y), digits[h%12], "baseline-shift:-30%")
	}
	canvas.Gend()

	// hour hand: special case: if the minute is greater than 30,
	// adjust the hour hand the move proportionally closer to the upcoming hour.
	t := hrangles[hour%12]
	if min > 30 {
		t = t - (30.0 * (float64(min) / 60.0))
	}
	hr := r * 0.6
	hx := rx + hr*math.Cos(t*radians)
	hy := ry + hr*math.Sin(t*radians)

	// minute hand
	mr := r * 0.9
	t = minangles[min] * radians
	mx := rx + mr*math.Cos(t)
	my := ry + mr*math.Sin(t)

	// second hand
	sr := r
	t = minangles[sec] * radians
	sx := rx + sr*math.Cos(t)
	sy := ry + sr*math.Sin(t)

	// draw the hands and center dot
	canvas.Line(cx, cy, int(hx), height-int(hy), fmt.Sprintf(linefmt, hrcolor, basesize))
	canvas.Line(cx, cy, int(mx), height-int(my), fmt.Sprintf(linefmt, mincolor, basesize/2))
	canvas.Line(cx, cy, int(sx), height-int(sy), fmt.Sprintf(linefmt, secolor, basesize/4))
	canvas.Circle(cx, cy, basesize, "fill:black")
	canvas.End()
}

// pacman draws pacman with dots
func pacman(w http.ResponseWriter, req *http.Request) {
	log.Printf("pacman: %s", req.RemoteAddr)
	query := req.URL.Query()
	angle := qfloat(query, "angle", 30.0, 10.0, 70.0)

	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	cx, cy := width/2, height/2
	r := width / 5
	p := r / 8
	canvas.Start(width, height)
	canvas.Rect(0, 0, width, height)

	// draw dots
	canvas.Gstyle("fill:white")
	for x := 0; x < 100; x += 12 {
		if x < 50 {
			canvas.Circle((width*x)/100, cy, p, "fill-opacity:0.5")
		} else {
			canvas.Circle((width*x)/100, cy, p, "fill-opacity:1")
		}
	}
	canvas.Gend()

	// draw pacman: two arcs, rotated,
	// the angle determines how wide the mouth is open
	canvas.Gstyle("fill:yellow")

	canvas.TranslateRotate(cx, cy, -angle)
	canvas.Arc(-r, 0, r, r, 30, false, true, r, 0)
	canvas.Gend()

	canvas.TranslateRotate(cx, cy, angle)
	canvas.Arc(-r, 0, r, r, 30, false, false, r, 0)
	canvas.Gend()

	canvas.Gend()
	canvas.End()
}
func tux(w http.ResponseWriter, req *http.Request) {
	log.Printf("tux: %s", req.RemoteAddr)
	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Rect(0, 0, width, height, "fill:white")
	canvas.Circle(width/2, height/2, height/2-20, "fill:black")
	canvas.Ellipse(width/2, height, width/2, height/2, "fill:black")
	canvas.Ellipse(width/3, height/2, 20, 40, "fill:white")
	canvas.Ellipse(2*width/3, height/2, 20, 40, "fill:white")
	canvas.Ellipse(width/3, height/2+18, 15, 20)
	canvas.Ellipse(2*width/3, height/2+18, 15, 20)

	canvas.Circle(width/3+7, height/2+20, 5, "fill:white")
	canvas.Circle(2*width/3+7, height/2+20, 5, "fill:white")

	canvas.Arc(60, height-60, width/3, 50, 0, false, true, width-60, height-60,
		"stroke-width:3;stroke-linecap:round;stroke:yellow;fill:yellow")

	canvas.Arc(60, height-60, width/3, 140, 0, false, false, width-60, height-60,
		"stroke-width:3;stroke-linecap:round;stroke:yellow")

	beakx := []int{58, width - 58, width / 2}
	beaky := []int{height - 62, height - 62, height - 20}
	canvas.Polygon(beakx, beaky, "fill:yellow")

	canvas.End()
}

const (
	d2r    = math.Pi / 180
	ustyle = "stroke:#DD4814;stroke-width:8"
)

func polar(cx, cy, r, t int) (int, int) {
	fr := float64(r)
	ft := float64(t) * d2r
	x := fr * math.Cos(ft)
	y := fr * math.Sin(ft)
	return cx + int(x), cy + int(y)
}

func ubuntu(w http.ResponseWriter, req *http.Request) {
	log.Printf("ubuntu: %s", req.RemoteAddr)
	w.Header().Set("Content-type", "image/svg+xml")
	canvas := svg.New(w)
	cx, cy := width/2, height/2
	canvas.Start(width, height)
	canvas.Rect(0, 0, width, height, "fill:white")
	canvas.Circle(cx, cy, cx, "fill:#DD4814")
	r := width / 3

	canvas.Circle(cx, cy, r-10, "fill:none;stroke:white;stroke-width:25")
	canvas.Gstyle("fill:white;" + ustyle)
	for _, t := range []int{300, 180, 60} {
		px, py := polar(cx, cy, r+10, t)
		canvas.Circle(px, py, 20)
	}
	canvas.Gend()

	canvas.Gstyle(ustyle)
	for _, t := range []int{120, 0, 240} {
		lx2, ly2 := polar(cx, cy, r+25, t)
		canvas.Line(cx, cy, lx2, ly2)
	}
	canvas.Gend()

	canvas.End()
}
