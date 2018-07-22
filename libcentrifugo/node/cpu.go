// +build !windows

package node

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
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
}

// And here is another implementation using syscall
func cpuUsageSeparated() (int64, int64, error) {
	rusage := &syscall.Rusage{}
	err := syscall.Getrusage(syscall.RUSAGE_SELF, rusage)
	if nil != err {
		return 0, 0, fmt.Errorf("getrusage: %v", err)
	}

	return syscall.TimevalToNsec(rusage.Utime), syscall.TimevalToNsec(rusage.Stime), nil
}
