package tools

import (
	"os"
)

func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return false // File does not exist.
		}
	}
	return true // File exists.
}
