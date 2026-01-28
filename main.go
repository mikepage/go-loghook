//go:build linux

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	maxLineSize = 1 << 20  // 1MB
	bufferSize  = 64 << 10 // 64KB
)

type config struct {
	file    string
	pattern *regexp.Regexp
	webhook string
}

func main() {
	cfg := parseFlags()
	log.Printf("Watching %s for /%s/", cfg.file, cfg.pattern)
	if err := watch(cfg); err != nil {
		log.Fatal(err)
	}
}

// parseFlags parses CLI arguments into config
func parseFlags() config {
	file := flag.String("file", "", "Log file to watch")
	pattern := flag.String("pattern", "", "Regex pattern to match")
	webhook := flag.String("webhook", "", "Webhook URL")
	flag.Parse()

	if *file == "" || *pattern == "" || *webhook == "" {
		flag.Usage()
		os.Exit(1)
	}

	re, err := regexp.Compile(*pattern)
	if err != nil {
		log.Fatalf("Invalid pattern: %v", err)
	}

	return config{file: *file, pattern: re, webhook: *webhook}
}

// watch monitors file for changes using inotify
func watch(cfg config) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	dir := filepath.Dir(cfg.file)
	_, err = unix.InotifyAddWatch(fd, dir, unix.IN_MODIFY|unix.IN_CREATE)
	if err != nil {
		return err
	}

	f, err := os.Open(cfg.file)
	if err != nil {
		return err
	}
	defer f.Close()

	f.Seek(0, io.SeekEnd) // Start from end

	target := filepath.Base(cfg.file)
	buf := make([]byte, 4096)

	// Handle signals in background
	done := make(chan struct{})
	go func() {
		<-sig
		close(done)
		unix.Close(fd) // Unblocks read
	}()

	for {
		n, err := unix.Read(fd, buf)
		if err != nil {
			select {
			case <-done:
				return nil
			default:
				return err
			}
		}

		for offset := 0; offset < n; {
			event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
			nameLen := int(event.Len)
			name := ""
			if nameLen > 0 {
				nameBytes := buf[offset+unix.SizeofInotifyEvent : offset+unix.SizeofInotifyEvent+nameLen]
				name = string(bytes.TrimRight(nameBytes, "\x00"))
			}
			offset += unix.SizeofInotifyEvent + nameLen

			if name != target {
				continue
			}
			if event.Mask&unix.IN_MODIFY != 0 {
				processLines(f, cfg)
			}
			if event.Mask&unix.IN_CREATE != 0 { // Log rotation
				f.Close()
				time.Sleep(100 * time.Millisecond)
				f, _ = os.Open(cfg.file)
			}
		}
	}
}

// processLines scans new lines and posts matches
func processLines(f *os.File, cfg config) {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, bufferSize), maxLineSize)
	hostname, _ := os.Hostname()

	for scanner.Scan() {
		if line := scanner.Text(); cfg.pattern.MatchString(line) {
			post(cfg, map[string]string{"hostname": hostname, "line": line})
		}
	}
}

// post sends payload to webhook
func post(cfg config, payload any) {
	data, _ := json.Marshal(payload)
	resp, err := http.Post(cfg.webhook, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("Webhook error: %v", err)
		return
	}
	resp.Body.Close()
}
