package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"centrifugo/sockjs"
	"github.com/julienschmidt/httprouter"
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

	client, err := newClient(session, closedSession)
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
			}
			continue
		}
		break
	}
}

func (app *application) apiHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "%s\n", ps.ByName("projectKey"))
}
