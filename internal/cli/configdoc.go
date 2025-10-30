package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
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
	var docs []FieldDoc
	if err := json.Unmarshal([]byte(configSchema), &docs); err != nil {
		fmt.Printf("error unmarshalling config schema: %v\n", err)
		os.Exit(1)
	}

	mdContent := convertDocsToMarkdown(docs, section, baseLevel, mdOutput)

	if mdOutput {
		fmt.Println(mdContent)
		return
	}

	go func() {
		url := "http://localhost:" + strconv.Itoa(port)
		// OpenURL will open the default browser.
		if err := browser.OpenURL(url); err != nil {
			fmt.Printf("Failed to open browser automatically: %v, see %s\n", err, url)
		}
	}()

	http.HandleFunc("/", makeMarkdownHandler(mdContent))
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

func makeMarkdownHandler(mdContent string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		if err := goldmark.Convert([]byte(mdContent), &buf); err != nil {
			http.Error(w, "Error converting markdown: "+err.Error(), http.StatusInternalServerError)
			return
		}

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
        margin-top: 1rem;
        margin-bottom: 0.6rem;
    }
    p { margin-bottom: 0.6rem; }
    code { color: #139f87; }
    h1 code, h2 code, h3 code, h4 code, h5 code, h6 code { color: #04416d; }
    h1 { margin-left: 0rem; }
    h2 { margin-left: 2rem; }
    h3 { margin-left: 4rem; }
    h4 { margin-left: 6rem; }
    h5 { margin-left: 8rem; }
    h6 { margin-left: 10rem; }
    .markdown-body p { margin-left: 0; }
    /* Visual indicator for clickable headers */
    .markdown-body h1.clickable::before,
    .markdown-body h2.clickable::before,
    .markdown-body h3.clickable::before,
    .markdown-body h4.clickable::before,
    .markdown-body h5.clickable::before,
    .markdown-body h6.clickable::before {
      content: "\25BC"; /* down arrow */
      display: inline-block;
      margin-right: 0.5rem;
      transition: transform 0.3s ease;
      color: #139f87;
      font-size: 1rem;
    }
    .markdown-body h1.clickable.expanded::before,
    .markdown-body h2.clickable.expanded::before,
    .markdown-body h3.clickable.expanded::before,
    .markdown-body h4.clickable.expanded::before,
    .markdown-body h5.clickable.expanded::before,
    .markdown-body h6.clickable.expanded::before {
      transform: rotate(180deg);
    }
    /* Transition for nested container: slide-down & fade-in */
    .nested-container {
      max-height: 0;
      opacity: 0;
      overflow: hidden;
      transition: max-height 0.5s ease, opacity 0.5s ease;
      margin-left: 1rem;
    }
    .nested-container.open {
      max-height: 20000px; /* large enough to fit content */
      opacity: 1;
    }
  </style>
</head>
<body>
  <div class="container markdown-body">
    <button id="toggle-all" class="btn btn-primary btn-sm mb-3">Expand All</button>
    %s
  </div>
  <!-- Optional Bootstrap JS Bundle -->
  <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/js/bootstrap.bundle.min.js"></script>
  <script>
    document.addEventListener("DOMContentLoaded", function() {
      // Adjust paragraph margins based on previous header margins.
      var paragraphs = document.querySelectorAll(".markdown-body p");
      paragraphs.forEach(function(p) {
        var prev = p.previousElementSibling;
        while (prev && !/^H[1-6]$/.test(prev.tagName)) {
          prev = prev.previousElementSibling;
        }
        if (prev) {
          var headerMargin = window.getComputedStyle(prev).marginLeft;
          p.style.marginLeft = headerMargin;
        }
      });
      // Hide nested sections by default and add toggle behavior.
      var headers = document.querySelectorAll('.markdown-body h1, .markdown-body h2, .markdown-body h3, .markdown-body h4, .markdown-body h5, .markdown-body h6');
      headers.forEach(function(header) {
        var level = parseInt(header.tagName.substring(1));
        var sibling = header.nextElementSibling;
        if (!sibling) return;
        var container = document.createElement('div');
        container.classList.add("nested-container");
        while (sibling && (!/^H[1-6]$/.test(sibling.tagName) || parseInt(sibling.tagName.substring(1)) > level)) {
          var next = sibling.nextElementSibling;
          container.appendChild(sibling);
          sibling = next;
        }
        if (container.children.length > 0) {
          header.parentNode.insertBefore(container, header.nextElementSibling);
          header.classList.add("clickable");
          header.style.cursor = "pointer";
          header.addEventListener("click", function() {
            if (container.classList.contains("open")) {
              container.classList.remove("open");
              header.classList.remove("expanded");
            } else {
              container.classList.add("open");
              header.classList.add("expanded");
            }
          });
        }
      });
      // Toggle All functionality.
      var toggleAllButton = document.getElementById("toggle-all");
      if (toggleAllButton) {
        toggleAllButton.addEventListener("click", function() {
          var headers = document.querySelectorAll(".markdown-body .clickable");
          var expandAll = toggleAllButton.textContent.trim() === "Expand All";
          headers.forEach(function(header) {
            var container = header.nextElementSibling;
            if (container && container.classList.contains("nested-container")) {
              if (expandAll) {
                container.classList.add("open");
                header.classList.add("expanded");
              } else {
                container.classList.remove("open");
                header.classList.remove("expanded");
              }
            }
          });
          toggleAllButton.textContent = expandAll ? "Collapse All" : "Expand All";
        });
      } else {
        console.log("Toggle All button not found.");
      }
    });
  </script>
</body>
</html>`, build.Version, buf.String())

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlPage))
	}
}
