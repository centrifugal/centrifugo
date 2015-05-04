package libcentrifugo

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/logger"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/websocket"
	"gopkg.in/igm/sockjs-go.v2/sockjs"
)

func newClientConnectionHandler(app *application) http.Handler {
	return sockjs.NewHandler("/connection", sockjs.DefaultOptions, app.clientConnectionHandler)
}

func (app *application) clientConnectionHandler(s sockjs.Session) {

	c, err := newClient(app, s)
	if err != nil {
		logger.ERROR.Println(err)
		return
	}
	defer func() {
		c.clean()
		logger.INFO.Println("client session closed")
	}()
	logger.INFO.Println("new client session established")

	go c.sendMessages()

	for {
		if msg, err := s.Recv(); err == nil {
			err = c.handleMessage([]byte(msg))
			if err != nil {
				logger.ERROR.Println(err)
				s.Close(3000, err.Error())
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

func (app *application) apiHandler(w http.ResponseWriter, r *http.Request) {
	defer timeTrack(time.Now(), "api call completed in")

	projectKey := r.URL.Path[len("/api/"):]
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

func (app *application) authHandler(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")
	if password == app.config.password {
		w.Header().Set("Content-Type", "application/json")
		s := securecookie.New([]byte(app.config.secret), nil)
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

func (app *application) Authenticated(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Token ")
			s := securecookie.New([]byte(app.config.secret), nil)
			var val string
			if err := s.Decode(tokenKey, token, &val); err == nil {
				if val == tokenValue {
					h(w, r)
					return
				}
			}
		}
		w.WriteHeader(401)
	}
}

func (app *application) infoHandler(w http.ResponseWriter, r *http.Request) {
	app.nodesMutex.Lock()
	defer app.nodesMutex.Unlock()
	info := map[string]interface{}{
		"version":   VERSION,
		"structure": app.structure.ProjectList,
		"engine":    app.engine.getName(),
		"node_name": app.config.name,
		"nodes":     app.nodes,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (app *application) actionHandler(w http.ResponseWriter, r *http.Request) {
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

func (app *application) adminWsConnectionHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	c, err := newAdminClient(app, ws)
	if err != nil {
		return
	}
	logger.INFO.Print("new admin session established")
	defer func() {
		close(c.closeChannel)
		err := app.removeAdminConnection(c)
		if err != nil {
			logger.ERROR.Println(err)
		}
		logger.INFO.Println("admin session closed")
	}()

	go c.writer()

	for {
		_, message, err := ws.ReadMessage()
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
}

type wsConnection struct {
	ws           *websocket.Conn
	writeChannel chan []byte // buffered channel of outbound messages.
}

func (conn wsConnection) Send(message string) error {

	select {
	case conn.writeChannel <- []byte(message):
	default:
		conn.ws.Close()
	}
	return nil
}

func (conn wsConnection) Close(status uint32, reason string) error {
	return conn.ws.Close()
}

// writer reads from channel and sends received messages into connection
func (conn *wsConnection) writer(closeChannel chan struct{}) {
	for {
		select {
		case message := <-conn.writeChannel:
			err := conn.ws.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				return
			}
		case <-closeChannel:
			return
		}
	}
}

func (app *application) wsConnectionHandler(w http.ResponseWriter, r *http.Request) {

	ws, err := websocket.Upgrade(w, r, nil, sockjs.WebSocketReadBufSize, sockjs.WebSocketWriteBufSize)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, `Can "Upgrade" only to "WebSocket".`, http.StatusBadRequest)
		return
	} else if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer ws.Close()

	conn := wsConnection{
		ws:           ws,
		writeChannel: make(chan []byte, 256),
	}

	c, err := newClient(app, conn)
	if err != nil {
		return
	}
	logger.INFO.Print("new client session established")
	defer func() {
		c.clean()
		logger.INFO.Println("client session closed")
	}()

	go c.sendMessages()

	go conn.writer(c.closeChannel)

	for {
		_, message, err := conn.ws.ReadMessage()
		if err != nil {
			break
		}
		err = c.handleMessage(message)
		if err != nil {
			logger.ERROR.Println(err)
			conn.ws.Close()
			break
		}
	}
}
