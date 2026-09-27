package util

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/runtime"
)

// WaitWithDirective waits for a container to stabilise after start.
// It streams the container logs, looking for a line containing
// "@kraken wait <duration>". If found, that duration is used and log
// streaming continues until the timer expires. If the defaultWait
// timeout expires without finding the directive, the function returns.
// Container log lines are printed with blank-line separators around each block.
func WaitWithDirective(kraken *app.Kraken, containerID, containerName string, defaultWait time.Duration) error {
	ctx := kraken.Context()
	rt := kraken.Runtime()
	log := kraken.Logger()
	out := kraken.Out()

	logs, err := rt.ContainerLogs(ctx, containerID, runtime.LogsOptions{
		Follow: true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		// Can't stream logs — fall back to a plain timer.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(defaultWait):
		}
		return nil
	}
	defer logs.Close()

	lineCh := make(chan string, 32)
	go readMuxedLines(logs, lineCh)

	scanTimer := time.NewTimer(defaultWait)
	defer scanTimer.Stop()

	printedAny := false

	// Phase 1: scan logs for @kraken wait directive (up to defaultWait).
	var waitDur time.Duration
	foundDirective := false

	scanLoop:
	for {
		select {
		case <-ctx.Done():
			if printedAny {
				fmt.Fprintln(out)
			}
			return ctx.Err()
		case <-scanTimer.C:
			if printedAny {
				fmt.Fprintln(out)
			}
			return nil
		case line, ok := <-lineCh:
			if !ok {
				if printedAny {
					fmt.Fprintln(out)
				}
				return nil
			}
			after, found := strings.CutPrefix(line, "@kraken wait ")
			if found {
				dur, err := time.ParseDuration(strings.TrimSpace(after))
				if err == nil {
					scanTimer.Stop()
					if printedAny {
						fmt.Fprintln(out)
					}
					waitDur = dur
					foundDirective = true
					break scanLoop
				}
			}
			if line == "" && !printedAny {
				continue // skip leading blank lines
			}
			if !printedAny {
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out, line)
			printedAny = true
		}
	}

	if !foundDirective {
		return nil
	}

	// Phase 2: directive found — print waiting message and continue streaming
	// logs until the wait timer expires.
	log.Info("Waiting %s to make sure `%s` is started.", waitDur, containerName)
	waitTimer := time.NewTimer(waitDur)
	defer waitTimer.Stop()
	printedAny = false

	for {
		select {
		case <-ctx.Done():
			if printedAny {
				fmt.Fprintln(out)
			}
			return ctx.Err()
		case <-waitTimer.C:
			if printedAny {
				fmt.Fprintln(out)
			}
			return nil
		case line, ok := <-lineCh:
			if !ok {
				if printedAny {
					fmt.Fprintln(out)
				}
				return nil
			}
			// Skip any further directives
			if _, found := strings.CutPrefix(line, "@kraken wait "); found {
				continue
			}
			if line == "" && !printedAny {
				continue // skip leading blank lines
			}
			if !printedAny {
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out, line)
			printedAny = true
		}
	}
}

// readMuxedLines parses Docker's multiplexed log stream (8-byte frame header +
// payload) and sends each line to ch, including blank lines. The trailing empty
// string produced by splitting on the frame's terminating \n is not sent. The
// channel is closed when the stream ends.
func readMuxedLines(r io.Reader, ch chan<- string) {
	defer close(ch)
	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(r, hdr); err != nil {
			return
		}
		size := binary.BigEndian.Uint32(hdr[4:8])
		if size == 0 {
			continue
		}
		frame := make([]byte, size)
		if _, err := io.ReadFull(r, frame); err != nil {
			return
		}
		lines := strings.Split(string(frame), "\n")
		// All elements except the last are real lines (blank or not).
		// The last element is the artifact of the trailing \n; only send it
		// if non-empty (frame had no trailing newline).
		for _, line := range lines[:len(lines)-1] {
			ch <- strings.TrimRight(line, "\r")
		}
		if last := strings.TrimRight(lines[len(lines)-1], "\r"); last != "" {
			ch <- last
		}
	}
}
