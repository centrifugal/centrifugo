package cli

import (
	"bytes"
	"fmt"
	"log"
	"net/http"

	_ "embed"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
)

//go:embed configdocs/config.md
var mdContent string

func ConfigDoc() *cobra.Command {
	var genConfigCmd = &cobra.Command{
		Use:   "configdoc",
		Short: "Generate ???",
		Long:  `Generate ???`,
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

func markdownHandler(w http.ResponseWriter, r *http.Request) {
	// Convert Markdown to HTML using goldmark.
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(mdContent), &buf); err != nil {
		http.Error(w, "Error converting Markdown", http.StatusInternalServerError)
		return
	}

	// Build the complete HTML page using a modern Bootswatch Minty theme.
	htmlPage := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Modern Markdown Display</title>
  <!-- Bootswatch Minty Theme (Bootstrap 5) -->
  <link href="https://cdn.jsdelivr.net/npm/bootswatch@5.3.0/dist/minty/bootstrap.min.css" rel="stylesheet">
  <style>
    body { padding: 2rem; }
    .markdown-body { max-width: 900px; margin: auto; }
    h1, h2, h3, h4, h5, h6 { margin-top: 1.5rem; }
  </style>
</head>
<body>
  <div class="container markdown-body">
    %s
  </div>
  <!-- Optional Bootstrap JS Bundle -->
  <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/js/bootstrap.bundle.min.js"></script>
</body>
</html>`, buf.String())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(htmlPage))
}
