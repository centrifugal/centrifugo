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
	if err := CreateMarkdownDocumentationWithComments(&conf, configDirs, "internal/cli/configdocs/config.md"); err != nil {
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
// whether the type is complex (struct, pointer/slice of struct) so it should be displayed as an "object".
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

// DocumentStruct iterates over the fields of a struct (using reflection) and writes Markdown
// for each field. The headers are chosen as follows:
//   - If parentKey is empty (i.e. we're documenting the root config), the field headers are printed
//     at the given level (for example "##").
//   - Otherwise, nested fields are printed one level deeper (e.g. if parent fields are "##", then nested fields are "###").
//
// Fields tagged with json:"-" are skipped. If a field is tagged with mapstructure:",squash", then its fields are merged
// into the current level (i.e. the header level does not increase).
func DocumentStruct(cfg interface{}, parentKey string, sb *strings.Builder, comments map[string]string, parentType string, level int) {
	// level is the header level to use for direct children when parentKey is empty.
	// For nested fields (when parentKey is not empty) we add one.
	var fieldLevel int
	if parentKey == "" {
		fieldLevel = level
	} else {
		fieldLevel = level + 1
		if fieldLevel > 5 {
			fieldLevel = 5
		}
	}

	t := reflect.TypeOf(cfg)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Skip fields with json:"-"
		if field.Tag.Get("json") == "-" {
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

		// Print the header for this field.
		sb.WriteString(fmt.Sprintf("%s `%s`\n\n", strings.Repeat("#", fieldLevel), fullKey))

		// Print type info.
		defaultVal := field.Tag.Get("default")
		displayType, isComplex := getDisplayType(field.Type)
		if isComplex {
			sb.WriteString(fmt.Sprintf("Type: `%s` object", displayType))
		} else {
			sb.WriteString(fmt.Sprintf("Type: `%s`", displayType))
		}
		if defaultVal != "" {
			sb.WriteString(fmt.Sprintf(". Default: `%s`", defaultVal))
		}
		sb.WriteString(".\n\n")

		// Print field comment if available.
		if comment, ok := comments[fieldDocKey]; ok && comment != "" {
			// If the comment starts with the Go field name, replace it with the option name in backticks.
			if strings.HasPrefix(comment, field.Name) {
				comment = fmt.Sprintf("`%s`%s", keyTag, comment[len(field.Name):])
			}
			sb.WriteString(comment + "\n\n")
		} else {
			sb.WriteString("No documentation available.\n\n")
		}

		// Recurse into nested structs, pointers to structs, or slices of structs.
		// For squash fields, merge without increasing header level.
		msTag := field.Tag.Get("mapstructure")
		squash := msTag != "" && strings.Contains(msTag, "squash")
		switch field.Type.Kind() {
		case reflect.Struct:
			if field.Type.PkgPath() == "time" && field.Type.Name() == "Time" {
				// Skip time.Time.
			} else {
				nestedType := field.Type
				nestedFullType := getFullTypeName(nestedType)
				nested := reflect.New(nestedType).Interface()
				if squash {
					DocumentStruct(nested, parentKey, sb, comments, parentType, level)
				} else {
					DocumentStruct(nested, fullKey, sb, comments, nestedFullType, fieldLevel)
				}
			}
		case reflect.Ptr:
			if field.Type.Elem().Kind() == reflect.Struct {
				nestedType := field.Type.Elem()
				nestedFullType := getFullTypeName(nestedType)
				nested := reflect.New(nestedType).Interface()
				if squash {
					DocumentStruct(nested, parentKey, sb, comments, parentType, level)
				} else {
					DocumentStruct(nested, fullKey, sb, comments, nestedFullType, fieldLevel)
				}
			}
		case reflect.Slice:
			elemType := field.Type.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				nestedFullType := getFullTypeName(elemType)
				nested := reflect.New(elemType).Interface()
				// For slices, treat like a non-squashed nested struct.
				DocumentStruct(nested, fullKey+"[]", sb, comments, nestedFullType, fieldLevel)
			}
		}
	}
}

// CreateMarkdownDocumentationWithComments ties everything together.
// It extracts comments from the given directories, prints a header for the root configuration,
// and then documents its fields.
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
	header := "Centrifugo configuration options"
	// Print the root config header using level 2.
	sb.WriteString(fmt.Sprintf("%s %s\n\n", strings.Repeat("#", 1), header))
	if comment, ok := comments[topFullType]; ok && comment != "" {
		sb.WriteString(comment + "\n\n")
	} else {
		sb.WriteString("No documentation available.\n\n")
	}
	// Document the fields of the root config.
	DocumentStruct(cfg, "", &sb, comments, topFullType, 2)
	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}
