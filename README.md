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
port 3000 is free
```

After sending the signal portkill waits up to `--wait` (3s by default) for
the port to be released and tells you whether it was. With `--force` it
follows up with SIGKILL if the process is still hanging on:

```
$ portkill -yf 8080
PID    USER  PROTO  ADDRESS    COMMAND
51022  dj    tcp6   [::]:8080  java -jar build/libs/api-0.3.1.jar
sent SIGTERM to 51022 (java)
51022 (java) still holds port 8080 after 3s
sent SIGKILL to 51022 (java)
port 8080 is free
```

Several ports and ranges at once, and UDP with `--udp`:

```
$ portkill 3000 5173 8000-8010
$ portkill --udp 5353
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
| `-s`, `--signal SIG` | signal to send: `TERM` (default), `INT`, `HUP`, `QUIT`, `KILL`, `USR1`, `USR2` |
| `-9` | same as `--signal KILL` |
| `-w`, `--wait DUR` | how long to wait for the ports to be free: `3s` (default), `500ms`, `10`; `0` sends the signal and exits |
| `-f`, `--force` | SIGKILL whatever still holds a port after `--wait` |
| `-l`, `--list` | list all listening ports (optionally just the given ones) |
| `--json` | JSON output for `--list` and for showing ports |
| `-u`, `--udp` | include UDP sockets (unconnected sockets bound to a port) |

A port can be a number (`3000`), a range (`8000-8010`) or a comma-separated
list of either (`3000,5173`).
| `-h`, `--help` | show help |
| `-V`, `--version` | print the version |

## Exit status

| code | meaning |
|---|---|
| 0 | the processes were killed and the ports are free; `--list` always exits 0 |
| 1 | nothing is listening on the given ports |
| 2 | usage error, stdin isn't a terminal and `-y` wasn't given, or listing sockets failed |
| 3 | a signal couldn't be sent, a port is still busy after `--wait`, or you answered no |

In a script, 0 and 1 both mean the port is free now:

```sh
portkill -y 3000; [ $? -le 1 ] && npm run dev
```

## How it works

On Linux portkill reads the socket tables in `/proc/net/tcp` and
`/proc/net/tcp6` (plus `udp` and `udp6` with `--udp`) and matches socket
inodes against `/proc/<pid>/fd`. On macOS it runs
`lsof -nP -iTCP -sTCP:LISTEN -F pcLnuR` (and `lsof -nP -iUDP` for UDP), parses
the field output, and asks `ps` for full command lines.

## License

MIT
