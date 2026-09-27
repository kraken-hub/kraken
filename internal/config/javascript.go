package config

import (
	"strings"

	"github.com/grafana/sobek"
)

const jsTemplate = `(function() { return JSON.stringify((%JS_CODE%)(JSON.parse(__JSON__))); })();`

func RunJS(code, json string) (string, error) {
	vm := sobek.New()

	vm.Set("__JSON__", json)

	val, err := vm.RunString(strings.Replace(jsTemplate, "%JS_CODE%", code, 1))
	if err != nil {
		return "", err
	}

	return val.String(), nil
}
