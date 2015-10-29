package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/code.google.com/p/go.net/websocket"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
)

func subscriber(chSub chan struct{}, url, origin, connectMessage, subscribeMessage string) {
	var err error
	var ws *websocket.Conn
	for {
		ws, err = websocket.Dial(url, "", origin)
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		break
	}
	if _, err := ws.Write([]byte(connectMessage)); err != nil {
		fmt.Println("subscriber connect write error")
		log.Fatal(err)
	}
	var msg = make([]byte, 512)

	if _, err = ws.Read(msg); err != nil {
		fmt.Println("subscriber connect read error")
		log.Fatal(err)
	}

	if _, err := ws.Write([]byte(subscribeMessage)); err != nil {
		fmt.Println("subscriber subscribe write error")
		log.Fatal(err)
	}
	if _, err = ws.Read(msg); err != nil {
		fmt.Println("subscriber subscribe read error")
		log.Fatal(err)
	}

	close(chSub)

	for {
		if _, err = ws.Read(msg); err != nil {
			fmt.Println("subscriber msg read error")
			log.Fatal(err)
		}
	}
}

func main() {

	origin := "http://localhost:8000/"
	url := os.Args[1]
	secret := os.Args[2]
	clients, _ := strconv.Atoi(os.Args[3])

	fmt.Printf("clients: %d\n", clients)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	token := auth.GenerateClientToken(secret, "test", timestamp, "")
	connectMessage := fmt.Sprintf("{\"params\": {\"timestamp\": \"%s\", \"token\": \"%s\", \"user\": \"test\"}, \"method\": \"connect\"}", timestamp, token)
	subscribeMessage := "{\"params\": {\"channel\": \"test\"}, \"method\": \"subscribe\"}"
	done := make(chan struct{})

	for i := 0; i < clients; i += 1 {
		chSub := make(chan struct{})
		go subscriber(chSub, url, origin, connectMessage, subscribeMessage)
		<-chSub
		fmt.Printf("\r%d", i+1)
	}

	// Just run until interrupted keeping connections open.
	<-done
}
