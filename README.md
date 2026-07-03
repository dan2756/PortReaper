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

List everything that's listening, sorted by port:

```
$ portkill --list
PORT  PROTO  ADDRESS         PID    USER      COMMAND
22    tcp    0.0.0.0:22      812    root      sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
22    tcp6   [::]:22         812    root      sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
3000  tcp    127.0.0.1:3000  48213  dj        node /home/dj/app/node_modules/.bin/vite --port 3000
5432  tcp    127.0.0.1:5432  1104   postgres  /usr/lib/postgresql/16/bin/postgres -D /var/lib/postgresql/16/main
```

Long command lines are cut to the terminal width; pipe the output to see
them in full.

`--json` prints the same information for scripts. With a port it only shows
the listeners, it never kills:

```
$ portkill --json 3000
[
  {
    "port": 3000,
    "proto": "tcp",
    "address": "127.0.0.1:3000",
    "pid": 48213,
    "ppid": 48190,
    "user": "dj",
    "uid": 1000,
    "name": "node",
    "command": "node /home/dj/app/node_modules/.bin/vite --port 3000"
  }
]
```

`pid` is `null` when the socket's owner isn't visible to you (another user's
process, without sudo).

| flag | |
|---|---|
| `-y`, `--yes` | don't ask; required when stdin isn't a terminal |
| `-l`, `--list` | list all listening ports (optionally just PORT) |
| `--json` | JSON output for `--list` and for showing a port |
| `-h`, `--help` | show help |
| `-V`, `--version` | print the version |

## How it works

On Linux portkill reads the socket tables in `/proc/net/tcp` and
`/proc/net/tcp6` and matches socket inodes against `/proc/<pid>/fd`. On macOS
it runs `lsof -nP -iTCP -sTCP:LISTEN -F pcLnuR` and parses the field output.

## License

MIT
