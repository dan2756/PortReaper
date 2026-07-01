# portkill

Shows and kills whatever is on a port.

"Address already in use" again? `portkill 3000` tells you which process is
sitting on the port, shows its full command line, and asks before sending it
SIGTERM.

## Install

```sh
go install github.com/dan2756/portkill@latest
```

## Usage

```
$ portkill 3000
PID    USER  PROTO  ADDRESS         COMMAND
48213  dj    tcp    127.0.0.1:3000  node /home/dj/app/node_modules/.bin/vite --port 3000
kill 1 process? [y/N] y
sent SIGTERM to 48213 (node)
```

| flag | |
|---|---|
| `-y`, `--yes` | don't ask; required when stdin isn't a terminal |
| `-h`, `--help` | show help |
| `-V`, `--version` | print the version |

## How it works

On Linux portkill reads the socket tables in `/proc/net/tcp` and
`/proc/net/tcp6` and matches socket inodes against `/proc/<pid>/fd`. On macOS
it runs `lsof -nP -iTCP -sTCP:LISTEN -F pcLnuR` and parses the field output.

## License

MIT
