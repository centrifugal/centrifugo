package main

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"text/template"

	"github.com/centrifugal/centrifugo/v6/internal/gen"
)

func main() {
	generateHandlersHTTP()
	generateHandlersGRPC()
	generateHandlersConsumer()
	generateRequestDecoder()
	generateResponseEncoder()
	generateResultEncoder()
}

type TemplateData struct {
	RequestCapitalized string
	RequestLower       string
	RequestSnake       string
}

func generateToFile(header, funcTmpl, outFile string, excludeRequests []string, includeRequests []string) {
	tmpl := template.Must(template.New("").Parse(funcTmpl))

	var buf bytes.Buffer
	buf.WriteString(header)

	for _, req := range gen.Requests {
		if slices.Contains(excludeRequests, req) {
			continue
		}
		if len(includeRequests) > 0 && !slices.Contains(includeRequests, req) {
			continue
		}
		err := tmpl.Execute(&buf, TemplateData{
			RequestCapitalized: req,
			RequestLower:       strings.ToLower(req),
			RequestSnake:       gen.CamelToSnake(req),
		})
		if err != nil {
			panic(err)
		}
	}

	file, err := os.Create(outFile) //nolint:gosec // Used only for code generation.
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = file.Close()
	}()
	_, _ = buf.WriteTo(file)
}
