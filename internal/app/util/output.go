package util

import (
	"encoding/json"
	"io"
)

// EncodeJSON writes v to w as indented JSON.
func EncodeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")
	return encoder.Encode(v)
}
