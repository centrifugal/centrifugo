package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/logger"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/websocket"
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

	client, err := newClient(app, session)
	if err != nil {
		logger.ERROR.Println(err)
		return
	}

	go client.sendMessages()

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

var (
	arrayJsonPrefix  byte = '['
	objectJsonPrefix byte = '{'
)

func getCommandsFromApiMessage(msgBytes []byte) ([]apiCommand, error) {
	var commands []apiCommand

	firstByte := msgBytes[0]

	switch firstByte {
	case objectJsonPrefix:
		// single command request
		var command apiCommand
		err := json.Unmarshal(msgBytes, &command)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	case arrayJsonPrefix:
		// array of commands received
		err := json.Unmarshal(msgBytes, &commands)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidApiMessage
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

	commands, err := getCommandsFromApiMessage(msgBytes)
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

const (
	tokenKey   = "token"
	tokenValue = "authorized"
)

func (app *application) authHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	password := r.FormValue("password")
	if password == app.password {
		w.Header().Set("Content-Type", "application/json")
		s := securecookie.New([]byte(app.secret), nil)
		token, err := s.Encode(tokenKey, tokenValue)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		resp := map[string]string{
			"token": token,
		}
		json.NewEncoder(w).Encode(resp)
		return
	}
	http.Error(w, "Bad Request", http.StatusBadRequest)
}

func (app *application) Authenticated(h httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Token ")
			s := securecookie.New([]byte(app.secret), nil)
			var val string
			if err := s.Decode(tokenKey, token, &val); err == nil {
				if val == tokenValue {
					h(w, r, ps)
					return
				}
			}
		}
		w.WriteHeader(401)
	}
}

func (app *application) infoHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	info := map[string]interface{}{
		"version":    VERSION,
		"structure":  app.structure.ProjectList,
		"engine":     app.engine.getName(),
		"node_name":  app.name,
		"node_count": len(app.nodes) + 1,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (app *application) actionHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	projectKey := r.FormValue("project")
	method := r.FormValue("method")

	project, exists := app.getProjectByKey(projectKey)
	if !exists {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var resp *response
	var err error

	switch method {
	case "publish":
		channel := r.FormValue("channel")
		data := r.FormValue("data")
		if data == "" {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		var decodedData interface{}
		err := json.Unmarshal([]byte(data), &decodedData)
		if err != nil {
			logger.ERROR.Println(err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		cmd := &publishApiCommand{
			Channel: channel,
			Data:    decodedData,
		}
		resp, err = app.handlePublishCommand(project, cmd)
	case "unsubscribe":
		channel := r.FormValue("channel")
		user := r.FormValue("user")
		cmd := &unsubscribeApiCommand{
			Channel: channel,
			User:    user,
		}
		resp, err = app.handleUnsubscribeCommand(project, cmd)
	case "disconnect":
		user := r.FormValue("user")
		cmd := &disconnectApiCommand{
			User: user,
		}
		resp, err = app.handleDisconnectCommand(project, cmd)
	case "presence":
		channel := r.FormValue("channel")
		cmd := &presenceApiCommand{
			Channel: channel,
		}
		resp, err = app.handlePresenceCommand(project, cmd)
	case "history":
		channel := r.FormValue("channel")
		cmd := &historyApiCommand{
			Channel: channel,
		}
		resp, err = app.handleHistoryCommand(project, cmd)
	}

	if err != nil {
		logger.ERROR.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

var upgrader = &websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}

type wsHandler struct {
	app *application
}

func (wsh wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c, err := newAdminClient(wsh.app, ws)
	if err != nil {
		return
	}
	logger.INFO.Print("new admin session established")
	defer func() {
		err := wsh.app.removeAdminConnection(c)
		if err != nil {
			logger.ERROR.Println(err)
		}
		logger.INFO.Println("admin session closed")
	}()
	go c.writer()
	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		resp, err := c.handleMessage(message)
		if err != nil {
			break
		}
		msgBytes, err := resp.toJson()
		if err != nil {
			break
		} else {
			err := c.send(string(msgBytes))
			if err != nil {
				break
			}
		}
	}
	c.ws.Close()
}
