package template

import (
	"io/ioutil"
	"strings"

	"mvdan.cc/sh/v3/shell"
)

func RenderTemplate(s string, env func(string) string) (string, error) {
	out, err := shell.Expand(strings.ReplaceAll(s, "`", "\\`"), env)
	if nil != err {
		return "", err
	}

	return out, nil
}

func RenderTemplateFile(t string, env func(string) string) (string, error) {
	data, err := ioutil.ReadFile(t)
	if nil != err {
		return "", err
	}

	return RenderTemplate(string(data), env)
}
