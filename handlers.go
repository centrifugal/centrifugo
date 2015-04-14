package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"gopkg.in/centrifugal/sockjs-go.v2/sockjs"
)

func newClientConnectionHandler(app *application) http.Handler {
	return sockjs.NewHandler("/connection", sockjs.DefaultOptions, app.clientConnectionHandler)
}

func (app *application) clientConnectionHandler(session sockjs.Session) {
	log.Println("new sockjs session established")
	var closedSession = make(chan struct{})
	defer func() {
		close(closedSession)
		log.Println("sockjs session closed")
	}()

	client, err := newClient(app, session, closedSession)
	if err != nil {
		log.Println(err)
		return
	}

	tick := time.Tick(20 * time.Second)

	go func() {
		for {
			select {
			case <-closedSession:
				return
			case <-tick:
				client.printIsAuthenticated()
			}
		}
	}()

	for {
		if msg, err := session.Recv(); err == nil {
			log.Println(msg)
			err = client.handleMessage(msg)
			if err != nil {
				log.Println(err)
				session.Close(3000, err.Error())
			}
			continue
		}
		break
	}
}

func (app *application) apiHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "%s\n", ps.ByName("projectKey"))
}

func (app *application) authHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "auth\n")
}

func (app *application) infoHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	info := map[string]interface{}{
		"version":   VERSION,
		"structure": app.structure,
		"engine":    app.engine,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (app *application) actionsHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "actions\n")
}
