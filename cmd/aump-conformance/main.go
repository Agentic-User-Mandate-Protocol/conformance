package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Agentic-User-Mandate-Protocol/conformance/internal/conformance"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] != "validate" {
		fmt.Fprintln(os.Stderr, "usage: aump-conformance validate [fixtures] [--format text|json|junit] [--output path] [--now RFC3339]")
		return 2
	}

	target, format, output, nowValue, err := parseValidateArgs(args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	var now *time.Time
	if nowValue != "" {
		parsed, err := time.Parse(time.RFC3339, nowValue)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		utc := parsed.UTC()
		now = &utc
	}

	report, err := conformance.RunSuite(target, now)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	rendered, err := render(report, format)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if output != "" {
		if err := conformance.WriteReport(output, rendered); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else {
		fmt.Print(rendered)
	}
	if report.Failed() > 0 {
		return 1
	}
	return 0
}

func parseValidateArgs(args []string) (target string, format string, output string, now string, err error) {
	target = "fixtures"
	format = "text"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--format" || arg == "--output" || arg == "--now" {
			if i+1 >= len(args) {
				err = fmt.Errorf("%s requires a value", arg)
				return
			}
			value := args[i+1]
			i++
			switch arg {
			case "--format":
				format = value
			case "--output":
				output = value
			case "--now":
				now = value
			}
			continue
		}
		if strings.HasPrefix(arg, "--format=") {
			format = strings.TrimPrefix(arg, "--format=")
			continue
		}
		if strings.HasPrefix(arg, "--output=") {
			output = strings.TrimPrefix(arg, "--output=")
			continue
		}
		if strings.HasPrefix(arg, "--now=") {
			now = strings.TrimPrefix(arg, "--now=")
			continue
		}
		if strings.HasPrefix(arg, "-") {
			err = fmt.Errorf("unknown option %s", arg)
			return
		}
		target = arg
	}
	return
}

func render(report conformance.SuiteReport, format string) (string, error) {
	switch format {
	case "text":
		return conformance.RenderText(report), nil
	case "json":
		return conformance.RenderJSON(report)
	case "junit":
		return conformance.RenderJUnit(report)
	default:
		return "", fmt.Errorf("unknown report format %q", format)
	}
}
