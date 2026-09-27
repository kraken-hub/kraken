package logger

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/gookit/color.v1"
)

type Logger struct {
	out io.Writer
	err io.Writer
	pid int
}

func New(out, err io.Writer) *Logger {
	return &Logger{out: out, err: err, pid: os.Getpid()}
}

func (l *Logger) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(l.out, color.Blue.Sprintf("[%d] %s", l.pid, msg))
}

func (l *Logger) Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(l.out, color.Yellow.Sprintf("[%d] %s", l.pid, msg))
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(l.err, color.Red.Sprintf("[%d] %s", l.pid, msg))
}
