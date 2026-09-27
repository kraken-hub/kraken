package diff

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/kylelemons/godebug/diff"
	"gopkg.in/gookit/color.v1"
)

func colorize(str string, c color.Color) string {
	return c.Sprintf("%s", str)
}

func Diff(A, B string) string {

	aLines := strings.Split(A, "\n")
	bLines := strings.Split(B, "\n")

	chunks := diff.DiffChunks(aLines, bLines)

	buf := new(bytes.Buffer)
	for _, c := range chunks {
		for _, line := range c.Added {
			fmt.Fprintf(buf, "%s\n", colorize("+"+line, color.Green))
		}
		for _, line := range c.Deleted {
			fmt.Fprintf(buf, "%s\n", colorize("-"+line, color.Red))
		}
		for _, line := range c.Equal {
			fmt.Fprintf(buf, "%s\n", " "+line)
		}
	}

	return strings.TrimRight(buf.String(), "\n")
}
