# How to run?

Run `go generate` to create a statik package that embeds the binary data underneath the public directory.

	$ go generate

Once the statik package is generated, run the web server:

	$ go run main.go

Visit [http://localhost:8080/public/hello.txt](http://localhost:8080/public/hello.txt) to see the file.
