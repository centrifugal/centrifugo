package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/config"
)

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
	if err := CreateJSONDocumentationWithComments(&conf, configDirs, "internal/cli/configdocs/schema.json"); err != nil {
		fmt.Println("Error writing JSON documentation:", err)
	} else {
		fmt.Println("JSON documentation generated successfully.")
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
// whether the type is complex (struct, pointer/slice of struct) so it should be treated as an "object".
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

// ExtractStructCommentsFromDir parses Go files in the directory (all files in one package)
// and returns a mapping of keys to comments. Top-level struct comments use "pkg.TypeName"
// and field comments use "pkg.TypeName.FieldName".
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

// ExtractStructCommentsFromDirs processes multiple directories and merges the comment maps.
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

// cleanJSONTag returns the first part of a JSON tag (splitting on commas).
func cleanJSONTag(tag string) string {
	if tag == "" {
		return ""
	}
	parts := strings.Split(tag, ",")
	return parts[0]
}

// DocumentStructJSON iterates over the fields of a struct (using reflection) and builds a JSON structure
// for each field. It now correctly increments the level for nested fields.
func DocumentStructJSON(cfg interface{}, parentKey string, parentType string, level int, comments map[string]string) []FieldDoc {
	var docs []FieldDoc

	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return docs
	}

	// Determine the current field level:
	fieldLevel := level
	if parentKey != "" {
		fieldLevel = level + 1
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Skip fields with json:"-"
		if field.Tag.Get("json") == "-" {
			continue
		}

		// Check for squash tag.
		msTag := field.Tag.Get("mapstructure")
		if msTag != "" && strings.Contains(msTag, "squash") {
			var nested interface{}
			if field.Type.Kind() == reflect.Ptr {
				if field.Type.Elem().Kind() == reflect.Struct {
					nested = reflect.New(field.Type.Elem()).Interface()
				}
			} else if field.Type.Kind() == reflect.Struct {
				nested = reflect.New(field.Type).Interface()
			}
			if nested != nil {
				docs = append(docs, DocumentStructJSON(nested, parentKey, parentType, level, comments)...)
			}
			continue
		}

		// Determine the JSON key (fallback to field name if tag is empty).
		keyTag := cleanJSONTag(field.Tag.Get("json"))
		if keyTag == "" || keyTag == "-" {
			keyTag = field.Name
		}
		var fullKey string
		if parentKey == "" {
			fullKey = keyTag
		} else {
			fullKey = parentKey + "." + keyTag
		}

		// Build the key to look up comments: parent's full type + "." + field name.
		fieldDocKey := parentType + "." + field.Name
		if doc, ok := comments[fieldDocKey]; ok && strings.Contains(doc, "NODOC") {
			continue
		}

		// Get default value.
		defaultVal := field.Tag.Get("default")
		// Get type info.
		displayType, isComplex := getDisplayType(field.Type)

		// Get field comment.
		var commentText string
		if comment, ok := comments[fieldDocKey]; ok && comment != "" {
			commentText = comment
		}

		// Use the computed fieldLevel for the current field.
		docEntry := FieldDoc{
			Field:         fullKey,
			Name:          keyTag,
			GoName:        field.Name,
			Level:         fieldLevel, // Use computed fieldLevel here.
			Type:          displayType,
			Default:       defaultVal,
			IsComplexType: isComplex,
			Comment:       commentText,
		}

		// Recurse into nested structs, pointers to structs, or slices of structs.
		var children []FieldDoc
		switch field.Type.Kind() {
		case reflect.Struct:
			if field.Type.PkgPath() == "time" && field.Type.Name() == "Time" {
				// Skip time.Time.
			} else {
				nestedType := field.Type
				nestedFullType := getFullTypeName(nestedType)
				nested := reflect.New(nestedType).Interface()
				children = DocumentStructJSON(nested, fullKey, nestedFullType, fieldLevel, comments)
			}
		case reflect.Ptr:
			if field.Type.Elem().Kind() == reflect.Struct {
				nestedType := field.Type.Elem()
				nestedFullType := getFullTypeName(nestedType)
				nested := reflect.New(nestedType).Interface()
				children = DocumentStructJSON(nested, fullKey, nestedFullType, fieldLevel, comments)
			}
		case reflect.Slice:
			elemType := field.Type.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				nestedFullType := getFullTypeName(elemType)
				nested := reflect.New(elemType).Interface()
				children = DocumentStructJSON(nested, fullKey+"[]", nestedFullType, fieldLevel, comments)
			}
		}

		if len(children) > 0 {
			docEntry.Children = children
		}

		docs = append(docs, docEntry)
	}

	return docs
}

// CreateJSONDocumentationWithComments ties everything together.
// It extracts comments from the given directories, builds the JSON documentation for the root configuration,
// and writes it to the specified output file.
func CreateJSONDocumentationWithComments(cfg interface{}, packageDirs []string, outputPath string) error {
	comments, err := ExtractStructCommentsFromDirs(packageDirs)
	if err != nil {
		return fmt.Errorf("failed to extract comments: %w", err)
	}

	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	topFullType := getFullTypeName(t)

	docs := DocumentStructJSON(cfg, "", topFullType, 1, comments)
	jsonBytes, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return os.WriteFile(outputPath, jsonBytes, 0644)
}
