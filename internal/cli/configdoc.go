package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	_ "embed"

	"github.com/centrifugal/centrifugo/v6/internal/build"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
)

//go:embed configdocs/config.json
var configRepresentation string

func ConfigDoc() *cobra.Command {
	var genConfigCmd = &cobra.Command{
		Use:   "configdoc",
		Short: "Show Centrifugo configuration documentation generated from source code",
		Long:  `Show Centrifugo configuration documentation generated from source code`,
		Run: func(cmd *cobra.Command, args []string) {
			configDoc()
		},
	}
	return genConfigCmd
}

func configDoc() {
	// Start a goroutine to open the URL automatically.
	go func() {
		url := "http://localhost:6060"
		// OpenURL will open the default browser.
		if err := browser.OpenURL(url); err != nil {
			log.Printf("Failed to open browser automatically: %v", err)
		}
	}()

	http.HandleFunc("/", markdownHandler)
	log.Println("Server running on http://localhost:6060")
	log.Fatal(http.ListenAndServe(":6060", nil))
}

// FieldDoc represents the JSON documentation for a configuration field.
type FieldDoc struct {
	Field           string     `json:"field"`
	Name            string     `json:"name"`
	GoName          string     `json:"go_name"`
	Level           int        `json:"level"`
	Type            string     `json:"type"`
	Default         string     `json:"default"`
	TypeDescription string     `json:"type_description"`
	Comment         string     `json:"comment"`
	IsComplexType   bool       `json:"is_complex_type"`
	Children        []FieldDoc `json:"children,omitempty"`
}

// ConvertDocsToMarkdown recursively converts a slice of FieldDoc entries into Markdown.
func convertDocsToMarkdown(docs []FieldDoc) string {
	var sb strings.Builder
	for _, doc := range docs {
		// Generate a header with the appropriate Markdown level.
		fieldLevel := doc.Level
		if fieldLevel > 6 {
			fieldLevel = 6
		}
		header := strings.Repeat("#", fieldLevel)
		comment := doc.Comment
		if comment == "" {
			comment = "No documentation available."
		}
		if strings.HasPrefix(comment, doc.GoName) {
			comment = fmt.Sprintf("`%s`%s", doc.Name, comment[len(doc.GoName):])
		}
		sb.WriteString(fmt.Sprintf("%s `%s`\n\n%s\n\n%s\n\n", header, doc.Field, doc.TypeDescription, comment))
		if len(doc.Children) > 0 {
			sb.WriteString(convertDocsToMarkdown(doc.Children))
		}
	}
	return sb.String()
}

func markdownHandler(w http.ResponseWriter, r *http.Request) {
	var docs []FieldDoc
	if err := json.Unmarshal([]byte(configRepresentation), &docs); err != nil {
		http.Error(w, "Error unmarshalling JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}

	mdContent := convertDocsToMarkdown(docs)

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
	}
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
