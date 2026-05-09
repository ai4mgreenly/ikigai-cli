# Build and install

This file covers how the project is built and installed for use.
The runtime CLI surface lives in `cli-surface.md`; this file is
about getting the binary onto the developer's machine.

## Install

- R-W23P-398Z: `make install` places the produced `ikigai-cli`
  binary at `~/.local/bin/ikigai-cli`, replacing any existing
  file at that path. The destination directory is created if it
  does not exist. This is a convenience for manual testing — the
  binary lands on a typical user's `PATH` without sudo and
  without touching system directories.
