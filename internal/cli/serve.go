package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

func Serve() *cobra.Command {
	var serveDir string
	var servePort int
	var serveAddr string
	var serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Run static file server (for development only)",
		Long:  `Run static file server (for development only)`,
		Run: func(cmd *cobra.Command, args []string) {
			serve(serveAddr, servePort, serveDir)
		},
	}
	serveCmd.Flags().StringVarP(&serveDir, "dir", "d", "./", "path to directory")
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 3000, "port to serve on")
	serveCmd.Flags().StringVarP(&serveAddr, "address", "a", "", "interface to serve on (default: all interfaces)")
	return serveCmd
}

func serve(serveAddr string, servePort int, serveDir string) {
	address := net.JoinHostPort(serveAddr, strconv.Itoa(servePort))
	fmt.Printf("start serving %s on %s\n", serveDir, address)
	if err := http.ListenAndServe(address, http.FileServer(http.Dir(serveDir))); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
