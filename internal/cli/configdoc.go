package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	_ "embed"

	"github.com/centrifugal/centrifugo/v6/internal/build"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
)

//go:embed configdoc/template.html
var htmlTemplate string

func ConfigDoc() *cobra.Command {
	var mdOutput bool
	var section string
	var port int
	var baseLevel int
	var configDocCmd = &cobra.Command{
		Use:   "configdoc",
		Short: "Show Centrifugo configuration documentation generated from source code",
		Long:  `Show Centrifugo configuration documentation generated from source code`,
		Run: func(cmd *cobra.Command, args []string) {
			configDoc(port, mdOutput, section, baseLevel)
		},
	}
	configDocCmd.Flags().BoolVarP(&mdOutput, "markdown", "m", false, "output markdown to stdout")
	configDocCmd.Flags().StringVarP(&section, "section", "s", "", "filter by top-level section name")
	configDocCmd.Flags().IntVarP(&port, "port", "p", 6060, "port to run server on")
	configDocCmd.Flags().IntVarP(&baseLevel, "base-level", "l", 0, "base level to use")
	return configDocCmd
}

func configDoc(port int, mdOutput bool, section string, baseLevel int) {
	// Documentation is built from the config structs at runtime — descriptions
	// come from the `doc` struct tags, types and defaults from reflection.
	docs := documentStruct(config.Config{}, "", 1)

	if mdOutput {
		mdContent := convertDocsToMarkdown(docs, section, baseLevel, mdOutput)
		fmt.Println(mdContent)
		return
	}

	// Filter docs by section if specified
	filteredDocs := docs
	if section != "" {
		filteredDocs = make([]FieldDoc, 0)
		for _, doc := range docs {
			if strings.HasPrefix(doc.Field, section) {
				filteredDocs = append(filteredDocs, doc)
			}
		}
	}

	go func() {
		url := "http://localhost:" + strconv.Itoa(port)
		// OpenURL will open the default browser.
		if err := browser.OpenURL(url); err != nil {
			fmt.Printf("Failed to open browser automatically: %v, see %s\n", err, url)
		}
	}()

	http.HandleFunc("/", makeMarkdownHandler(filteredDocs))
	if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil && !errors.Is(err, http.ErrServerClosed) { //nolint:gosec // Only for development use.
		fmt.Println("Failed to start server:", err)
		os.Exit(1)
	}
}

// FieldDoc represents the JSON documentation for a configuration field.
type FieldDoc struct {
	Field         string     `json:"field"`
	Name          string     `json:"name"`
	GoName        string     `json:"go_name"`
	Level         int        `json:"level"`
	Type          string     `json:"type"`
	Default       string     `json:"default"`
	Comment       string     `json:"comment"`
	IsComplexType bool       `json:"is_complex_type"`
	Children      []FieldDoc `json:"children,omitempty"`
}

// ConvertDocsToMarkdown recursively converts a slice of FieldDoc entries into Markdown.
func convertDocsToMarkdown(docs []FieldDoc, section string, baseLevel int, mdOutput bool) string {
	var sb strings.Builder
	for _, doc := range docs {
		if section != "" && !strings.HasPrefix(doc.Field, section) {
			continue
		}
		// Generate a header with the appropriate Markdown level.
		fieldLevel := doc.Level + baseLevel
		if fieldLevel > 6 {
			fieldLevel = 6
		}
		header := strings.Repeat("#", fieldLevel)

		typeDesc := fmt.Sprintf("Type: `%s`", doc.Type)
		if doc.IsComplexType {
			typeDesc = fmt.Sprintf("Type: `%s` object", doc.Type)
		}
		if doc.Default != "" {
			typeDesc += fmt.Sprintf(". Default: `%s`", doc.Default)
		}

		var envVar string
		if !strings.Contains(doc.Field, "[]") {
			if !doc.IsComplexType {
				envVar = "CENTRIFUGO_" + strings.ToUpper(strings.ReplaceAll(doc.Field, `.`, "_"))
			} else {
				if strings.Contains(doc.Field, "namespaces") || strings.Contains(doc.Field, "proxies") || strings.Contains(doc.Field, "consumers") {
					envVar = "CENTRIFUGO_" + strings.ToUpper(strings.ReplaceAll(doc.Field, `.`, "_"))
				} else if doc.Default == "[]" {
					envVar = "CENTRIFUGO_" + strings.ToUpper(strings.ReplaceAll(doc.Field, `.`, "_"))
				}
			}
		} else {
			if !strings.HasSuffix(doc.Field, ".name") &&
				(strings.HasPrefix(doc.Field, "namespaces") ||
					strings.HasPrefix(doc.Field, "proxies") ||
					strings.HasPrefix(doc.Field, "consumers")) &&
				(!doc.IsComplexType || doc.Default == "[]") {
				field := strings.Replace(doc.Field, `[]`, "_<NAME>", 1)
				envVar = "CENTRIFUGO_" + strings.ToUpper(strings.ReplaceAll(field, `.`, "_"))
			}
			if strings.Contains(envVar, "[]") {
				envVar = ""
			}
		}

		if envVar != "" {
			typeDesc += fmt.Sprintf("\n\nEnv: `%s`", envVar)
		}

		comment := doc.Comment
		if comment == "" {
			comment = "No documentation available."
		}
		if strings.HasPrefix(comment, doc.GoName) {
			comment = fmt.Sprintf("`%s`%s", doc.Name, comment[len(doc.GoName):])
		}

		field := fmt.Sprintf("`%s`", doc.Field)
		if !mdOutput && doc.IsComplexType {
			if doc.Default == "[]" {
				field += " `[...]`"
			} else {
				field += " `{...}`"
			}
		}

		sb.WriteString(fmt.Sprintf("%s %s\n\n%s\n\n%s\n\n", header, field, typeDesc, comment))
		if len(doc.Children) > 0 {
			sb.WriteString(convertDocsToMarkdown(doc.Children, section, baseLevel, mdOutput))
		}
	}
	return sb.String()
}

func makeMarkdownHandler(docs []FieldDoc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if simple markdown view is requested
		if r.URL.Query().Get("simple") == "1" {
			serveSimpleMarkdownView(w, docs)
			return
		}

		// Convert docs to JSON for JavaScript consumption
		configJSON, err := json.Marshal(docs)
		if err != nil {
			http.Error(w, "Error marshaling config data: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Parse the HTML template
		tmpl, err := template.New("config").Parse(htmlTemplate)
		if err != nil {
			http.Error(w, "Error parsing template: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Prepare template data
		data := struct {
			Version    string
			Content    template.HTML
			ConfigJSON template.JS
		}{
			Version:    build.Version,
			Content:    "",                      // Content is now generated by JavaScript.
			ConfigJSON: template.JS(configJSON), //nolint:gosec // Trusted data.
		}

		// Execute template
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Error executing template: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func serveSimpleMarkdownView(w http.ResponseWriter, docs []FieldDoc) {
	mdContent := convertDocsToMarkdown(docs, "", 0, false)

	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(mdContent), &buf); err != nil {
		http.Error(w, "Error converting markdown: "+err.Error(), http.StatusInternalServerError)
		return
	}

	simpleHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Centrifugo %s Configuration Options (Simple View)</title>
  <link href="https://cdn.jsdelivr.net/npm/bootswatch@5.3.0/dist/materia/bootstrap.min.css" rel="stylesheet">
  <style>
    body { padding: 2rem; }
    .markdown-body { max-width: 1024px; margin: auto; }
    code { color: #139f87; background-color: #f8f9fa; padding: 0.2rem 0.4rem; border-radius: 0.25rem; }
    .controls { text-align: center; margin-bottom: 2rem; }
  </style>
</head>
<body>
  <div class="container markdown-body">
    <div class="controls">
      <a href="/" class="btn btn-primary">Switch to Enhanced View</a>
    </div>
    %s
  </div>
</body>
</html>`, build.Version, buf.String())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(simpleHTML))
}

var docCodeRe = regexp.MustCompile(`<<(.+?)>>`)

// convertDocCodes turns <<value>> markers in doc text into Markdown inline code.
func convertDocCodes(s string) string {
	return docCodeRe.ReplaceAllString(s, "`$1`")
}

// getDisplayType returns the base type name and whether it's a complex (object)
// type. Both the type label and the complex/object classification are shared via
// configtypes so configdoc and the admin config UI (Pro) stay consistent.
func getDisplayType(t reflect.Type) (string, bool) {
	return configtypes.TypeLabel(t), configtypes.IsComplexType(t)
}

// documentStruct walks a config struct via reflection and builds the field
// documentation tree at runtime — descriptions from the `doc` tag, types and
// defaults from reflection. Fields tagged `expose:"-"` are excluded.
func documentStruct(cfg interface{}, parentKey string, level int) []FieldDoc {
	var docs []FieldDoc
	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return docs
	}

	fieldLevel := level
	if parentKey != "" {
		fieldLevel = level + 1
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Tag.Get("json") == "-" {
			continue
		}
		// Flatten squashed/embedded structs.
		if configtypes.IsSquash(field) {
			var nested interface{}
			if field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct {
				nested = reflect.New(field.Type.Elem()).Interface()
			} else if field.Type.Kind() == reflect.Struct {
				nested = reflect.New(field.Type).Interface()
			}
			if nested != nil {
				docs = append(docs, documentStruct(nested, parentKey, level)...)
			}
			continue
		}
		if field.Tag.Get("expose") == "-" {
			continue
		}

		keyTag := configtypes.JSONKey(field)
		fullKey := keyTag
		if parentKey != "" {
			fullKey = parentKey + "." + keyTag
		}

		displayType, isComplex := getDisplayType(field.Type)
		docEntry := FieldDoc{
			Field:         fullKey,
			Name:          keyTag,
			GoName:        field.Name,
			Level:         fieldLevel,
			Type:          displayType,
			Default:       field.Tag.Get("default"),
			IsComplexType: isComplex,
			Comment:       convertDocCodes(field.Tag.Get("doc")),
		}

		var children []FieldDoc
		switch field.Type.Kind() {
		case reflect.Struct:
			if !(field.Type.PkgPath() == "time" && field.Type.Name() == "Time") {
				children = documentStruct(reflect.New(field.Type).Interface(), fullKey, fieldLevel)
			}
		case reflect.Ptr:
			if field.Type.Elem().Kind() == reflect.Struct {
				children = documentStruct(reflect.New(field.Type.Elem()).Interface(), fullKey, fieldLevel)
			}
		case reflect.Slice:
			elem := field.Type.Elem()
			if elem.Kind() == reflect.Ptr {
				elem = elem.Elem()
			}
			if elem.Kind() == reflect.Struct {
				children = documentStruct(reflect.New(elem).Interface(), fullKey+"[]", fieldLevel)
			}
		}
		if len(children) > 0 {
			docEntry.Children = children
		}
		docs = append(docs, docEntry)
	}
	return docs
}
