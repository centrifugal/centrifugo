package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/logger"

	"github.com/julienschmidt/httprouter"
	"gopkg.in/centrifugal/sockjs-go.v2/sockjs"
)

func newClientConnectionHandler(app *application) http.Handler {
	return sockjs.NewHandler("/connection", sockjs.DefaultOptions, app.clientConnectionHandler)
}

func (app *application) clientConnectionHandler(session sockjs.Session) {
	logger.INFO.Println("new client session established")
	var closedSession = make(chan struct{})
	defer func() {
		close(closedSession)
		logger.INFO.Println("client session closed")
	}()

	client, err := newClient(app, session, closedSession)
	if err != nil {
		logger.ERROR.Println(err)
		return
	}

	go func() {
		for {
			select {
			case <-closedSession:
				err = client.clean()
				if err != nil {
					logger.ERROR.Println(err)
				}
				return
			}
		}
	}()

	for {
		if msg, err := session.Recv(); err == nil {
			err = client.handleMessage(msg)
			if err != nil {
				logger.ERROR.Println(err)
				session.Close(3000, err.Error())
				break
			}
			continue
		}
		break
	}
}

func getCommandsFromApiMessage(msgBytes []byte, msgType string) ([]apiCommand, error) {
	var commands []apiCommand
	switch msgType {
	case "map":
		// single command request
		var command apiCommand
		err := json.Unmarshal(msgBytes, &command)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	case "array":
		// array of commands received
		err := json.Unmarshal(msgBytes, &commands)
		if err != nil {
			return nil, err
		}
	}
	return commands, nil
}

type jsonApiRequest struct {
	Sign string
	Data string
}

func timeTrack(start time.Time, name string) {
	logger.DEBUG.Printf("%s %s\n", name, time.Since(start))
}

func (app *application) apiHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	defer timeTrack(time.Now(), "api call completed in")

	projectKey := ps.ByName("projectKey")
	contentType := r.Header.Get("Content-Type")

	var sign string
	var encodedData string

	if strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		// json request
		var req jsonApiRequest
		var decoder = json.NewDecoder(r.Body)
		err := decoder.Decode(&req)
		if err != nil {
			logger.ERROR.Println(err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		sign = req.Sign
		encodedData = req.Data
	} else {
		// application/x-www-form-urlencoded request
		sign = r.FormValue("sign")
		encodedData = r.FormValue("data")
	}

	if sign == "" {
		logger.ERROR.Println("no sign found in API request")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if encodedData == "" {
		logger.ERROR.Println("no data found in API request")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	project, exists := app.getProjectByKey(projectKey)
	if !exists {
		logger.ERROR.Println("no project found with key", projectKey)
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	secret := project.Secret

	isValid := checkApiSign(secret, projectKey, encodedData, sign)
	if !isValid {
		logger.ERROR.Println("invalid sign")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	msgBytes := []byte(encodedData)
	msgType, err := getMessageType(msgBytes)
	if err != nil {
		logger.ERROR.Println(err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	commands, err := getCommandsFromApiMessage(msgBytes, msgType)
	if err != nil {
		logger.ERROR.Println(err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var mr multiResponse

	for _, command := range commands {
		resp, err := app.handleApiCommand(project, command)
		if err != nil {
			logger.ERROR.Println(err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		mr = append(mr, resp)
	}
	jsonResp, err := mr.toJson()
	if err != nil {
		logger.ERROR.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

func (app *application) authHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "auth\n")
}

func (app *application) infoHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	info := map[string]interface{}{
		"version":   VERSION,
		"structure": app.structure.ProjectList,
		"engine":    app.engine,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (app *application) actionsHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	defer timeTrack(time.Now(), "api call")
	fmt.Fprintf(w, "actions\n")
}
