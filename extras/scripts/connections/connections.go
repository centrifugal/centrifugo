package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"golang.org/x/net/websocket"
)

func subscriber(numChannels int, chSub chan struct{}, url, origin, connectMessage string) {
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

	for i := 1; i <= numChannels; i++ {
		channel := fmt.Sprintf("channel%d", i)
		subscribeMessage := "{\"params\": {\"channel\": \"" + channel + "\"}, \"method\": \"subscribe\"}"
		if _, err := ws.Write([]byte(subscribeMessage)); err != nil {
			fmt.Println("subscriber subscribe write error")
			log.Fatal(err)
		}
		if _, err = ws.Read(msg); err != nil {
			fmt.Println("subscriber subscribe read error")
			log.Fatal(err)
		}
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
	var numChannels = 1
	if len(os.Args) > 4 {
		numChannels, _ = strconv.Atoi(os.Args[4])
	}

	fmt.Printf("clients: %d\nchannels: %d\n", clients, numChannels)

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	token := auth.GenerateClientToken(secret, "test", timestamp, "")
	connectMessage := fmt.Sprintf("{\"params\": {\"timestamp\": \"%s\", \"token\": \"%s\", \"user\": \"test\"}, \"method\": \"connect\"}", timestamp, token)

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	connected := 0

	for i := 0; i < clients; i += 1 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			chSub := make(chan struct{})
			go subscriber(numChannels, chSub, url, origin, connectMessage)
			<-chSub
			mu.Lock()
			connected += 1
			fmt.Printf("\r%d", connected)
			mu.Unlock()
		}()

	}

	wg.Wait()

	// Just run until interrupted keeping connections open.
	select {}
}
