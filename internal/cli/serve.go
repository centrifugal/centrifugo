package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
)

func Serve(serveAddr string, servePort int, serveDir string) {
	address := net.JoinHostPort(serveAddr, strconv.Itoa(servePort))
	fmt.Printf("start serving %s on %s\n", serveDir, address)
	if err := http.ListenAndServe(address, http.FileServer(http.Dir(serveDir))); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
