package dotenv

import (
	"context"
	"os"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Load sources a shell env file and returns declared variables.
// Supports comments, blank lines, export keyword, and variable expansion
// within the file (e.g. BASE_URL=http://example.com; API=${BASE_URL}/api).
func Load(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	file, err := syntax.NewParser().Parse(f, path)
	if err != nil {
		return nil, err
	}

	r, err := interp.New(interp.Env(expand.ListEnviron()))
	if err != nil {
		return nil, err
	}
	if err := r.Run(context.Background(), file); err != nil {
		return nil, err
	}

	// Collect only variables explicitly assigned in the file, not interpreter
	// built-ins (IFS, PWD, HOME, OPTIND, etc.) that the shell runner sets itself.
	assigned := map[string]bool{}
	syntax.Walk(file, func(node syntax.Node) bool {
		if a, ok := node.(*syntax.Assign); ok && a.Name != nil {
			assigned[a.Name.Value] = true
		}
		return true
	})

	result := map[string]string{}
	for k, v := range r.Vars {
		if assigned[k] && v.Kind == expand.String && v.IsSet() {
			result[k] = v.Str
		}
	}
	return result, nil
}
