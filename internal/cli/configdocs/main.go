package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/config"
)

func main() {
	conf, _, err := config.GetConfig(nil, "")
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if err = conf.Validate(); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	configDirs := []string{"internal/config", "internal/configtypes"}
	if err := CreateMarkdownDocumentationWithComments(&conf, configDirs, "internal/configdocs/config.md"); err != nil {
		fmt.Println("Error writing Markdown:", err)
	} else {
		fmt.Println("Markdown documentation generated successfully.")
	}
}

// formatTypeName removes the "configtypes." prefix from a type string.
func formatTypeName(typeStr string) string {
	return strings.ReplaceAll(typeStr, "configtypes.", "")
}

// getFullTypeName returns a fully qualified type name in the form "pkg.TypeName".
func getFullTypeName(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	pkgPath := t.PkgPath()
	parts := strings.Split(pkgPath, "/")
	pkgName := parts[len(parts)-1]
	return pkgName + "." + t.Name()
}

// getDisplayType returns the base type name (without package prefix) and a boolean indicating
// whether the type is non-primitive (struct, pointer/slice of struct) that should be displayed with " object".
func getDisplayType(t reflect.Type) (string, bool) {
	// Handle pointer to struct (except time.Time).
	if t.Kind() == reflect.Ptr {
		if t.Elem().Kind() == reflect.Struct {
			if t.Elem().PkgPath() == "time" && t.Elem().Name() == "Time" {
				return formatTypeName(t.String()), false
			}
			return t.Elem().Name(), true
		}
	}
	// Handle struct (except time.Time).
	if t.Kind() == reflect.Struct {
		if t.PkgPath() == "time" && t.Name() == "Time" {
			return formatTypeName(t.String()), false
		}
		return t.Name(), true
	}
	// Handle slice of structs.
	if t.Kind() == reflect.Slice {
		elem := t.Elem()
		if elem.Kind() == reflect.Ptr {
			if elem.Elem().Kind() == reflect.Struct {
				if elem.Elem().PkgPath() == "time" && elem.Elem().Name() == "Time" {
					return formatTypeName(t.String()), false
				}
				return "[]" + elem.Elem().Name(), true
			}
		}
		if elem.Kind() == reflect.Struct {
			if elem.PkgPath() == "time" && elem.Name() == "Time" {
				return formatTypeName(t.String()), false
			}
			return "[]" + elem.Name(), true
		}
	}
	// For other types, just return the cleaned type string.
	return formatTypeName(t.String()), false
}

// ExtractStructCommentsFromDir parses all Go files in the directory (which should belong to one package)
// and returns a mapping of fully qualified keys to comments.
// Top-level struct comments are stored under "pkg.TypeName" and field comments under "pkg.TypeName.FieldName".
func ExtractStructCommentsFromDir(dir string) (map[string]string, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	comments := make(map[string]string)
	for _, pkg := range pkgs {
		pkgName := pkg.Name
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.TYPE {
					continue
				}
				for _, spec := range genDecl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					fullTypeKey := pkgName + "." + typeSpec.Name.Name
					if genDecl.Doc != nil {
						comments[fullTypeKey] = strings.TrimSpace(genDecl.Doc.Text())
					}
					for _, field := range structType.Fields.List {
						fieldComment := ""
						if field.Doc != nil {
							fieldComment = strings.TrimSpace(field.Doc.Text())
						} else if field.Comment != nil {
							fieldComment = strings.TrimSpace(field.Comment.Text())
						}
						for _, name := range field.Names {
							key := fullTypeKey + "." + name.Name
							comments[key] = fieldComment
						}
					}
				}
			}
		}
	}
	return comments, nil
}

// ExtractStructCommentsFromDirs accepts multiple directories and combines the comment maps.
func ExtractStructCommentsFromDirs(dirs []string) (map[string]string, error) {
	combined := make(map[string]string)
	for _, dir := range dirs {
		cmnts, err := ExtractStructCommentsFromDir(dir)
		if err != nil {
			return nil, err
		}
		for k, v := range cmnts {
			combined[k] = v
		}
	}
	return combined, nil
}

// cleanJSONTag splits the JSON tag on commas and returns only the first part.
func cleanJSONTag(tag string) string {
	if tag == "" {
		return ""
	}
	parts := strings.Split(tag, ",")
	return parts[0]
}

// GenerateMarkdownForConfigWithComments recursively walks the configuration struct using reflection.
// It outputs a Markdown document with hierarchical keys (dot‑notation).
// If a field is tagged with `mapstructure:",squash"`, its options are merged into the parent level.
// Before outputting any section or field, the code checks if its comment (if any) contains "NODOC".
// If so, that section or field is skipped.
// Additionally, if a field comment starts with the Go field name, that prefix is replaced by the option name (from the JSON tag, in backticks).
func GenerateMarkdownForConfigWithComments(cfg interface{}, parentPath string, sb *strings.Builder, comments map[string]string, fullType string) {
	// Skip documentation if the struct's comment contains "NODOC".
	if cm, ok := comments[fullType]; ok && strings.Contains(cm, "NODOC") {
		return
	}

	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	if parentPath == "" {
		sb.WriteString(fmt.Sprintf("## %s\n\n", fullType))
		if comment, ok := comments[fullType]; ok && comment != "" {
			sb.WriteString(comment + "\n\n")
		} else {
			sb.WriteString("No documentation available.\n\n")
		}
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		msTag := field.Tag.Get("mapstructure")
		squash := msTag != "" && strings.Contains(msTag, "squash")
		if squash {
			fieldDocKey := fullType + "." + field.Name
			if doc, ok := comments[fieldDocKey]; ok && strings.Contains(doc, "NODOC") {
				continue
			}
			switch field.Type.Kind() {
			case reflect.Struct:
				if field.Type.PkgPath() == "time" && field.Type.Name() == "Time" {
					// Skip time.Time.
				} else {
					nestedType := field.Type
					nestedFullType := getFullTypeName(nestedType)
					nested := reflect.New(nestedType).Interface()
					GenerateMarkdownForConfigWithComments(nested, parentPath, sb, comments, nestedFullType)
				}
			case reflect.Ptr:
				if field.Type.Elem().Kind() == reflect.Struct {
					nestedType := field.Type.Elem()
					nestedFullType := getFullTypeName(nestedType)
					nested := reflect.New(nestedType).Interface()
					GenerateMarkdownForConfigWithComments(nested, parentPath, sb, comments, nestedFullType)
				}
			case reflect.Slice:
				elemType := field.Type.Elem()
				if elemType.Kind() == reflect.Ptr {
					elemType = elemType.Elem()
				}
				if elemType.Kind() == reflect.Struct {
					nestedFullType := getFullTypeName(elemType)
					nested := reflect.New(elemType).Interface()
					GenerateMarkdownForConfigWithComments(nested, parentPath, sb, comments, nestedFullType)
				}
			}
			continue
		}

		// Use the "json" tag as the option name and clean it.
		keyTag := cleanJSONTag(field.Tag.Get("json"))
		if keyTag == "" || keyTag == "-" {
			keyTag = field.Name
		}
		fullKey := keyTag
		if parentPath != "" {
			fullKey = parentPath + "." + keyTag
		}

		fieldDocKey := fullType + "." + field.Name
		if doc, ok := comments[fieldDocKey]; ok && strings.Contains(doc, "NODOC") {
			continue
		}

		defaultVal := field.Tag.Get("default")
		displayType, isComplex := getDisplayType(field.Type)
		// If the type is non-primitive, display as "`TypeName` object", otherwise as "`TypeName`".
		if isComplex {
			sb.WriteString(fmt.Sprintf("### %s\n\n", fullKey))
			sb.WriteString(fmt.Sprintf("Type: `%s` object", displayType))
		} else {
			sb.WriteString(fmt.Sprintf("### %s\n\n", fullKey))
			sb.WriteString(fmt.Sprintf("Type: `%s`", displayType))
		}
		if defaultVal != "" {
			sb.WriteString(fmt.Sprintf(". Default: `%s`", defaultVal))
		}
		sb.WriteString(".\n\n")

		if comment, ok := comments[fieldDocKey]; ok && comment != "" {
			// If the comment starts with the Go field name, replace that prefix with the option name in backticks.
			if strings.HasPrefix(comment, field.Name) {
				comment = fmt.Sprintf("`%s`%s", keyTag, comment[len(field.Name):])
			}
			sb.WriteString(comment + "\n\n")
		} else {
			sb.WriteString("No documentation available.\n\n")
		}

		switch field.Type.Kind() {
		case reflect.Struct:
			if field.Type.PkgPath() == "time" && field.Type.Name() == "Time" {
				// Do not recurse into time.Time.
			} else {
				nestedType := field.Type
				nestedFullType := getFullTypeName(nestedType)
				nested := reflect.New(nestedType).Interface()
				GenerateMarkdownForConfigWithComments(nested, fullKey, sb, comments, nestedFullType)
			}
		case reflect.Ptr:
			if field.Type.Elem().Kind() == reflect.Struct {
				nestedType := field.Type.Elem()
				nestedFullType := getFullTypeName(nestedType)
				nested := reflect.New(nestedType).Interface()
				GenerateMarkdownForConfigWithComments(nested, fullKey, sb, comments, nestedFullType)
			}
		case reflect.Slice:
			elemType := field.Type.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				nestedFullType := getFullTypeName(elemType)
				nested := reflect.New(elemType).Interface()
				GenerateMarkdownForConfigWithComments(nested, fullKey+"[]", sb, comments, nestedFullType)
			}
		}
	}
}

// CreateMarkdownDocumentationWithComments ties everything together.
// It accepts the configuration instance (e.g. of type Config), a slice of package directories,
// and the output path for the Markdown file.
func CreateMarkdownDocumentationWithComments(cfg interface{}, packageDirs []string, outputPath string) error {
	comments, err := ExtractStructCommentsFromDirs(packageDirs)
	if err != nil {
		return fmt.Errorf("failed to extract comments: %w", err)
	}

	var sb strings.Builder
	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	topFullType := getFullTypeName(t)
	GenerateMarkdownForConfigWithComments(cfg, "", &sb, comments, topFullType)
	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}
