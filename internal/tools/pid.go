package tools

import (
	"os"
	"strconv"
)

func WritePidFile(pidFile string) error {
	if pidFile == "" {
		return nil
	}
	pid := []byte(strconv.Itoa(os.Getpid()) + "\n")
	return os.WriteFile(pidFile, pid, 0644)
}
