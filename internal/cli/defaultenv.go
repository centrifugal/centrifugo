package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/config/envconfig"

	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/spf13/cobra"
)

func DefaultEnv() *cobra.Command {
	var baseConfigFile string
	var defaultEnvCmd = &cobra.Command{
		Use:   "defaultenv",
		Short: "Generate full environment var list with defaults",
		Long:  `Generate full Centrifugo environment var list with defaults`,
		Run: func(cmd *cobra.Command, args []string) {
			defaultEnv(baseConfigFile)
		},
	}
	defaultEnvCmd.Flags().StringVarP(&baseConfigFile, "base", "b", "", "path to the base config file to use")
	return defaultEnvCmd
}

func defaultEnv(baseFile string) {
	conf, meta, err := config.GetConfig(nil, baseFile)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if err = conf.Validate(); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	printSortedEnvVars(meta.KnownEnvVars)
}

func printSortedEnvVars(knownEnvVars map[string]envconfig.VarInfo) {
	var envKeys []string
	for env := range knownEnvVars {
		envKeys = append(envKeys, env)
	}
	sort.Strings(envKeys)
	for _, env := range envKeys {
		if strings.HasSuffix(env, "-") {
			// Hacky way to skip unnecessary struct field, can be based on struct tag.
			continue
		}
		fmt.Printf("%s=%s\n", env, valueToStringReflect(knownEnvVars[env].Field))
	}
}

// valueToStringReflect converts a reflect.Value to a string in a way suitable for environment variables.
func valueToStringReflect(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		// Check if the element type of the slice/array is a struct
		if v.Type().Elem().Kind() == reflect.Struct {
			// Marshal the entire array/slice of structs to JSON
			jsonValue, err := json.Marshal(v.Interface())
			if err != nil {
				panic(err)
			}
			// Escape double quotes to make the value suitable for environment variables
			return fmt.Sprintf("\"%v\"", strings.ReplaceAll(string(jsonValue), `"`, `\"`))
		}
		var elements []string
		for i := 0; i < v.Len(); i++ {
			elements = append(elements, valueToStringReflect(v.Index(i)))
		}
		if len(elements) == 0 {
			return "\"\""
		}
		return fmt.Sprintf("%v", strings.Join(elements, " "))
	case reflect.Struct:
		// You can customize how structs should be serialized if needed
		return fmt.Sprintf("%v", v.Interface()) // Fallback to default formatting (customize if necessary)
	case reflect.Map:
		jsonValue, err := json.Marshal(v.Interface())
		if err != nil {
			panic(err)
		}
		// Escape double quotes to make the value suitable for environment variables
		return fmt.Sprintf("\"%v\"", strings.ReplaceAll(string(jsonValue), `"`, `\"`))
	case reflect.Ptr:
		if v.IsNil() {
			return ""
		}
		return valueToStringReflect(v.Elem()) // Dereference the pointer and recursively process
	case reflect.Invalid:
		return "" // Handle zero/nil values
	case reflect.String:
		return fmt.Sprintf("\"%v\"", v.Interface())
	default:
		// Fallback for other types (int, bool, etc.)
		return fmt.Sprintf("%v", v.Interface())
	}
}
