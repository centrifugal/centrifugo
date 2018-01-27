package main

// https://github.com/dcodeIO/protobuf.js/issues/55

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/centrifugal/centrifugo/lib/proto"
	"github.com/centrifugal/centrifugo/lib/proto/client"
	"github.com/gorilla/websocket"
	"github.com/igm/sockjs-go/sockjs"
)

var reply1binary []byte
var reply2binary []byte
var reply1json []byte
var reply2json []byte
var textMessages chan []byte
var binaryMessages chan []byte

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func xhrJSONHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	select {
	case <-time.After(25e9):
		io.WriteString(w, "Timeout!\n")
	case msg := <-textMessages:
		w.Write(msg)
	}
}

func xhrBinaryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	select {
	case <-time.After(25e9):
		io.WriteString(w, "Timeout!\n")
	case msg := <-binaryMessages:
		bs := make([]byte, 8)
		n := binary.PutUvarint(bs, uint64(len(reply1binary)))
		w.Write(bs[:n])
		w.Write(msg)
	}
}

func sockjsHandler(session sockjs.Session) {

	var buf bytes.Buffer
	buf.Write(reply1json)
	buf.Write([]byte("\n"))
	buf.Write(reply2json)
	session.Send(buf.String())

	for {
		if msg, err := session.Recv(); err == nil {
			println(msg)
			continue
		}
		break
	}
}

func jsonHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	wr, _ := conn.NextWriter(websocket.TextMessage)

	wr.Write(reply1json)
	wr.Write([]byte("\n"))
	wr.Write(reply2json)

	wr.Close()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			return
		}
	}
}

func binaryHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	wr, _ := conn.NextWriter(websocket.BinaryMessage)

	bs := make([]byte, 8)
	n := binary.PutUvarint(bs, uint64(len(reply1binary)))
	println(n)
	wr.Write(bs[:n])
	fmt.Printf("%#v\n", bs[:n])
	wr.Write(reply1binary)
	bs = make([]byte, 8)
	n = binary.PutUvarint(bs, uint64(len(reply2binary)))
	wr.Write(bs[:n])
	wr.Write(reply2binary)
	wr.Close()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		fmt.Printf("%#v\n", data)
		offset := 0
		for offset < len(data) {
			l, n := binary.Uvarint(data[offset:])
			println(len(data), offset, l, n)
			d := data[offset+n : offset+n+int(l)]
			offset = offset + n + int(l)
			var c client.Command
			c.Unmarshal(d)
			println(c.ID, c.Method)
		}
	}
}

func main() {

	msg1 := &proto.Message{
		Data: proto.Raw([]byte(`{"test": "юникод1"}`)),
	}

	msg1binary, _ := msg1.Marshal()

	msg1json, err := json.Marshal(msg1)
	if err != nil {
		panic(err)
	}

	asyncMessage1binary, _ := (&client.AsyncMessage{
		Type: client.AsyncMessageTypeData,
		Data: msg1binary,
	}).Marshal()

	asyncMessage1json, _ := json.Marshal(client.AsyncMessage{
		Type: client.AsyncMessageTypeData,
		Data: msg1json,
	})

	reply1binary, err = (&client.Reply{
		Result: asyncMessage1binary,
	}).Marshal()
	if err != nil {
		panic(err)
	}

	reply1json, _ = json.Marshal(client.Reply{
		Result: asyncMessage1json,
	})

	msg2 := &proto.Message{
		Data: proto.Raw([]byte(`{"test": "юникод2"}`)),
	}

	msg2binary, _ := msg2.Marshal()
	msg2json, err := json.Marshal(msg2)
	if err != nil {
		panic(err)
	}

	asyncMessage2binary, _ := (&client.AsyncMessage{
		Type: client.AsyncMessageTypeData,
		Data: msg2binary,
	}).Marshal()

	asyncMessage2json, _ := json.Marshal(client.AsyncMessage{
		Type: client.AsyncMessageTypeData,
		Data: msg2json,
	})

	reply2binary, err = (&client.Reply{
		Result: asyncMessage2binary,
	}).Marshal()
	if err != nil {
		panic(err)
	}

	reply2json, _ = json.Marshal(client.Reply{
		Result: asyncMessage2json,
	})

	textMessages = make(chan []byte)
	binaryMessages = make(chan []byte)
	go func() {
		for {
			time.Sleep(2 * time.Second)
			select {
			case textMessages <- reply1json:
			default:
				continue
			}
		}
	}()
	go func() {
		for {
			time.Sleep(2 * time.Second)
			select {
			case binaryMessages <- reply1binary:
			default:
				continue
			}
		}
	}()

	http.HandleFunc("/json", jsonHandler)
	http.HandleFunc("/binary", binaryHandler)
	http.HandleFunc("/xhrjson", xhrJSONHandler)
	http.HandleFunc("/xhrbinary", xhrBinaryHandler)
	http.Handle("/sockjs/", sockjs.NewHandler("/sockjs", sockjs.DefaultOptions, sockjsHandler))
	if err := http.ListenAndServe("127.0.0.1:1337", nil); err != nil {
		log.Fatal(err)
	}
}
