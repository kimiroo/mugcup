package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type options struct {
	displayOn *bool
	output    string // "text" (default) or "json"
}

func printHelp() {
	fmt.Print(`mugcup-cli - CLI for controlling the running mugcup tray app

Usage:
  mugcup-cli <command> [args] [options]

Commands:
  launch                  Start mugcup (no-op success if already running)
  start [time]             Keep on for the given duration (e.g. 30m, 1h, 2h15m). No arg cycles presets
  start infinite           Keep on indefinitely
  start preset <n>         Start the n-th preset from the configured list (0-based)
  stop                     Turn off the timer and keep-on
  config                   Show the current config
  status                   Show the current status
  exit                     Exit the running mugcup
  import <path.json>       Read a config file and apply it (stdin if omitted)
  export <path.json>       Save the current config to a file (stdout if omitted)
  help                     Show this help

Options:
  -d, --display-on <true|false>   Explicitly set whether to also keep the display on (omit to keep the current setting)
  -o, --output <text|json>        Output format. start/stop/status/config/export/import
                                   print the result as multiple lines (text) or just that value (json)

Examples:
  mugcup-cli launch
  mugcup-cli start 1h30m
  mugcup-cli start preset 0
  mugcup-cli start infinite -d true
  mugcup-cli status -o json
  mugcup-cli export config.json
  mugcup-cli exit
`)
}

func parseOptions(args []string) (options, []string, error) {
	opts := options{output: "text"}
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-d", "--display-on":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("%s requires a true/false value", a)
			}
			i++
			v, err := parseBool(args[i])
			if err != nil {
				return opts, nil, fmt.Errorf("invalid value for %s: %s", a, args[i])
			}
			opts.displayOn = &v
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
		resp, ok := sendToRunningInstance(Request{Command: "start", Args: positional, DisplayOn: opts.displayOn})
		if !ok {
			fail(notRunningMsg)
		}
		printResult(opts, resp)

	case "stop":
		resp, ok := sendToRunningInstance(Request{Command: "stop", DisplayOn: opts.displayOn})
		if !ok {
			fail(notRunningMsg)
		}
		printResult(opts, resp)

	case "status":
		resp, ok := sendToRunningInstance(Request{Command: "status", DisplayOn: opts.displayOn})
		if !ok {
			fail(notRunningMsg)
		}
		printResult(opts, resp)

	case "config":
		resp, ok := sendToRunningInstance(Request{Command: "config", DisplayOn: opts.displayOn})
		if !ok {
			fail(notRunningMsg)
		}
		printResult(opts, resp)

	case "export":
		runExport(opts, positional)

	case "import":
		runImport(opts, positional)

	default:
		fail("unknown command: %s\nRun 'mugcup-cli help' for usage.", args[0])
	}
}

func runExport(opts options, positional []string) {
	resp, ok := sendToRunningInstance(Request{Command: "export", DisplayOn: opts.displayOn})
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

	resp, ok := sendToRunningInstance(Request{Command: "import", ConfigJSON: string(raw), DisplayOn: opts.displayOn})
	if !ok {
		fail(notRunningMsg)
	}
	printResult(opts, resp)
}
