package node

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// cpuUsage is the simplest possible method to extract CPU usage info on most of platforms
// Centrifugo runs. I have not found a more sophisticated cross platform way to extract
// this info without using CGO.
func cpuUsage() (int64, error) {
	cmd := exec.Command("ps", "aux")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return 0, err
	}
	currentPID := os.Getpid()
	for {
		line, err := out.ReadString('\n')
		if err != nil {
			return 0, err
		}
		tokens := strings.Split(line, " ")
		var ft []string
		for _, t := range tokens {
			if t != "" && t != "\t" {
				ft = append(ft, t)
			}
		}
		pid, err := strconv.Atoi(ft[1])
		if err != nil {
			continue
		}
		if pid != currentPID {
			continue
		}
		cpu, err := strconv.ParseFloat(ft[2], 64)
		if err != nil {
			return 0, err
		}
		return int64(cpu), nil
	}
	return 0, errors.New("no cpu info found")
}
