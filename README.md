# secrio

Secrio is a local-first secrets manager. Secrets are grouped as vault > project > environment > secret and stored in one encrypted SQLite file per vault, on your machine: no server, no network. The `secrio` command line lets you store them and hand them to a program as environment variables without writing them to disk or to your shell history.

## Build

```sh
go build -o secrio ./cmd/secrio
```

Go 1.26, no CGO.

## Quick start

```sh
secrio vault create work                 # asks for a password (twice), then prints "next: secrio unlock work"
secrio unlock work                       # session: 12h max, 1h idle (--ttl, --idle); creating a vault opens no session
secrio project create hotenb --vault work
secrio env create production --vault work --project hotenb

secrio set DATABASE_URL --vault work --project hotenb --env production   # masked prompt
secrio list --vault work --project hotenb --env production               # names only
secrio get DATABASE_URL --vault work --project hotenb --env production
secrio run --vault work --project hotenb --env production -- ./deploy.sh
secrio lock work
```

For scripts, set `SECRIO_PASSWORD` instead of running `unlock` (`vault create` then uses it without prompting, and `project create` and `env create` work right after it); if `SECRIO_PASSWORD` is set it is used whenever no usable session exists, including when the OS keychain is unavailable. The vault is opened with it for that one command and no session is created:

```sh
printf %s "$DB_URL" | SECRIO_PASSWORD=... secrio set DATABASE_URL
```

## Commands

| Command | What it does |
|---|---|
| `vault create <name>` | Create a vault (name: letters, digits, `.`, `_`, `-`, max 64; not starting with `.`, not containing `..`, not a reserved Windows name such as `con` or `com1`) |
| `vault list` | List vault names |
| `unlock [vault]` | Start a session (`--ttl` max lifetime, default 12h, must be positive; `--idle` idle timeout, default 1h, `--idle 0` disables it) |
| `lock [vault]` | End the session |
| `project create <name>` | Create a project in the context vault |
| `env create <name>` | Create an environment in the context project |
| `list` | List projects, environments or secret names, depending on how much context is set; never values |
| `set <NAME>` | Store a secret; the value comes from a masked prompt or from stdin |
| `get <NAME>` | Print the value (no trailing newline unless stdout is a terminal) |
| `run [flags] -- <command> [args...]` | Run a command with the environment's secrets as environment variables |
| `completion <shell>` | Shell completion script |

Global flags: `--vault`, `--project`, `--env`. `secrio --version` prints the version.

## Context: `.secrio.toml`

Instead of repeating the flags, put a `.secrio.toml` in your project directory:

```toml
vault = "work"
project = "hotenb"
env = "production"
```

Each of `vault`, `project`, `env` is resolved separately. Priority: flags, then `SECRIO_VAULT` / `SECRIO_PROJECT` / `SECRIO_ENV`, then the nearest `.secrio.toml`, searched from the current directory upwards. Unknown keys are an error. The data directory is `SECRIO_HOME` if set, otherwise the platform default.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Error |
| 2 | Usage error |
| 3 | Vault locked, wrong password, or no password available without a terminal |

Under `run`, secrio's own failures before the command starts (including a locked vault, a wrong password, a missing vault/project/env context and a missing `--` or command) exit 125, not 3 or 2, so they cannot be mistaken for the child's own exit code. The one exception: a flag-parse error or an unknown command at the root (for example `run --nosuch -- true`) is still a usage error, exit 2. Otherwise `run` returns the child's own exit code; 125 if secrio failed before starting the command; 126 if it could not be executed; 127 if it was not found; 128+N if it was killed by signal N.

## Security notes

- A value never goes in argv: `set` reads a masked prompt (or stdin when it is not a terminal) and has no value argument or flag, so nothing shows in `ps` or shell history. Piped input drops one trailing newline; an empty value is refused.
- `get` and `run` are the only commands that reveal values. `run` removes `SECRIO_PASSWORD` from the child's environment and closes the vault before the child starts.
- The OS keychain holds only a random session key, never the password or the vault key.
- Without a terminal, secrio never prompts: it fails (exit 3 for a missing password).
- `SECRIO_PASSWORD` is visible to processes of your user (`/proc`, `ps eww`); prefer `unlock` on a shared machine.
- Threat model and limits: [docs/architecture.md](docs/architecture.md).

## Current limits

No import, export, edit or delete command yet, and no desktop app. `unlock` needs a working OS keychain; other commands can use `SECRIO_PASSWORD` without one.

- Two concurrent `vault create` runs can leave an orphaned vault file: the registry is last-writer-wins across processes.
- A project or environment provided by `.secrio.toml` cannot be cleared from the command line: an empty `--project ""` counts as unset and the file's value applies.
- Vault, project and environment names cannot contain control characters (newline, ESC, ...).
- Vault names are case-insensitive on macOS and Windows file systems, so `vault create` refuses a name that differs from an existing one only by case; reserved Windows names (`con`, `prn`, `aux`, `nul`, `com1`-`com9`, `lpt1`-`lpt9`, also with an extension) and `..` inside a name are refused everywhere.
- `SECRIO_PASSWORD` exported in a shell profile costs one Argon2id run (about a second and 256 MiB of memory) for every command that has no session.
