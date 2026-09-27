package app

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/pkg/dotenv"
	"github.com/kraken-hub/kraken/pkg/eventemitter"
	"github.com/kraken-hub/kraken/pkg/logger"
)

type Kraken struct {
	*eventemitter.EventEmitter

	in  io.ReadCloser
	out io.Writer
	err io.Writer

	context context.Context

	rootCmd *cobra.Command

	runtime runtime.Runtime
}

func (k *Kraken) In() io.ReadCloser            { return k.in }
func (k *Kraken) Out() io.Writer               { return k.out }
func (k *Kraken) Err() io.Writer               { return k.err }
func (k *Kraken) Context() context.Context     { return k.context }
func (k *Kraken) RootCmd() *cobra.Command      { return k.rootCmd }
func (k *Kraken) Runtime() runtime.Runtime     { return k.runtime }
func (k *Kraken) SetRuntime(r runtime.Runtime) { k.runtime = r }
func (k *Kraken) Logger() *logger.Logger       { return logger.New(k.out, k.err) }

func loadEnvFile(path string) {
	vars, err := dotenv.Load(path)
	if err != nil {
		return
	}
	for k, v := range vars {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func loadDefaultEnv() {
	if home, err := os.UserHomeDir(); err == nil {
		loadEnvFile(filepath.Join(home, ".kraken", "env"))
	}
	loadEnvFile("/etc/kraken/env")
}

func New(cmdName, version string) *Kraken {
	loadDefaultEnv()
	initColors()
	kraken := &Kraken{EventEmitter: eventemitter.New()}

	kraken.in = os.Stdin
	kraken.out = os.Stdout
	kraken.err = os.Stderr

	kraken.context = contextWithSigterm(context.Background())

	kraken.rootCmd = NewRootCmd(cmdName, version, func(c *cobra.Command, args []string) error {
		printBanner(kraken.out)
		c.HelpFunc()(c, args)
		return nil
	})
	ApplyDefaultFlags(kraken.rootCmd)

	// Initialise the runtime before any sub-command runs
	kraken.rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		driver := os.Getenv("KRAKEN_RUNTIME")
		host := os.Getenv("KRAKEN_HOST")
		r, err := runtime.New(driver, host)
		if err != nil {
			return err
		}
		kraken.runtime = r
		return nil
	}

	return kraken
}
