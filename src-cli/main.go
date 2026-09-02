package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Version and BuildVariant are set at build time via -ldflags
// "-X main.Version=1.2.3 -X main.BuildVariant=stable" — build.ps1 does this
// automatically from version.yaml, same as it does for mugcup.exe.
var (
	Version      = "dev"
	BuildVariant = "dev"
)

type options struct {
	displayOn       *bool
	autoStart       *bool
	autoUpdateCheck *bool
	autoUpdateApply *bool
	language        *string // "auto", "en", or "ko" — see mugcup/i18n on the GUI side
	trayClickAction *string // "cycle", "indefinite", or "menu" — see settings.TrayClickAction
	yes             bool    // -y/--yes: skip the "update" command's [y/N] confirmation
	noUpdate        bool    // --no-update: only consumed by "launch" (see runLaunch)
	output          string  // "text" (default) or "json"
}

// hasSetting reports whether any settings-only option was given, for
// validating the standalone "set" command has something to do.
func (o options) hasSetting() bool {
	return o.displayOn != nil || o.autoStart != nil || o.autoUpdateCheck != nil || o.autoUpdateApply != nil || o.language != nil || o.trayClickAction != nil
}

// asRequest carries this options' settings-only fields into a Request,
// alongside whatever else the caller sets (Command, Args, ...).
func (o options) asRequest() Request {
	return Request{
		DisplayOn:       o.displayOn,
		AutoStart:       o.autoStart,
		AutoUpdateCheck: o.autoUpdateCheck,
		AutoUpdateApply: o.autoUpdateApply,
		Language:        o.language,
		TrayClickAction: o.trayClickAction,
	}
}

func printHelp() {
	fmt.Print(`mugcup-cli - CLI for controlling the running mugcup tray app

Usage:
  mugcup-cli <command> [args] [options]

Commands:
  launch                  Start mugcup (no-op success if already running).
                            --no-update skips its startup auto-update check
  start <time>             Keep on for the given duration (e.g. 30m, 1h, 2h15m). Required — no default
  start indefinite         Keep on indefinitely
  start preset <n>         Start the n-th preset from the configured list (0-based)
  start until <time>       Keep on until the given time today (e.g. 18:00), a specific date & time
                            (e.g. 2026-01-02 18:00, quoted if it has a space), or an RFC3339
                            timestamp (e.g. 2026-01-02T18:00:00+09:00). Must be in the future
  stop                     Turn off the timer and keep-on
  set                      Change settings that aren't tied to a running timer (see Options below).
                            Requires at least one of -d, --auto-start, --auto-update-check,
                            --auto-update-apply, --language, --tray-click-action
  config                   Show the current config
  settings                 Open the settings window
  status                   Show the current status
  update                   Check for updates, and if one is found, ask [y/N] before installing
                            (-y/--yes skips the prompt and installs immediately)
  exit                     Exit the running mugcup
  import <path.json>       Read a config file and apply it (stdin if omitted)
  export <path.json>       Save the current config to a file (stdout if omitted)
  version                  Show mugcup-cli's own version and build variant
  help                     Show this help

Options:
  -d, --display-on <true|false>      Also keep the display on. Persists; unlike the others, only has an
                                     on-screen effect while a timer is running (omit to keep the current setting)
  --auto-start <true|false>          Start mugcup automatically with Windows
  --auto-update-check <true|false>   Automatically check for updates
  --auto-update-apply <true|false>   Install a found update without asking (only takes effect if
                                     auto-update-check is also on)
  --language <auto|en|ko>            Set the GUI's display language (tray menu, popup window, dialogs).
                                     Doesn't affect mugcup-cli's own output, which stays English-only
  --tray-click-action <cycle|indefinite|menu>
                                     What a tray left-click does: cycle through presets, toggle
                                     indefinite keep-on, or just open the tray menu
  -y, --yes                          Skip the "update" command's [y/N] confirmation
  --no-update                        With "launch": start mugcup without its startup auto-update check
  -o, --output <text|json>           Output format. start/stop/set/status/config/export/import
                                     print the result as multiple lines (text) or just that value (json)

Examples:
  mugcup-cli launch
  mugcup-cli launch --no-update
  mugcup-cli start 1h30m
  mugcup-cli start preset 0
  mugcup-cli start until 18:00
  mugcup-cli start until "2026-01-02 18:00"
  mugcup-cli start indefinite -d true
  mugcup-cli set --auto-start true --auto-update-apply false
  mugcup-cli set --language ko
  mugcup-cli set --tray-click-action cycle
  mugcup-cli status -o json
  mugcup-cli export config.json
  mugcup-cli update
  mugcup-cli update -y
  mugcup-cli exit
`)
}

// supportedLanguages are the values --language accepts, in the same order
// mugcup/i18n ships catalogs for.
var supportedLanguages = []string{"auto", "en", "ko"}

// supportedTrayClickActions are the values --tray-click-action accepts —
// see settings.TrayClickAction on the GUI side.
var supportedTrayClickActions = []string{"cycle", "indefinite", "menu"}

func contains(list []string, v string) bool {
	for _, s := range list {
		if v == s {
			return true
		}
	}
	return false
}

// parseBoolFlag consumes name's required true/false value at args[*i+1],
// advancing *i past it.
func parseBoolFlag(args []string, i *int, name string) (*bool, error) {
	if *i+1 >= len(args) {
		return nil, fmt.Errorf("%s requires a true/false value", name)
	}
	*i++
	v, err := parseBool(args[*i])
	if err != nil {
		return nil, fmt.Errorf("invalid value for %s: %s", name, args[*i])
	}
	return &v, nil
}

// parseEnumFlag consumes name's required value at args[*i+1] (lowercased),
// advancing *i past it, and exits via failUnsupportedValue if it isn't one
// of supported.
func parseEnumFlag(opts options, args []string, i *int, name string, supported []string, field, jsonListKey string) (*string, error) {
	if *i+1 >= len(args) {
		return nil, fmt.Errorf("%s requires a value (%s)", name, strings.Join(supported, "/"))
	}
	*i++
	v := strings.ToLower(args[*i])
	if !contains(supported, v) {
		failUnsupportedValue(opts, field, jsonListKey, supported, args[*i])
	}
	return &v, nil
}

// failUnsupportedValue rejects an unrecognized enum-flag value (--language,
// --tray-click-action, ...) and exits with the supported list — as JSON
// under jsonListKey, or plain text. opts.output must already be resolved
// (see outputFormat) since this can fire before -o itself is parsed.
func failUnsupportedValue(opts options, field, jsonListKey string, supported []string, got string) {
	if opts.output == "json" {
		printJSON(map[string]any{
			"success":   false,
			"message":   fmt.Sprintf("unsupported %s: %s", field, got),
			jsonListKey: supported,
		})
		os.Exit(1)
	}
	fail("unsupported %s: %s\nSupported values: %s", field, got, strings.Join(supported, ", "))
}

// outputFormat pre-scans args for -o/--output, so an enum flag earlier in
// the command line can still fail as JSON. Falls back to "text"; the main
// parse loop below still validates -o itself.
func outputFormat(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" || args[i] == "--output" {
			v := strings.ToLower(args[i+1])
			if v == "text" || v == "json" {
				return v
			}
		}
	}
	return "text"
}

func parseOptions(args []string) (options, []string, error) {
	opts := options{output: outputFormat(args)}
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-d", "--display-on":
			v, err := parseBoolFlag(args, &i, a)
			if err != nil {
				return opts, nil, err
			}
			opts.displayOn = v
		case "--auto-start":
			v, err := parseBoolFlag(args, &i, a)
			if err != nil {
				return opts, nil, err
			}
			opts.autoStart = v
		case "--auto-update-check":
			v, err := parseBoolFlag(args, &i, a)
			if err != nil {
				return opts, nil, err
			}
			opts.autoUpdateCheck = v
		case "--auto-update-apply":
			v, err := parseBoolFlag(args, &i, a)
			if err != nil {
				return opts, nil, err
			}
			opts.autoUpdateApply = v
		case "--language":
			v, err := parseEnumFlag(opts, args, &i, a, supportedLanguages, "language", "supportedLanguages")
			if err != nil {
				return opts, nil, err
			}
			opts.language = v
		case "--tray-click-action":
			v, err := parseEnumFlag(opts, args, &i, a, supportedTrayClickActions, "tray click action", "supportedTrayClickActions")
			if err != nil {
				return opts, nil, err
			}
			opts.trayClickAction = v
		case "-o", "--output":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("%s requires a text/json value", a)
			}
			i++
			v := strings.ToLower(args[i])
			if v != "text" && v != "json" {
				return opts, nil, fmt.Errorf("invalid value for %s (must be text or json): %s", a, args[i])
			}
			opts.output = v
		case "-y", "--yes":
			opts.yes = true
		case "--no-update":
			opts.noUpdate = true
		default:
			positional = append(positional, a)
		}
	}
	return opts, positional, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("must be true or false")
	}
}

// printResult renders a response per -o. A successful response carrying a
// payload (status/config) prints only that payload — as human-readable text
// or as bare JSON — since message would just restate the same fields in
// prose. Responses without a payload (launch/exit/...) and all failures
// print message instead.
func printResult(opts options, resp Response) {
	if !resp.Success {
		if opts.output == "json" {
			printJSON(map[string]any{"success": false, "message": resp.Message})
		} else {
			fmt.Println(resp.Message)
		}
		os.Exit(1)
	}

	switch {
	case resp.Status != nil:
		if opts.output == "json" {
			printJSON(resp.Status)
		} else {
			fmt.Println(renderStatusText(resp.Status))
		}
	case resp.Config != nil:
		if opts.output == "json" {
			printJSON(resp.Config)
		} else {
			fmt.Println(renderConfigText(resp.Config))
		}
	default:
		if opts.output == "json" {
			printJSON(map[string]any{"success": true, "message": resp.Message})
		} else {
			fmt.Println(resp.Message)
		}
	}
}

func printJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to convert the result to JSON: "+err.Error())
		os.Exit(1)
	}
	fmt.Println(string(b))
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

const notRunningMsg = "mugcup is not currently running. Run 'mugcup-cli launch' first."

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		printHelp()
		return
	}

	command := strings.ToLower(args[0])
	if command == "help" || command == "-h" || command == "--help" || command == "/?" {
		printHelp()
		return
	}
	if command == "version" || command == "-v" || command == "--version" {
		fmt.Printf("mugcup-cli %s (%s)\n", Version, BuildVariant)
		return
	}

	opts, positional, err := parseOptions(args[1:])
	if err != nil {
		fail("argument error: %s", err.Error())
	}

	switch command {
	case "launch":
		printResult(opts, runLaunch(opts))

	case "exit", "quit":
		printResult(opts, runExit(opts))

	case "start":
		runStart(opts, positional)

	case "stop":
		runSimple(opts, "stop")

	case "set":
		if !opts.hasSetting() {
			fail("set requires at least one of: -d/--display-on, --auto-start, --auto-update-check, --auto-update-apply, --language, --tray-click-action.\nRun 'mugcup-cli help' for usage.")
		}
		runSimple(opts, "set")

	case "status":
		runSimple(opts, "status")

	case "config":
		runSimple(opts, "config")

	case "export":
		runExport(opts, positional)

	case "import":
		runImport(opts, positional)

	case "update":
		runUpdate(opts)

	case "settings":
		runSimple(opts, "settings")

	default:
		fail("unknown command: %s\nRun 'mugcup-cli help' for usage.", args[0])
	}
}

// runSimple sends command (plus opts' settings-only fields) to the running
// instance and prints the result — the shared shape behind
// stop/set/status/config/settings.
func runSimple(opts options, command string) {
	req := opts.asRequest()
	req.Command = command
	resp, ok := sendToRunningInstance(req)
	if !ok {
		fail(notRunningMsg)
	}
	printResult(opts, resp)
}

func runStart(opts options, positional []string) {
	if len(positional) == 0 {
		fail("start requires an argument: a duration (e.g. 30m, 1h30m), 'indefinite', 'preset <n>', or 'until <time>'.\nRun 'mugcup-cli help' for usage.")
	}
	args := positional
	if strings.ToLower(positional[0]) == "until" {
		if len(positional) < 2 {
			fail("start until requires a target time (e.g. 18:00, 2026-01-02 18:00).\nRun 'mugcup-cli help' for usage.")
		}
		target := strings.Join(positional[1:], " ")
		d, err := parseUntilTarget(target)
		if err != nil {
			fail("%s", err.Error())
		}
		if d <= 0 {
			fail("the target must be in the future: %s", target)
		}
		// mugcup.exe just wants a duration (settings.ParseDuration on args[1]);
		// it never needs to know what "18:00" means.
		args = []string{"until", d.String()}
	}
	req := opts.asRequest()
	req.Command = "start"
	req.Args = args
	resp, ok := sendToRunningInstance(req)
	if !ok {
		fail(notRunningMsg)
	}
	printResult(opts, resp)
}

func runExport(opts options, positional []string) {
	req := opts.asRequest()
	req.Command = "export"
	resp, ok := sendToRunningInstance(req)
	if !ok {
		fail(notRunningMsg)
	}
	if !resp.Success {
		printResult(opts, resp)
		return
	}
	data, err := json.MarshalIndent(resp.Config, "", "  ")
	if err != nil {
		fail("failed to convert config to JSON: %s", err.Error())
	}
	if len(positional) == 0 {
		fmt.Println(string(data))
		return
	}
	if err := os.WriteFile(positional[0], data, 0644); err != nil {
		fail("failed to save the config file: %s", err.Error())
	}
	printResult(opts, Response{Success: true, Message: "Exported config to: " + positional[0], Config: resp.Config})
}

func runImport(opts options, positional []string) {
	var raw []byte
	var err error
	if len(positional) == 0 {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(positional[0])
	}
	if err != nil {
		fail("failed to read the config file: %s", err.Error())
	}

	req := opts.asRequest()
	req.Command = "import"
	req.ConfigJSON = string(raw)
	resp, ok := sendToRunningInstance(req)
	if !ok {
		fail(notRunningMsg)
	}
	printResult(opts, resp)
}

// runUpdate checks for an update and, if one is found, confirms before
// installing it — a [y/N] prompt on stdin, or immediately with -y/--yes.
// This is the CLI's own confirmation, separate from (and never triggering)
// the native dialog the GUI's About view shows for the same check.
func runUpdate(opts options) {
	req := opts.asRequest()
	req.Command = "update"
	// mugcup.exe's "update" handler makes a real GitHub API call before
	// responding, so it needs updateTimeout's longer budget rather than the
	// default commandTimeout (fine for every other command, which mugcup.exe
	// answers purely from local state).
	resp, ok := sendToRunningInstanceTimeout(req, updateTimeout)
	if !ok {
		fail(notRunningMsg)
	}
	if !resp.Success || resp.Update == nil || !resp.Update.Available {
		printResult(opts, resp)
		return
	}

	if !opts.yes {
		fmt.Println(resp.Message)
		fmt.Print("Install it now? [y/N] ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Update cancelled.")
			return
		}
	}

	applyReq := opts.asRequest()
	applyReq.Command = "update-apply"
	applyResp, ok := sendToRunningInstance(applyReq)
	if !ok {
		fail(notRunningMsg)
	}
	printResult(opts, applyResp)
}
