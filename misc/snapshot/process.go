package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"syscall"
	"time"
)

// process is a running Centrifugo server with its output captured line by line.
type process struct {
	cmd *exec.Cmd

	mu     sync.Mutex
	lines  []string
	notify chan struct{}

	exited   chan struct{}
	exitCode int
}

func startProcess(binary string, args []string, dir string, env []string) (*process, error) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	p := &process{cmd: cmd, notify: make(chan struct{}, 1), exited: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
		for sc.Scan() {
			p.mu.Lock()
			p.lines = append(p.lines, sc.Text())
			p.mu.Unlock()
			p.signal()
		}
		_, _ = io.Copy(io.Discard, pr)
	}()
	go func() {
		err := cmd.Wait()
		_ = pw.Close()
		<-scanDone
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
				if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
					code = 128 + int(ws.Signal())
				}
			} else {
				code = -1
			}
		}
		p.mu.Lock()
		p.exitCode = code
		p.mu.Unlock()
		close(p.exited)
		p.signal()
	}()
	return p, nil
}

func (p *process) signal() {
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

func (p *process) numLines() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.lines)
}

func (p *process) linesFrom(i int) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i > len(p.lines) {
		return nil
	}
	return append([]string(nil), p.lines[i:]...)
}

func (p *process) isExited() bool {
	select {
	case <-p.exited:
		return true
	default:
		return false
	}
}

// waitLine waits for a line at index >= from whose log message matches any
// of the patterns. It returns the line index or -1.
func (p *process) waitLine(from int, patterns []*regexp.Regexp, timeout time.Duration) (int, string) {
	deadline := time.After(timeout)
	next := from
	for {
		lines := p.linesFrom(next)
		for i, l := range lines {
			msg := logMessage(l)
			for _, re := range patterns {
				if re.MatchString(msg) {
					return next + i, l
				}
			}
		}
		next += len(lines)
		if p.isExited() && len(p.linesFrom(next)) == 0 {
			return -1, ""
		}
		select {
		case <-p.notify:
		case <-deadline:
			return -1, ""
		}
	}
}

func (p *process) sendSignal(sig os.Signal) error {
	return p.cmd.Process.Signal(sig)
}

// wait waits for the process to exit, killing it after timeout.
func (p *process) wait(timeout time.Duration) (code int, killed bool) {
	select {
	case <-p.exited:
	case <-time.After(timeout):
		_ = p.cmd.Process.Kill()
		<-p.exited
		killed = true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode, killed
}
