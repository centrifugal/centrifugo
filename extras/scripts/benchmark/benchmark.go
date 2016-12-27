package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"golang.org/x/net/websocket"
)

func publisher(chTrigger chan int, chTime chan time.Time, url, origin, connectMessage, subscribeMessage, publishMessage string) {
	var err error
	var ws *websocket.Conn
	for {
		ws, err = websocket.Dial(url, "", origin)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		break
	}

	var msg = make([]byte, 512)

	if _, err := ws.Write([]byte(connectMessage)); err != nil {
		fmt.Println("publisher connect write error")
		log.Fatal(err)
	}
	if _, err = ws.Read(msg); err != nil {
		fmt.Println("publisher connect read error")
		log.Fatal(err)
	}

	if _, err := ws.Write([]byte(subscribeMessage)); err != nil {
		fmt.Println("publisher subscribe write error")
		log.Fatal(err)
	}
	if _, err = ws.Read(msg); err != nil {
		fmt.Println("publisher subscribe read error")
		log.Fatal(err)
	}

	for {
		<-chTrigger

		if _, err := ws.Write([]byte(publishMessage)); err != nil {
			fmt.Println("publisher publish write error")
			log.Fatal(err)
		}

		chTime <- time.Now()

		if _, err = ws.Read(msg); err != nil {
			fmt.Println("publisher publish read error")
			log.Fatal(err)
		}

		if _, err = ws.Read(msg); err != nil {
			fmt.Println("publisher message read error")
			log.Fatal(err)
		}

	}
}

func subscriber(chSub, chMsg, chStart chan int, url, origin, connectMessage, subscribeMessage, publishMessage string) {
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

	chSub <- 1

	for {
		if _, err = ws.Read(msg); err != nil {
			fmt.Println("subscriber msg read error")
			log.Fatal(err)
		}
		//fmt.Println("message received")
		chMsg <- 1
	}
}

func main() {

	origin := "http://localhost:8000/"
	url := os.Args[1]
	secret := os.Args[2]
	maxClients, _ := strconv.Atoi(os.Args[3])
	increment, _ := strconv.Atoi(os.Args[4])
	repeats, _ := strconv.Atoi(os.Args[5])

	fmt.Printf("max clients: %d\n", maxClients)
	fmt.Printf("increment: %d\n", increment)
	fmt.Printf("repeat: %d\n", repeats)

	messagesReceived := 0

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	token := auth.GenerateClientToken(secret, "test", timestamp, "")

	connectMessage := fmt.Sprintf("{\"params\": {\"timestamp\": \"%s\", \"token\": \"%s\", \"user\": \"test\"}, \"method\": \"connect\"}", timestamp, token)
	subscribeMessage := "{\"params\": {\"channel\": \"test\"}, \"method\": \"subscribe\"}"
	publishMessage := "{\"params\": {\"data\": {\"input\": \"I am benchmarking Centrifuge at moment\"}, \"channel\": \"test\"}, \"method\": \"publish\"}"

	chSub := make(chan int)
	chMsg := make(chan int)
	chStart := make(chan int)
	chTrigger := make(chan int)
	chTime := make(chan time.Time)

	var startTime time.Time

	totalTime := 0.0

	fullTime := 0.0

	go func() {
		publisher(chTrigger, chTime, url, origin, connectMessage, subscribeMessage, publishMessage)
	}()

	for i := 0; i < maxClients; i += increment {

		time.Sleep(50 * time.Millisecond)
		totalTime = 0
		for j := 0; j < increment; j++ {
			time.Sleep(5 * time.Millisecond)
			go func() {
				subscriber(chSub, chMsg, chStart, url, origin, connectMessage, subscribeMessage, publishMessage)
			}()
			<-chSub

		}

		currentClients := i + increment

		// repeat several times to get average time value
		for k := 0; k < repeats; k++ {
			time.Sleep(100 * time.Millisecond)
			fullTime = 0.0
			messagesReceived = 0
			// publish message
			chTrigger <- 1
			startTime = <-chTime
			for {
				<-chMsg
				messagesReceived += 1
				elapsed := time.Since(startTime)
				fullTime += float64(elapsed)
				if messagesReceived == currentClients {
					break
				}
			}
			totalTime += fullTime / float64(currentClients)
		}
		fmt.Printf("%d\t%d\n", currentClients, int(totalTime/float64(repeats)))
	}
}
