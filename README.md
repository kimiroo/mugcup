# mugcup

The GUI app (`src`) and the CLI app (`src-cli`) are separate Go modules, each
built independently. `src` is the tray UI and IPC server; `src-cli` is only
an IPC client that sends commands to a running `mugcup.exe`.

Build:

```powershell
.\build.ps1
```

Normal use runs from the tray with no console window. The default build
produces two files per architecture folder:

- `mugcup.exe`: the GUI app for normal use. If launched with command-line
  arguments, it doesn't process them — it just shows a warning box pointing
  to `mugcup-cli.exe` and exits.
- `mugcup-cli.exe`: the console CLI app — `cmd.exe` waits for it to finish
  and returns to the prompt.

CLI commands are sent over IPC to a running `mugcup.exe` instance. Every
command except `launch`/`exit` requires `mugcup.exe` to already be running;
if it isn't, the CLI just tells you to run `mugcup-cli launch` first (it
won't start the GUI for you).

```powershell
.\mugcup-cli.exe launch                 # start mugcup (no-op success if already running)
.\mugcup-cli.exe start 1h30m
.\mugcup-cli.exe start indefinite
.\mugcup-cli.exe start preset 0         # pick a preset by index from the configured list
.\mugcup-cli.exe stop
.\mugcup-cli.exe config                 # show the current config
.\mugcup-cli.exe status
.\mugcup-cli.exe export config.json     # save the current config to a file (stdout if omitted)
.\mugcup-cli.exe import config.json     # read a config file and apply it (stdin if omitted)
.\mugcup-cli.exe exit

# options
.\mugcup-cli.exe start 30m -d true      # -d/--display-on <true|false>: explicitly keep the display on or not
.\mugcup-cli.exe status -o json         # -o/--output <text|json>: output format
```
