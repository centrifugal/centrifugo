// shotchart: make NBA shotcharts
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/ajstarks/svgo"
)

// shotdata defines the shotchart JSON response from stats.nba.com
type shotdata struct {
	Resource   string `json:"resource"`
	Parameters struct {
		Leagueid       string `json:"LeagueID"`
		Season         string `json:"Season"`
		Seasontype     string `json:"SeasonType"`
		Teamid         int    `json:"TeamID"`
		Playerid       int    `json:"PlayerID"`
		Contextfilter  string `json:"ContextFilter"`
		Contextmeasure string `json:"ContextMeasure"`
	} `json:"parameters"`
	Resultsets []struct {
		Name    string          `json:"name"`
		Headers []string        `json:"headers"`
		Rowset  [][]interface{} `json:"rowSet"`
	} `json:"resultSets"`
}

// vmap maps one range into another
func vmap(value, low1, high1, low2, high2 float64) float64 {
	return low2 + (high2-low2)*(value-low1)/(high1-low1)
}

const (
	shotsURLfmt     = "http://stats.nba.com/stats/shotchartdetail?PlayerID=%s&CFID=33&CFPARAMS=2014-15&ContextFilter=&ContextMeasure=FGA&DateFrom=&DateTo=&GameID=&GameSegment=&LastNGames=0&LeagueID=00&Location=&MeasureType=Base&Month=0&OpponentTeamID=0&Outcome=&PaceAdjust=N&PerMode=PerGame&Period=0&PlusMinus=N&Position=&Rank=N&RookieYear=&Season=2014-15&SeasonSegment=&SeasonType=Regular+Season&TeamID=0&VsConference=&VsDivision=&mode=Advanced&showDetails=0&showShots=1&showZones=0"
	picURLfmt       = "http://stats.nba.com/media/players/230x185/%s.png"
	activepicURLfmt = "http://stats.nba.com/media/players/700/%s.png"
)

// shotAPI retrieves shotchart data from the source (currently stats.nba.com)
func shotAPI(id string) (io.ReadCloser, error) {
	resp, err := http.Get(fmt.Sprintf(shotsURLfmt, id))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to retreive network data for %s (%s)", id, resp.Status)
	}
	return resp.Body, nil
}

// shotchart retrieves shot data given an id, either from local files or the network API
func shotchart(id string, network bool) {
	var (
		shots   shotdata
		r       io.ReadCloser
		err     error
		picture string
	)

	if network {
		r, err = shotAPI(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return
		}
		picture = fmt.Sprintf(activepicURLfmt, id)
	} else {
		r, err = os.Open(id + ".json")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return
		}
		picture = id + ".png"
	}

	defer r.Close()
	err = json.NewDecoder(r).Decode(&shots)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	// define the canvas,  read and parse the response
	canvas := svg.New(os.Stdout)
	width, height := 900, 846
	fw, fh := float64(width), float64(height)
	//imw, imh := 230, 185
	imw, imh := 700, 440
	top := fw / 10
	canvas.Start(width, height)
	canvas.Rect(0, 0, width, height, "fill:white;stroke:black;stroke-width:2")
	//canvas.Image(width-imw, height-imh, imw, imh, picture)
	canvas.Image(width/2, height-imh, imw, imh, picture)
	canvas.Gstyle("font-family:Calibri,sans-serif;font-size:24px")
	nfg := 0
	for _, r := range shots.Resultsets {
		if r.Name == "Shot_Chart_Detail" {
			var playername string
			attempts := len(r.Rowset)
			for _, rs := range r.Rowset {
				var x, y float64
				var fill string
				for i, v := range rs {
					if i == 4 {
						playername = v.(string)
					}
					if i == 17 {
						x = v.(float64)
					}
					if i == 18 {
						y = v.(float64)
					}
					if i == 20 {
						xp := int(vmap(x, -300, 300, 0, fw))
						yp := int(vmap(y, -300, 300, top, fh))
						if v.(float64) == 0 {
							fill = "red"
						} else {
							nfg++
							fill = "black"
						}
						canvas.Circle(xp, (yp-(height/2))+10, 4, "fill-opacity:0.3;fill:"+fill)
					}
				}
			}
			fgpct := (float64(nfg) / float64(attempts)) * 100
			canvas.Text(10, height-40, playername, "fill:gray")
			canvas.Text(10, height-10, fmt.Sprintf("%d out of %d", attempts, nfg), "fill:gray")
			canvas.Text(width/2, height-10, fmt.Sprintf("%.1f%%", fgpct), "text-anchor:middle;font-size:120%;fill:gray")
			canvas.Gend()
		}
	}
	canvas.End()

}
func main() {
	var network bool
	flag.BoolVar(&network, "net", false, "retrieve data from the network")
	flag.Parse()

	for _, file := range flag.Args() {
		shotchart(file, network)
	}
}
