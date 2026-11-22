// Package cli implements the sumble command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/MaxHorstmann/sumble-cli/internal/api"
)

const usageText = `sumble - lightweight CLI for the Sumble API

Usage:
  sumble people find   --name NAME (--company NAME | --company-domain DOMAIN) [flags]
  sumble people find   --linkedin URL | --person-id ID [flags]
  sumble people search QUERY [--org-id ID]... [--location CODE]... [flags]
  sumble orgs find     --url DOMAIN | --slug SLUG | --id ID | --name NAME [flags]

Common flags:
  --attrs LIST     comma-separated attributes to return ("all" for everything)
  --json           print the raw JSON response instead of a table
  --timeout DUR    overall time limit, including polling (default 2m)

Run "sumble <command> <subcommand> -h" for details.

Authentication: set SUMBLE_API_KEY (create a key at https://sumble.com/account/api-keys).
`

const missingKeyMsg = "error: SUMBLE_API_KEY is not set (create one at https://sumble.com/account/api-keys)"

type app struct {
	stdout, stderr io.Writer
	client         *api.Client // nil when SUMBLE_API_KEY is unset (help only)
}

// Main runs the CLI and returns the process exit code:
// 0 success, 1 error or no results, 2 usage error.
func Main(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	a := &app{stdout: stdout, stderr: stderr}
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	if isHelp(args[0]) || args[0] == "help" {
		fmt.Fprint(stdout, usageText)
		return 0
	}

	// Fail fast without an API key; only help requests are allowed without one.
	if key := strings.TrimSpace(getenv("SUMBLE_API_KEY")); key != "" {
		a.client = api.New(key)
		if base := getenv("SUMBLE_API_BASE_URL"); base != "" {
			a.client.BaseURL = strings.TrimRight(base, "/")
		}
	} else if !anyHelp(args) {
		fmt.Fprintln(stderr, missingKeyMsg)
		return 1
	}

	switch args[0] {
	case "people":
		return a.dispatch(ctx, "people", args[1:], map[string]func(context.Context, []string) int{
			"find":   a.peopleFind,
			"search": a.peopleSearch,
		})
	case "orgs", "organizations":
		return a.dispatch(ctx, "orgs", args[1:], map[string]func(context.Context, []string) int{
			"find": a.orgsFind,
		})
	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n\n%s", args[0], usageText)
		return 2
	}
}

func (a *app) dispatch(ctx context.Context, group string, args []string, subs map[string]func(context.Context, []string) int) int {
	if len(args) > 0 {
		if fn, ok := subs[args[0]]; ok {
			return fn(ctx, args[1:])
		}
		if isHelp(args[0]) {
			fmt.Fprint(a.stdout, usageText)
			return 0
		}
		fmt.Fprintf(a.stderr, "error: unknown subcommand %q for %q\n\n", args[0], group)
	}
	fmt.Fprint(a.stderr, usageText)
	return 2
}

func isHelp(s string) bool { return s == "-h" || s == "-help" || s == "--help" }

func anyHelp(args []string) bool {
	for _, s := range args {
		if isHelp(s) {
			return true
		}
	}
	return false
}

// outputOpts are the flags shared by all commands.
type outputOpts struct {
	attrs   string
	json    bool
	timeout time.Duration
}

func addCommon(fs *flag.FlagSet, o *outputOpts, defaultAttrs string) {
	fs.StringVar(&o.attrs, "attrs", defaultAttrs, `comma-separated attributes to return, or "all"`)
	fs.BoolVar(&o.json, "json", false, "print the raw JSON response instead of a table")
	fs.DurationVar(&o.timeout, "timeout", 2*time.Minute, "overall time limit, including polling")
}

func (o outputOpts) attributes() ([]string, error) {
	var list []string
	for _, s := range strings.Split(o.attrs, ",") {
		if s = strings.TrimSpace(s); s != "" {
			list = append(list, s)
		}
	}
	if len(list) == 0 {
		return nil, errors.New("--attrs must not be empty")
	}
	for _, s := range list {
		if s == "all" && len(list) > 1 {
			return nil, errors.New(`--attrs "all" cannot be combined with other attributes`)
		}
	}
	return list, nil
}

func (a *app) flagSet(name, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n  %s\n\nFlags:\n", synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// parse parses flags that may be interspersed with positional arguments.
// It returns the positionals, or ok=false with the exit code to use.
func (a *app) parse(fs *flag.FlagSet, args []string) (pos []string, code int, ok bool) {
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, 0, false
			}
			return nil, 2, false
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, 0, true
		}
		// Parse stops at the first non-flag, or after "--".
		if consumed := len(args) - len(rest); consumed > 0 && args[consumed-1] == "--" {
			return append(pos, rest...), 0, true
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

func (a *app) usageErr(fs *flag.FlagSet, format string, args ...any) int {
	fmt.Fprintf(a.stderr, "error: "+format+"\nRun '%s -h' for usage.\n", append(args, fs.Name())...)
	return 2
}

// run executes an API call and renders the result.
func (a *app) run(ctx context.Context, o outputOpts, attrs []string, listKey string, call func(context.Context) (*api.Response, error)) int {
	if a.client == nil {
		fmt.Fprintln(a.stderr, missingKeyMsg)
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	resp, err := call(ctx)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			fmt.Fprintf(a.stderr, "error: %v (try a larger --timeout)\n", err)
		case errors.Is(err, context.Canceled):
			fmt.Fprintln(a.stderr, "interrupted")
		default:
			fmt.Fprintf(a.stderr, "error: %v\n", err)
		}
		return 1
	}
	a.reportCredits(resp)

	rows, err := extractRows(resp.Raw, listKey)
	if err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return 1
	}
	matched := matchedRows(rows)
	if o.json {
		writeJSON(a.stdout, resp.Raw)
	} else if len(matched) > 0 {
		writeTable(a.stdout, matched, attrs)
	}
	if len(matched) == 0 {
		fmt.Fprintln(a.stderr, "no results")
		return 1
	}
	return 0
}

func (a *app) reportCredits(r *api.Response) {
	if r.CreditsUsed == nil {
		return
	}
	remaining := r.CreditsRemaining
	if remaining == nil && r.InitialCreditsRemaining != nil {
		v := *r.InitialCreditsRemaining - *r.CreditsUsed
		remaining = &v
	}
	msg := "credits used: " + fmtNum(*r.CreditsUsed)
	if remaining != nil {
		msg += ", remaining: " + fmtNum(*remaining)
	}
	fmt.Fprintln(a.stderr, msg)
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// int64List is a repeatable, comma-separated integer flag.
type int64List []int64

func (l *int64List) String() string { return fmt.Sprint([]int64(*l)) }

func (l *int64List) Set(s string) error {
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer %q", p)
		}
		*l = append(*l, n)
	}
	return nil
}

// stringList is a repeatable, comma-separated string flag.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(s string) error {
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*l = append(*l, p)
		}
	}
	return nil
}
