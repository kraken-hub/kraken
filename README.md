# Kraken

Docker orchestrator CLI.

## Install

Linux & Darwin/macOS only.

### Quick setup

```sh
curl -fsSL https://raw.githubusercontent.com/kraken-hub/kraken/main/scripts/setup.sh | bash
```

Installs kraken to `~/.kraken/bin`, adds it to your `PATH`, and enables shell completion. Restart your terminal afterwards.

Re-running this script won't upgrade an existing install. To update, rerun `install.sh` against the same directory:

```sh
curl -fsSL https://raw.githubusercontent.com/kraken-hub/kraken/main/scripts/install.sh | bash -s -- --dir ~/.kraken/bin
```

### Binary only

Install just the binary, without touching your shell config:

```sh
curl -fsSL https://raw.githubusercontent.com/kraken-hub/kraken/main/scripts/install.sh | [sudo] bash
```

OR

```sh
curl -fsSL https://raw.githubusercontent.com/kraken-hub/kraken/main/scripts/install.sh | [sudo] bash -s -- [options]
```

Available options:

```
--dir <TARGET-DIR>     Specify a target directory (default: /usr/local/bin, example: ~/.kraken/bin)
--name <FILENAME>      Specify the filename (default: kraken)
--version <VERSION>    Specify an version (default: latest)
```

## Shell completion

Run `kraken completion` for setup instructions (already handled automatically by `setup.sh`).
