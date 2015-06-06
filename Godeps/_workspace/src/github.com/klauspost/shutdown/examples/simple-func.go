// +build ignore

package main

import (
	"github.com/klauspost/shutdown"
	"log"
	"net/http"
	"os"
	"syscall"
)

// This example shows a server that has logging to a file
//
// When the webserver is closed, it will close the file when all requests have
// been finished.
//
// In a real world, you would not want multiple goroutines writing to the same file
//
// To execute, use 'go run simple-func.go'

// This is the function we would like to execute at shutdown.
func closeFile(i interface{}) {
	f := i.(*os.File)
	log.Println("Closing", f.Name()+"...")
	f.Close()
}

func main() {
	// Make shutdown catch Ctrl+c and system terminate
	shutdown.OnSignal(0, os.Interrupt, syscall.SIGTERM)

	// Create a log file
	var logFile *os.File
	logFile, _ = os.Create("log.txt")

	// When shutdown is initiated, close the file
	shutdown.FirstFunc(closeFile, logFile)

	// Start a webserver
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		// Get a lock, and write to the file if we get it.
		// While we have the lock the file will not be closed.
		if shutdown.Lock() {
			_, _ = logFile.WriteString(req.URL.String() + "\n")
			shutdown.Unlock()
		}
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
