package docker

import (
	"context"
	"fmt"
	"os"
	ossignal "os/signal"
	"runtime"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/moby/sys/signal"
	"github.com/moby/term"
)

// resizeTtyTo resizes tty to specific height and width
func resizeTtyTo(ctx context.Context, client client.ContainerAPIClient, id string, height, width uint, isExec bool) error {
	if height == 0 && width == 0 {
		return nil
	}

	options := container.ResizeOptions{
		Height: height,
		Width:  width,
	}

	var err error
	if isExec {
		err = client.ContainerExecResize(ctx, id, options)
	} else {
		err = client.ContainerResize(ctx, id, options)
	}
	return err
}

// resizeTty is to resize the tty with cli out's tty size
func resizeTty(ctx context.Context, client client.ContainerAPIClient, std interface{}, id string, isExec bool) error {
	height, width := GetTtySize(std)
	return resizeTtyTo(ctx, client, id, height, width, isExec)
}

// initTtySize is to init the tty's size to the same as the window, if there is an error, it will retry 10 times.
func initTtySize(ctx context.Context, client client.ContainerAPIClient, std interface{}, id string, isExec bool, resizeTtyFunc func(ctx context.Context, client client.ContainerAPIClient, std interface{}, id string, isExec bool) error) {
	rttyFunc := resizeTtyFunc
	if rttyFunc == nil {
		rttyFunc = resizeTty
	}
	if err := rttyFunc(ctx, client, std, id, isExec); err != nil {
		go func() {
			var err error
			for retry := 0; retry < 10; retry++ {
				time.Sleep(time.Duration(retry+1) * 10 * time.Millisecond)
				if err = rttyFunc(ctx, client, std, id, isExec); err == nil {
					break
				}
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "failed to resize tty, using default size")
			}
		}()
	}
}

// MonitorTtySize updates the container tty size when the terminal tty changes size
func MonitorTtySize(ctx context.Context, client client.ContainerAPIClient, std interface{}, id string, isExec bool) error {
	initTtySize(ctx, client, std, id, isExec, resizeTty)
	if runtime.GOOS == "windows" {
		go func() {
			prevH, prevW := GetTtySize(std)
			for {
				time.Sleep(time.Millisecond * 250)
				h, w := GetTtySize(std)

				if prevW != w || prevH != h {
					resizeTty(ctx, client, std, id, isExec)
				}
				prevH = h
				prevW = w
			}
		}()
	} else {
		sigchan := make(chan os.Signal, 1)
		ossignal.Notify(sigchan, signal.SIGWINCH)
		go func() {
			for range sigchan {
				resizeTty(ctx, client, std, id, isExec)
			}
		}()
	}
	return nil
}

func GetTtySize(std interface{}) (height uint, width uint) {
	fd, tty := term.GetFdInfo(std)
	if !tty {
		return 0, 0
	}
	ws, err := term.GetWinsize(fd)
	if err != nil {
		fmt.Println("Error getting TTY size")
		if ws == nil {
			return 0, 0
		}
	}
	return uint(ws.Height), uint(ws.Width)
}
