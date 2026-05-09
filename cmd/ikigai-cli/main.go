// Thin wiring layer per R-BUFE-M5E0. All meaningful behavior lives in
// importable packages; this entry point only parses flags and hooks
// stdin/stdout/signals.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ai4mgreenly/ikigai-cli/internal/agent"
	"github.com/ai4mgreenly/ikigai-cli/internal/model"
	"github.com/ai4mgreenly/ikigai-cli/internal/provider"
	anthropicprovider "github.com/ai4mgreenly/ikigai-cli/internal/provider/anthropic"
	"github.com/ai4mgreenly/ikigai-cli/internal/schema"
	"github.com/ai4mgreenly/ikigai-cli/internal/startup"
	"github.com/ai4mgreenly/ikigai-cli/internal/tools"
	"github.com/ai4mgreenly/ikigai-cli/internal/trace"
	"github.com/ai4mgreenly/ikigai-cli/internal/wire"
)

type parsedFlags struct {
	model  string
	effort string
	// R-1OPL-X3LD: path to a JSON Schema file; structured_output
	// emitted in the iteration's result event must validate
	// against it. Empty means no schema-validation gate.
	jsonSchema string
	// R-1O1T-0MEX: --dangerously-skip-permissions is accepted as a
	// no-op. v1 has no permission system, so the flag exists only
	// for ralph-loops/Claude-Code parity.
	dangerouslySkipPermissions bool
	// R-6TC0-ZSKM: -p (short form) and --print (long form) are accepted
	// as no-ops. Print mode is the only invocation mode; the flag has
	// no behavioral effect beyond being accepted without error.
	p string
	// R-489X-89DC: remaining ralph-loops flags accepted as no-ops.
	verbose            bool
	inputFormat        string
	outputFormat       string
	replayUserMessages bool
	toolsFlag          string
	// R-92NN-7DNI: debug trace to stdout when true.
	raw bool
}

// R-YARD-835I: ikigai-cli must accept the exact same flag set
// ralph-loops uses for `claude` — no more, no less. newFlagSet binds
// every accepted flag in one place so a test can introspect it and
// pin parity with the documented ralph-loops invocation.
func newFlagSet(errOut io.Writer) (*flag.FlagSet, *parsedFlags) {
	fs := flag.NewFlagSet("ikigai-cli", flag.ContinueOnError)
	fs.SetOutput(errOut)
	out := &parsedFlags{}

	// R-XV7A-7AEF: --help renders every flag in double-dash form to
	// match `claude --help`. Go's default flag.PrintDefaults prints
	// single-dash, so we override Usage to emit `--name` lines.
	fs.Usage = func() {
		w := fs.Output()
		fmt.Fprintln(w, "Usage of ikigai-cli:")
		fs.VisitAll(func(f *flag.Flag) {
			if f.Usage == "\x00" {
				return // short-alias, rendered as part of the long form above
			}
			if f.DefValue == "" {
				fmt.Fprintf(w, "  --%s\n    \t%s\n", f.Name, f.Usage)
			} else {
				fmt.Fprintf(w, "  --%s (default %q)\n    \t%s\n", f.Name, f.DefValue, f.Usage)
			}
		})
	}

	// R-XBYO-1ZI1: --model takes the bare provider model ID (or a
	// documented alias). Provider is inferred from the prefix here;
	// registry/effort validation is layered on later.
	fs.StringVar(&out.model, "model", "", "model ID (e.g. claude-haiku-4-5 or alias haiku)")
	// R-31CY-UXSX: --effort is accepted by the flag parser but the
	// model registry decides whether a given model accepts a value;
	// Haiku 4.5 in MVP rejects any non-empty --effort.
	fs.StringVar(&out.effort, "effort", "", "reasoning/thinking effort (model-specific; Haiku 4.5 accepts none)")
	// R-1O1T-0MEX: accepted as a no-op for parity with the real
	// claude binary; v1 has no permission system.
	fs.BoolVar(&out.dangerouslySkipPermissions, "dangerously-skip-permissions", false, "no-op (v1 has no permission system)")
	// R-1OPL-X3LD, R-JNEB-EVLU: the value is the JSON Schema document
	// itself as an inline JSON string — not a filesystem path.
	fs.StringVar(&out.jsonSchema, "json-schema", "", "inline JSON Schema document for structured_output validation")
	// R-6TC0-ZSKM: --print (long form) and -p (short alias) are both
	// accepted as no-ops for ralph-loops drop-in parity. "p" is hidden
	// from --help so it does not appear as the fake long form --p.
	fs.StringVar(&out.p, "print", "", "no-op (accepted for ralph-loops drop-in parity; short alias: -p)")
	fs.StringVar(&out.p, "p", "", "\x00") // short alias for --print; hidden from --help
	// R-Z4YN-KG36: remaining ralph-loops flags accepted; each is a no-op.
	fs.BoolVar(&out.verbose, "verbose", false, "no-op (ikigai-cli has no verbosity dial)")
	fs.StringVar(&out.inputFormat, "input-format", "", "no-op (stream-json is the only supported input format)")
	fs.StringVar(&out.outputFormat, "output-format", "", "no-op (stream-json is the only supported output format)")
	fs.BoolVar(&out.replayUserMessages, "replay-user-messages", false, "no-op (ikigai-cli already emits user events for every user-message turn)")
	// R-YFCR-J9IL: --tools narrows the tool surface offered to the model.
	fs.StringVar(&out.toolsFlag, "tools", "", "comma-separated tool names to offer (empty = all tools)")
	// R-92NN-7DNI: --raw writes a debug trace of every boundary crossing
	// (stdin, stdout, HTTP, tools) to stdout. This is an ikigai-cli
	// extension; it is not in the ralph-loops/claude drop-in flag set.
	fs.BoolVar(&out.raw, "raw", false, "write stdin/stdout/HTTP/tool debug trace to stdout")

	return fs, out
}

func parseFlags(args []string, errOut io.Writer) (parsedFlags, error) {
	fs, out := newFlagSet(errOut)
	if err := fs.Parse(args); err != nil {
		return parsedFlags{}, err
	}
	return *out, nil
}

// run executes the full startup → iteration pipeline, writing to the supplied
// stdout/stderr. Returns the process exit code (0 on success, 1/2 on error).
// Extracted from main for testability — R-2247-BPXI requires that startup
// errors write nothing to stdout, which can only be verified by calling this
// function directly with a controlled stdout writer.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags, err := parseFlags(args, stderr)
	if err != nil {
		return 2
	}

	resolved, err := model.Resolve(flags.model)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}

	// R-YRPM-NUDF / R-ZCFX-5XZ8: registry-driven validation.
	if err := model.Validate(resolved); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}

	// R-31CY-UXSX: per-model effort validation.
	if err := model.ValidateEffort(resolved, flags.effort); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}

	// R-YL2Y-7HXQ: missing provider credential is a fatal startup error.
	switch resolved.Provider {
	case model.ProviderAnthropic:
		if err := startup.RequireAnthropicKey(); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
	}

	_ = flags.dangerouslySkipPermissions

	// R-YFCR-J9IL: resolve --tools to a Descriptor slice; unknown name is fatal.
	selectedTools, err := tools.Select(flags.toolsFlag)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}

	apiKey := os.Getenv(startup.AnthropicKeyEnv)
	client, err := anthropicprovider.New(apiKey, resolved.BareID)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}

	// R-92NN-7DNI: attach tracer when --raw is set; trace goes to stdout.
	var tr *trace.Tracer
	if flags.raw {
		tr = trace.New(stdout, apiKey)
		client.SetTracer(tr)
	}

	// R-7GAW-CQ00: wire stdin -> agent -> stdout.
	if err := runIteration(context.Background(), stdin, stdout, client, resolved, flags.effort, flags.jsonSchema, tr, selectedTools); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// tracingWriter wraps an io.Writer and echoes each Write to the tracer as a
// stdout event. Used by runIteration to satisfy R-92NN-7DNI's [<stdout] boundary.
type tracingWriter struct {
	w  io.Writer
	tr *trace.Tracer
}

func (tw *tracingWriter) Write(p []byte) (int, error) {
	n, err := tw.w.Write(p)
	if err == nil {
		tw.tr.LogStdoutEvent(string(p))
	}
	return n, err
}

// runIteration reads user events from stdin, drives agent.Run against client,
// and writes stream-json events to stdout. Extracted for testability.
// R-7GAW-CQ00. tracer may be nil.
func runIteration(ctx context.Context, stdin io.Reader, stdout io.Writer, client provider.Client, resolved model.Resolved, effort string, jsonSchema string, tracer *trace.Tracer, toolDescs []tools.Descriptor) error {
	var sch *schema.Schema
	if jsonSchema != "" {
		// R-JNEB-EVLU: value is an inline JSON string, not a file path.
		var parseErr error
		sch, parseErr = schema.Parse([]byte(jsonSchema))
		if parseErr != nil {
			return fmt.Errorf("--json-schema: %w", parseErr)
		}
	}

	// R-A351-VO9A: dispatch as soon as the first user event arrives; do
	// not wait for stdin EOF. ralph-loops keeps stdin open until it reads
	// the result event, so blocking on EOF deadlocks the iteration.
	sr := wire.NewStdinReader(stdin)
	var msgs []provider.Message
	for {
		ev, err := sr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("stdin: %w", err)
		}
		// R-92NN-7DNI: log each stdin event as soon as it is received.
		tracer.LogStdinEvent(sr.LastRaw())
		var blocks []provider.Block
		for _, b := range ev.Message.Content {
			if tb, ok := b.(wire.TextBlock); ok {
				blocks = append(blocks, provider.TextBlock{Text: tb.Text})
			}
		}
		msgs = append(msgs, provider.Message{Role: provider.RoleUser, Blocks: blocks})
		// R-A351-VO9A: one user event is sufficient to start the iteration.
		break
	}

	provTools := make([]provider.Tool, len(toolDescs))
	for i, d := range toolDescs {
		provTools[i] = provider.Tool{Name: d.Name, InputSchema: d.InputSchema}
	}

	req := provider.Request{
		Model:        resolved.BareID,
		Effort:       effort,
		SystemPrompt: agent.FramingPrompt,
		Messages:     msgs,
		Tools:        provTools,
	}

	// R-92NN-7DNI: wrap stdout so each emitted stream-json event is
	// also logged as a [<stdout] trace entry.
	var sessOut io.Writer = stdout
	if tracer != nil {
		sessOut = &tracingWriter{w: stdout, tr: tracer}
	}
	sess := wire.NewSession(sessOut)
	return agent.Run(ctx, client, sess, req, sch, tracer)
}
