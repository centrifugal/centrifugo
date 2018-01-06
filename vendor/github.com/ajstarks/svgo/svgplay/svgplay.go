// svgplay: sketch with SVGo, (derived from the old misc/goplay), except:
// (1) only listen on localhost, (default port 1999)
// (2) always render html,
// (3) SVGo default code,
package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"text/template"
)

var (
	httpListen = flag.String("port", "1999", "port to listen on")
	useFloat = flag.Bool("f", false, "use the floating point version")
)

var (
	// a source of numbers, for naming temporary files
	uniq = make(chan int)
)

func main() {
	flag.Parse()

	// source of unique numbers
	go func() {
		for i := 0; ; i++ {
			uniq <- i
		}
	}()

	http.HandleFunc("/", FrontPage)
	http.HandleFunc("/compile", Compile)
	log.Printf("☠ ☠ ☠ Warning: this server allows a client connecting to 127.0.0.1:%s to execute code on this computer ☠ ☠ ☠", *httpListen)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+*httpListen, nil))
}

// FrontPage is an HTTP handler that renders the svgoplay interface.
// If a filename is supplied in the path component of the URI,
// its contents will be put in the interface's text area.
// Otherwise, the default "hello, world" program is displayed.
func FrontPage(w http.ResponseWriter, req *http.Request) {
	data, err := ioutil.ReadFile(req.URL.Path[1:])
	if err != nil {
		if (*useFloat) {
			data = helloWorldFloat
		} else {
			data = helloWorld
		}
	}
	frontPage.Execute(w, data)
}

// Compile is an HTTP handler that reads Go source code from the request,
// runs the program (returning any errors),
// and sends the program's output as the HTTP response.
func Compile(w http.ResponseWriter, req *http.Request) {
	out, err := compile(req)
	if err != nil {
		compileError(w, out, err)
		return
	}

	// write the output of target as the http response
	w.Write(out)
}

var (
	tmpdir, pkgdir, buildpid string
)

func init() {
	// find real temporary directory (for rewriting filename in output)
	var err error
	tmpdir, err = filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		log.Fatal(err)
	}

	buildpid = strconv.Itoa(os.Getpid())
}

func compile(req *http.Request) (out []byte, err error) {
	// target is the base name for .go, object, executable files
	target := filepath.Join(tmpdir, "svgplay"+buildpid+strconv.Itoa(<-uniq))
	src := target + ".go"

	// write body to target.go
	body := new(bytes.Buffer)
	if _, err = body.ReadFrom(req.Body); err != nil {
		return
	}
	defer os.Remove(src)
	if err = ioutil.WriteFile(src, body.Bytes(), 0666); err != nil {
		return
	}
	return run("", "go", "run", src)
}

// error writes compile, link, or runtime errors to the HTTP connection.
// The JavaScript interface uses the 404 status code to identify the error.
func compileError(w http.ResponseWriter, out []byte, err error) {
	w.WriteHeader(404)
	if out != nil {
		elines := bytes.Split(out, []byte{'\n'})
		for _, l := range elines {
			i := bytes.Index(l, []byte{':'})
			output.Execute(w, l[i+1:])
		}
	} else {
		output.Execute(w, err.Error())
	}
}

// run executes the specified command and returns its output and an error.
func run(dir string, args ...string) ([]byte, error) {
	var buf bytes.Buffer
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = &buf
	cmd.Stderr = cmd.Stdout
	err := cmd.Run()
	return buf.Bytes(), err
}

var frontPage = template.Must(template.New("frontPage").Parse(frontPageText)) // HTML template
var output = template.Must(template.New("output").Parse(outputText))          // HTML template

var outputText = `<pre>{{printf "%s" . |html}}</pre>`

var frontPageText = `<!doctype html>
<html>
<head>
<title>svgplay: SVGo sketching</title>
<style>
body {
	font-size: 12pt;
}
pre, textarea {
	font-family: Optima, Calibri, 'DejaVu Sans', sans-serif;
	font-size: 100%;
	line-height: 15pt;
}
.hints {
	font-size: 80%;
	font-family: sans-serif;
	color: #333333;
	font-style: italic;
	text-align: right;
}
#edit, #output, #errors { width: 100%; text-align: left; }
#edit { height: 560px; }
#errors { color: #c00; }
</style>
<script>

function insertTabs(n) {
	// find the selection start and end
	var cont  = document.getElementById("edit");
	var start = cont.selectionStart;
	var end   = cont.selectionEnd;
	// split the textarea content into two, and insert n tabs
	var v = cont.value;
	var u = v.substr(0, start);
	for (var i=0; i<n; i++) {
		u += "\t";
	}
	u += v.substr(end);
	// set revised content
	cont.value = u;
	// reset caret position after inserted tabs
	cont.selectionStart = start+n;
	cont.selectionEnd = start+n;
}

function autoindent(el) {
	var curpos = el.selectionStart;
	var tabs = 0;
	while (curpos > 0) {
		curpos--;
		if (el.value[curpos] == "\t") {
			tabs++;
		} else if (tabs > 0 || el.value[curpos] == "\n") {
			break;
		}
	}
	setTimeout(function() {
		insertTabs(tabs);
	}, 1);
}

function keyHandler(event) {
	var e = window.event || event;
	if (e.keyCode == 9) { // tab
		insertTabs(1);
		e.preventDefault();
		return false;
	}
	if (e.keyCode == 13) { // enter
		if (e.shiftKey) { // +shift
			compile(e.target);
			e.preventDefault();
			return false;
		} else {
			autoindent(e.target);
		}
	}
	return true;
}

var xmlreq;

function autocompile() {
	if(!document.getElementById("autocompile").checked) {
		return;
	}
	compile();
}

function compile() {
	var prog = document.getElementById("edit").value;
	var req = new XMLHttpRequest();
	xmlreq = req;
	req.onreadystatechange = compileUpdate;
	req.open("POST", "/compile", true);
	req.setRequestHeader("Content-Type", "text/plain; charset=utf-8");
	req.send(prog);	
}

function compileUpdate() {
	var req = xmlreq;
	if(!req || req.readyState != 4) {
		return;
	}
	if(req.status == 200) {
		document.getElementById("output").innerHTML = req.responseText;
		document.getElementById("errors").innerHTML = "";
	} else {
		document.getElementById("errors").innerHTML = req.responseText;
		document.getElementById("output").innerHTML = "";
	}
}
</script>
</head>
<body>
<table width="100%"><tr><td width="60%" valign="top">
<textarea autofocus="true" id="edit" spellcheck="false" onkeydown="keyHandler(event);" onkeyup="autocompile();">{{printf "%s" . |html}}</textarea>
<div class="hints">
(Shift-Enter to compile and run.)&nbsp;&nbsp;&nbsp;&nbsp;
<input type="checkbox" id="autocompile" value="checked" /> Compile and run after each keystroke
</div>
<td width="3%">
<td width="27%" align="right" valign="top">
<div id="output"></div>
</table>
<div id="errors"></div>
</body>
</html>
`

var helloWorld = []byte(`package main

import (
	"math/rand"
	"os"
	"time"
	"github.com/ajstarks/svgo"
)

func rn(n int) int { return rand.Intn(n) }

func main() {
	canvas := svg.New(os.Stdout)
	width := 500
	height := 500
	nstars := 250
	style := "font-size:48pt;fill:white;text-anchor:middle"
	
	rand.Seed(time.Now().Unix())
	canvas.Start(width, height)
	canvas.Rect(0,0,width,height)
	for i := 0; i < nstars; i++ {
		canvas.Circle(rn(width), rn(height), rn(3), "fill:white")
	}
	canvas.Circle(width/2, height, width/2, "fill:rgb(77, 117, 232)")
	canvas.Text(width/2, height*4/5, "hello, world", style)
	canvas.End()
}`)

var helloWorldFloat = []byte(`
package main

import (
	"math/rand"
	"os"
	"time"
	"github.com/ajstarks/svgo/float"
)

func rn(n float64) float64 { return rand.Float64() * n }

func main() {
	canvas := svg.New(os.Stdout)
	width := 500.0
	height := 500.0
	nstars := 250
	style := "font-size:48pt;fill:white;text-anchor:middle"
	
	rand.Seed(time.Now().Unix())
	canvas.Start(width, height)
	canvas.Rect(0,0,width,height)
	for i := 0; i < nstars; i++ {
		canvas.Circle(rn(width), rn(height), rn(3), "fill:white")
	}
	canvas.Circle(width/2, height, width/2, "fill:rgb(77, 117, 232)")
	canvas.Text(width/2, height*4/5, "hello, world", style)
	canvas.End()
}`)
