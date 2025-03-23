package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	_ "embed"

	"github.com/centrifugal/centrifugo/v6/internal/build"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
)

//go:embed configdoc/schema.json
var configSchema string

func ConfigDoc() *cobra.Command {
	var mdOutput bool
	var section string
	var port int
	var configDocCmd = &cobra.Command{
		Use:   "configdoc",
		Short: "Show Centrifugo configuration documentation generated from source code",
		Long:  `Show Centrifugo configuration documentation generated from source code`,
		Run: func(cmd *cobra.Command, args []string) {
			configDoc(port, mdOutput, section)
		},
	}
	configDocCmd.Flags().BoolVarP(&mdOutput, "markdown", "m", false, "output markdown to stdout")
	configDocCmd.Flags().StringVarP(&section, "section", "s", "", "filter by top-level section name")
	configDocCmd.Flags().IntVarP(&port, "port", "p", 6060, "port to run server on")
	return configDocCmd
}

func configDoc(port int, mdOutput bool, section string) {
	var docs []FieldDoc
	if err := json.Unmarshal([]byte(configSchema), &docs); err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	mdContent := convertDocsToMarkdown(docs, section)

	if mdOutput {
		fmt.Println(mdContent)
		return
	}

	// Start a goroutine to open the URL automatically.
	go func() {
		url := "http://localhost:" + strconv.Itoa(port)
		// OpenURL will open the default browser.
		if err := browser.OpenURL(url); err != nil {
			log.Printf("Failed to open browser automatically: %v", err)
		}
	}()

	http.HandleFunc("/", makeMarkdownHandler(mdContent))
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port), nil))
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
func convertDocsToMarkdown(docs []FieldDoc, section string) string {
	var sb strings.Builder
	for _, doc := range docs {
		if section != "" && !strings.HasPrefix(doc.Field, section) {
			continue
		}
		// Generate a header with the appropriate Markdown level.
		fieldLevel := doc.Level
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
		sb.WriteString(fmt.Sprintf("%s `%s`\n\n%s\n\n%s\n\n", header, doc.Field, typeDesc, comment))
		if len(doc.Children) > 0 {
			sb.WriteString(convertDocsToMarkdown(doc.Children, section))
		}
	}
	return sb.String()
}

func makeMarkdownHandler(mdContent string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		if err := goldmark.Convert([]byte(mdContent), &buf); err != nil {
			http.Error(w, "Error converting markdown: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Build the complete HTML page using a modern Bootswatch Minty theme.
		htmlPage := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Centrifugo %s configuration options</title>
  <!-- Bootswatch Minty Theme (Bootstrap 5) -->
  <link href="https://cdn.jsdelivr.net/npm/bootswatch@5.3.0/dist/materia/bootstrap.min.css" rel="stylesheet">
  <style>
    body { padding: 2rem; }
    .markdown-body { max-width: 1024px; margin: auto; }
	h1, h2, h3, h4, h5, h6 {
		font-size: 1.2rem;
		margin-top: 1.5rem;
		margin-bottom: 0.6rem;
	}
	p { margin-bottom: 0.6rem; }
	code { color: #139f87; }
	h1 code, h2 code, h3 code, h4 code, h5 code, h6 code { color: #04416d; }
    /* Set initial left margins for headers */
    h1 { margin-left: 0rem; }
    h2 { margin-left: 2rem; }
    h3 { margin-left: 4rem; }
    h4 { margin-left: 6rem; }
    h5 { margin-left: 8rem; }
    h6 { margin-left: 10rem; }
    /* Reset paragraphs to no left margin by default */
    .markdown-body p { margin-left: 0; }
  </style>
</head>
<body>
  <div class="container markdown-body">
    %s
  </div>
  <!-- Optional Bootstrap JS Bundle -->
  <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/js/bootstrap.bundle.min.js"></script>
  <script>
    document.addEventListener("DOMContentLoaded", function() {
      // Select all paragraphs inside .markdown-body
      var paragraphs = document.querySelectorAll(".markdown-body p");
      paragraphs.forEach(function(p) {
        var prev = p.previousElementSibling;
        // Traverse backward until a header is found
        while (prev && !/^H[1-6]$/.test(prev.tagName)) {
          prev = prev.previousElementSibling;
        }
        if (prev) {
          // Get computed left margin of the header and assign it to the paragraph
          var headerMargin = window.getComputedStyle(prev).marginLeft;
          p.style.marginLeft = headerMargin;
        }
      });
    });
  </script>
</body>
</html>`, build.Version, buf.String())

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlPage))
	}
}
