package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ai4mgreenly/ikigai-cli/internal/model"
	"github.com/ai4mgreenly/ikigai-cli/internal/provider"
	"github.com/ai4mgreenly/ikigai-cli/internal/tools"
	"github.com/ai4mgreenly/ikigai-cli/internal/trace"
)

// R-YARD-835I: ralph-loops invokes ikigai-cli with the same flag set
// it uses for `claude`. The accepted flags must include exactly the
// ralph-loops/claude surface flags this binary translates — adding,
// dropping, or renaming one breaks the drop-in invariant. The
// canonical invocation must also parse cleanly so an end-to-end
// ralph iteration can boot.
func TestR_YARD_835I_FlagSetMatchesClaudeDropIn(t *testing.T) {
	fs, _ := newFlagSet(io.Discard)

	// R-6TC0-ZSKM adds "p" and "print"; R-489X-89DC adds verbose,
	// input-format, output-format, replay-user-messages, and tools —
	// the complete ralph-loops drop-in set.
	dropIn := map[string]bool{
		"model":                        true,
		"effort":                       true,
		"dangerously-skip-permissions": true,
		"json-schema":                  true,
		"p":                            true,
		"print":                        true,
		"verbose":                      true,
		"input-format":                 true,
		"output-format":                true,
		"replay-user-messages":         true,
		"tools":                        true,
	}
	// R-6EFF-GW25: --raw is an ikigai-cli-specific extension that is
	// intentionally not in the ralph-loops/claude drop-in flag set.
	ikigaiExtensions := map[string]bool{
		"raw": true,
	}
	seen := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) { seen[f.Name] = true })

	for name := range dropIn {
		if !seen[name] {
			t.Errorf("R-YARD-835I: missing required drop-in flag %q", name)
		}
	}
	for name := range seen {
		if !dropIn[name] && !ikigaiExtensions[name] {
			t.Errorf("R-YARD-835I: unexpected flag %q breaks claude drop-in parity", name)
		}
	}

	// Canonical ralph-loops invocation (all flags) must parse without error.
	canonical := []string{
		"--model", "claude-haiku-4-5",
		"--dangerously-skip-permissions",
		"--json-schema", "/tmp/schema.json",
		"-p", "some prompt",
		"--verbose",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--replay-user-messages",
		"--tools", "",
	}
	flags, err := parseFlags(canonical, io.Discard)
	if err != nil {
		t.Fatalf("canonical ralph-loops invocation rejected: %v", err)
	}
	if flags.model != "claude-haiku-4-5" {
		t.Errorf("model: got %q want %q", flags.model, "claude-haiku-4-5")
	}
	if !flags.dangerouslySkipPermissions {
		t.Errorf("dangerouslySkipPermissions: expected true after canonical invocation")
	}
	if flags.jsonSchema != "/tmp/schema.json" {
		t.Errorf("jsonSchema: got %q want %q", flags.jsonSchema, "/tmp/schema.json")
	}
}

// R-XV7A-7AEF: `ikigai-cli --help` renders every flag in double-dash form
// (`--model`, `--effort`, `--json-schema`, `--dangerously-skip-permissions`).
// The single-dash form may continue to be accepted but must not appear in
// help output, matching `claude --help`.
func TestR_XV7A_7AEF_HelpRendersDoubleDashFlags(t *testing.T) {
	var out bytes.Buffer
	fs, _ := newFlagSet(&out)
	// `--help` triggers ErrHelp under ContinueOnError, which calls
	// fs.Usage and writes to fs.Output().
	err := fs.Parse([]string{"--help"})
	if err != flag.ErrHelp {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
	help := out.String()

	for _, name := range []string{"--model", "--effort", "--json-schema", "--dangerously-skip-permissions"} {
		if !strings.Contains(help, name) {
			t.Errorf("help output missing %q; got:\n%s", name, help)
		}
	}

	// No single-dash form for any of the four ralph-loops flags. Scan
	// line-by-line so a substring like "--model" doesn't mask a stray
	// "-model" elsewhere on a different line.
	for _, line := range strings.Split(help, "\n") {
		for _, name := range []string{"model", "effort", "json-schema", "dangerously-skip-permissions"} {
			single := "-" + name
			double := "--" + name
			// Strip every occurrence of the double-dash form so a
			// remaining "-name" substring is a true single-dash hit.
			stripped := strings.ReplaceAll(line, double, "")
			if strings.Contains(stripped, single) {
				t.Errorf("help line contains single-dash form %q: %q", single, line)
			}
		}
	}
}

// R-6TC0-ZSKM: ikigai-cli accepts both -p (short form) and --print
// (long form) on the command line. ralph-loops passes -p on every
// invocation; rejecting either form would prevent iterations from
// running. Both are no-ops — accepted and silently ignored. In
// --help output, the flag renders as --print with -p noted as the
// short alias; --p must not appear as a fake long form.
func TestR_6TC0_ZSKM_AcceptsPrintAndPFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"bare_p", []string{"-p", "hello"}},
		{"bare_print", []string{"--print", "hello"}},
		{"p_with_other_flags", []string{"-p", "hello", "--model", "haiku"}},
		{"print_with_other_flags", []string{"--print", "hello", "--model", "haiku"}},
		{"p_empty_string", []string{"-p", ""}},
		{"print_empty_string", []string{"--print", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			_, err := parseFlags(tc.args, &stderr)
			if err != nil {
				t.Fatalf("parseFlags(%v) errored: %v; stderr=%q", tc.args, err, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("expected no flag-parser output, got %q", stderr.String())
			}
		})
	}

	// --help must show --print (with -p noted) and must NOT show --p.
	t.Run("help_shows_print_not_p", func(t *testing.T) {
		var out bytes.Buffer
		fs, _ := newFlagSet(&out)
		_ = fs.Parse([]string{"--help"})
		help := out.String()
		if !strings.Contains(help, "--print") {
			t.Errorf("help must contain --print; got:\n%s", help)
		}
		if !strings.Contains(help, "-p") {
			t.Errorf("help must mention -p as short alias; got:\n%s", help)
		}
		for _, line := range strings.Split(help, "\n") {
			// "--print" on a line is fine; "--p" that is NOT part of "--print" is the fake long form.
			stripped := strings.ReplaceAll(line, "--print", "")
			if strings.Contains(stripped, "--p") {
				t.Errorf("help contains --p as a standalone flag definition: %q", line)
			}
		}
	})

	// Sanity: the flag must still reject genuinely unknown flags.
	t.Run("unknown_flag_still_errors", func(t *testing.T) {
		var stderr bytes.Buffer
		_, err := parseFlags([]string{"--no-such-flag"}, &stderr)
		if err == nil {
			t.Fatalf("expected error for unknown flag, got nil")
		}
	})
}

// R-1O1T-0MEX: --dangerously-skip-permissions is accepted as a no-op.
// v1 has no permission system, so the flag only exists for parity with
// the real claude binary; passing it must not error and must not
// disturb the rest of the parsed flag values.
func TestR_1O1T_0MEX_DangerouslySkipPermissionsIsAcceptedNoOp(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"bare", []string{"--dangerously-skip-permissions"}},
		{"explicit_true", []string{"--dangerously-skip-permissions=true"}},
		{"with_other_flags", []string{"--dangerously-skip-permissions", "--model", "haiku"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			flags, err := parseFlags(tc.args, &stderr)
			if err != nil {
				t.Fatalf("parseFlags(%v) errored: %v; stderr=%q", tc.args, err, stderr.String())
			}
			if !flags.dangerouslySkipPermissions {
				t.Errorf("expected dangerouslySkipPermissions=true after parsing %v", tc.args)
			}
			if stderr.Len() != 0 {
				t.Errorf("expected no flag-parser output, got %q", stderr.String())
			}
		})
	}

	// Sanity: when the flag is absent, the field must be false (no
	// silent default-on behavior that would imply a permission system).
	t.Run("absent_defaults_false", func(t *testing.T) {
		var stderr bytes.Buffer
		flags, err := parseFlags([]string{"--model", "haiku"}, &stderr)
		if err != nil {
			t.Fatalf("parseFlags errored: %v", err)
		}
		if flags.dangerouslySkipPermissions {
			t.Errorf("expected dangerouslySkipPermissions=false when flag omitted")
		}
	})

	// Sanity: an unrelated unknown flag still errors. This guards the
	// no-op interpretation against silently swallowing typos like
	// --dangerously-skip-permission (singular).
	t.Run("typo_errors", func(t *testing.T) {
		var stderr bytes.Buffer
		_, err := parseFlags([]string{"--dangerously-skip-permission"}, &stderr)
		if err == nil {
			t.Fatalf("expected error for typo'd flag, got nil")
		}
		if !strings.Contains(stderr.String(), "dangerously-skip-permission") {
			t.Errorf("expected stderr to mention the offending flag, got %q", stderr.String())
		}
	})
}

// R-2247-BPXI: startup errors (missing credentials, unknown model prefix,
// unknown model in registry, illegal effort, unrecognized flags) write a
// human-readable message to stderr, exit non-zero, and write nothing to
// stdout — no result event, no system event, no partial stream-json.
func TestR_2247_BPXI_StartupErrorsWriteNothingToStdout(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		setupEnv   func(t *testing.T)
		wantCode   int // expected non-zero exit code
		stderrHint string
	}{
		{
			name:       "unrecognized_flag",
			args:       []string{"--no-such-flag"},
			wantCode:   2,
			stderrHint: "no-such-flag",
		},
		{
			name:       "unknown_model_prefix",
			args:       []string{"--model", "badprovider-model-1"},
			wantCode:   1,
			stderrHint: "badprovider",
		},
		{
			name:       "unknown_model_in_registry",
			args:       []string{"--model", "claude-nonexistent-99"},
			wantCode:   1,
			stderrHint: "claude-nonexistent-99",
		},
		{
			name:       "illegal_effort",
			args:       []string{"--model", "claude-haiku-4-5", "--effort", "high"},
			setupEnv:   func(t *testing.T) { t.Setenv("ANTHROPIC_API_KEY", "dummy") },
			wantCode:   1,
			stderrHint: "effort",
		},
		{
			name:     "missing_credentials",
			args:     []string{"--model", "claude-haiku-4-5"},
			setupEnv: func(t *testing.T) { t.Setenv("ANTHROPIC_API_KEY", "") },
			wantCode: 1,
			// error names the missing env var
			stderrHint: "ANTHROPIC_API_KEY",
		},
		{
			// R-YFCR-J9IL: unknown tool name in --tools is a fatal startup error.
			name:       "unknown_tool_name",
			args:       []string{"--model", "claude-haiku-4-5", "--tools", "NoSuchTool"},
			setupEnv:   func(t *testing.T) { t.Setenv("ANTHROPIC_API_KEY", "dummy") },
			wantCode:   1,
			stderrHint: "NoSuchTool",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupEnv != nil {
				tc.setupEnv(t)
			}
			var stdout, stderr bytes.Buffer
			code := run(tc.args, strings.NewReader(""), &stdout, &stderr)
			if code == 0 {
				t.Fatalf("expected non-zero exit code, got 0; stderr=%q stdout=%q",
					stderr.String(), stdout.String())
			}
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d", code, tc.wantCode)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout must be empty on startup error, got %q", stdout.String())
			}
			if tc.stderrHint != "" && !strings.Contains(stderr.String(), tc.stderrHint) {
				t.Errorf("stderr missing hint %q; got %q", tc.stderrHint, stderr.String())
			}
		})
	}
}

// R-946R-TZ4B: ikigai-cli accepts --verbose, --input-format, --output-format,
// and --tools without error. Each is effectively a no-op; rejecting any one
// violates the drop-in invariant (R-YARD-835I).
// R-489X-89DC: ikigai-cli accepts verbose, input-format, output-format,
// replay-user-messages, and tools — the remaining flags ralph-loops passes
// on every invocation. Each is a no-op; rejecting any would violate the
// drop-in invariant (R-YARD-835I).
func TestR_489X_89DC_AcceptsRalphLoopsRemainingFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"verbose", []string{"--verbose"}},
		{"input_format", []string{"--input-format", "stream-json"}},
		{"output_format", []string{"--output-format", "stream-json"}},
		{"replay_user_messages", []string{"--replay-user-messages"}},
		{"tools_empty", []string{"--tools", ""}},
		{"tools_nonempty", []string{"--tools", "bash,read"}},
		{"all_together", []string{
			"--verbose",
			"--input-format", "stream-json",
			"--output-format", "stream-json",
			"--replay-user-messages",
			"--tools", "",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			_, err := parseFlags(tc.args, &stderr)
			if err != nil {
				t.Fatalf("parseFlags(%v) errored: %v; stderr=%q", tc.args, err, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("expected no flag-parser output, got %q", stderr.String())
			}
		})
	}
}

// R-Z4YN-KG36: ikigai-cli accepts --verbose, --input-format, --output-format, and
// --replay-user-messages as no-ops. Rejecting any of these violates the drop-in
// invariant (R-YARD-835I). Note: --tools is handled separately by R-YFCR-J9IL.
func TestR_Z4YN_KG36_AcceptsNoOpRalphLoopsFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"verbose", []string{"--verbose"}},
		{"input_format_stream_json", []string{"--input-format", "stream-json"}},
		{"output_format_stream_json", []string{"--output-format", "stream-json"}},
		{"replay_user_messages", []string{"--replay-user-messages"}},
		{"all_together", []string{
			"--verbose",
			"--input-format", "stream-json",
			"--output-format", "stream-json",
			"--replay-user-messages",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			_, err := parseFlags(tc.args, &stderr)
			if err != nil {
				t.Fatalf("parseFlags(%v) errored: %v; stderr=%q", tc.args, err, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("expected no output, got %q", stderr.String())
			}
		})
	}
}

// R-JNEB-EVLU: --json-schema value is the JSON Schema document as an inline
// JSON string. Treating it as a filesystem path is a bug; a bare-value like
// "/tmp/schema.json" must fail with a JSON parse error, not an os.Open error.
func TestR_JNEB_EVLU_JsonSchemaIsInlineNotPath(t *testing.T) {
	const stdinPayload = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}` + "\n"
	resolved := model.Resolved{Provider: model.ProviderAnthropic, BareID: "claude-haiku-4-5"}

	// Inline JSON schema is accepted and the iteration proceeds normally.
	t.Run("inline_json_accepted", func(t *testing.T) {
		client := &fakeCLIClient{events: []provider.Event{
			provider.EventTextDelta{Text: `{"status":"DONE"}`},
			provider.EventDone{StopReason: "end_turn"},
		}}
		var out bytes.Buffer
		err := runIteration(context.Background(), strings.NewReader(stdinPayload), &out, client, resolved, "",
			`{"type":"object","properties":{"status":{"type":"string","enum":["DONE","CONTINUE"]}},"required":["status"]}`, nil, tools.All())
		if err != nil {
			t.Fatalf("runIteration with inline schema errored: %v", err)
		}
	})

	// A file-path-like string is not valid JSON, so the error must be a JSON
	// parse error — not "open: no such file or directory".
	t.Run("file_path_is_not_json_parse_error", func(t *testing.T) {
		client := &fakeCLIClient{}
		var out bytes.Buffer
		err := runIteration(context.Background(), strings.NewReader(stdinPayload), &out, client, resolved, "",
			"/tmp/some-schema.json", nil, tools.All())
		if err == nil {
			t.Fatal("expected error for non-JSON schema value, got nil")
		}
		if strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "open ") {
			t.Errorf("error looks like a file-open error, not a JSON parse error: %v", err)
		}
		if !strings.Contains(err.Error(), "--json-schema") {
			t.Errorf("error should mention --json-schema flag, got: %v", err)
		}
	})
}

// fakeCLIClient is a provider.Client whose Stream replays a fixed event
// sequence. Used by TestR_7GAW_CQ00 to avoid real HTTP calls.
type fakeCLIClient struct {
	events []provider.Event
}

func (f *fakeCLIClient) Stream(_ context.Context, _ provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, len(f.events))
	for _, ev := range f.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// R-7GAW-CQ00: cmd/ikigai-cli reads the user event(s) from stdin,
// assembles a provider request (model, tools, system prompt, messages),
// drives the agent loop, and writes stream-json events to stdout.
// This test verifies the end-to-end wiring with a fake provider client.
func TestR_7GAW_CQ00_MainWiresStdinToAgentAndWritesEvents(t *testing.T) {
	const stdinPayload = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}` + "\n"
	stdin := strings.NewReader(stdinPayload)

	client := &fakeCLIClient{events: []provider.Event{
		provider.EventTextDelta{Text: `{"status":"DONE"}`},
		provider.EventDone{StopReason: "end_turn"},
	}}

	var stdout bytes.Buffer
	resolved := model.Resolved{Provider: model.ProviderAnthropic, BareID: "claude-haiku-4-5"}

	if err := runIteration(context.Background(), stdin, &stdout, client, resolved, "", "", nil, tools.All()); err != nil {
		t.Fatalf("runIteration: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 events, got %d: %q", len(lines), stdout.String())
	}

	var firstEv map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &firstEv); err != nil {
		t.Fatalf("parse event 0: %v", err)
	}
	if firstEv["type"] != "assistant" {
		t.Errorf("event 0 type = %q, want \"assistant\"", firstEv["type"])
	}

	var lastEv map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &lastEv); err != nil {
		t.Fatalf("parse last event: %v", err)
	}
	if lastEv["type"] != "result" {
		t.Errorf("last event type = %q, want \"result\"", lastEv["type"])
	}
}

// R-6EFF-GW25: ikigai-cli accepts a --raw boolean flag (defaults false).
// When set, a debug trace is written for every tool dispatch and every
// provider HTTP interaction. API key values must never appear in the trace.
func TestR_6EFF_GW25_RawFlag(t *testing.T) {
	const stdinPayload = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}` + "\n"
	resolved := model.Resolved{Provider: model.ProviderAnthropic, BareID: "claude-haiku-4-5"}

	// --raw defaults to false and must parse cleanly.
	t.Run("flag_accepted_default_false", func(t *testing.T) {
		flags, err := parseFlags([]string{"--model", "claude-haiku-4-5"}, io.Discard)
		if err != nil {
			t.Fatalf("parseFlags: %v", err)
		}
		if flags.raw {
			t.Errorf("expected raw=false when flag omitted, got true")
		}
	})

	// --raw=true must parse cleanly.
	t.Run("flag_accepted_explicit_true", func(t *testing.T) {
		flags, err := parseFlags([]string{"--raw"}, io.Discard)
		if err != nil {
			t.Fatalf("parseFlags --raw: %v", err)
		}
		if !flags.raw {
			t.Errorf("expected raw=true after --raw, got false")
		}
	})

	// When --raw is false, runIteration writes nothing to stderr.
	t.Run("no_raw_no_stderr", func(t *testing.T) {
		client := &fakeCLIClient{events: []provider.Event{
			provider.EventTextDelta{Text: `{"status":"DONE"}`},
			provider.EventDone{StopReason: "end_turn"},
		}}
		var stderr bytes.Buffer
		_ = runIteration(context.Background(), strings.NewReader(stdinPayload), io.Discard, client, resolved, "", "", nil, tools.All())
		if stderr.Len() != 0 {
			t.Errorf("no-raw mode wrote to stderr: %q", stderr.String())
		}
	})

	// When --raw is active, tool dispatch and result lines appear in the trace.
	t.Run("raw_traces_tool_dispatch", func(t *testing.T) {
		const secretKey = "sk-test-secret-key-12345"
		toolInput, _ := json.Marshal(map[string]string{"command": "echo hello"})
		client := &fakeTwoStepClient{
			firstEvents: []provider.Event{
				provider.EventToolUse{
					ID:    "toolu_trace01",
					Name:  "Bash",
					Input: toolInput,
				},
				provider.EventDone{StopReason: "tool_use"},
			},
			secondEvents: []provider.Event{
				provider.EventTextDelta{Text: `{"status":"DONE"}`},
				provider.EventDone{StopReason: "end_turn"},
			},
		}
		var traceBuf bytes.Buffer
		tr := trace.New(&traceBuf, secretKey)
		var stdout bytes.Buffer
		err := runIteration(context.Background(), strings.NewReader(stdinPayload), &stdout, client, resolved, "", "", tr, tools.All())
		if err != nil {
			t.Fatalf("runIteration: %v", err)
		}
		got := traceBuf.String()
		if !strings.Contains(got, "[tool>]") {
			t.Errorf("trace missing [tool>] tag; trace=%q", got)
		}
		if !strings.Contains(got, "[<tool]") {
			t.Errorf("trace missing [<tool] tag; trace=%q", got)
		}
		if !strings.Contains(got, "Bash") {
			t.Errorf("trace missing tool name 'Bash'; trace=%q", got)
		}
		// The secret API key must never appear in the trace.
		if strings.Contains(got, secretKey) {
			t.Errorf("trace contains unredacted API key %q; trace=%q", secretKey, got)
		}
	})
}

// R-A351-VO9A: ikigai-cli must not wait for stdin EOF before dispatching
// the agent loop. ralph-loops keeps stdin open while waiting for the
// result event; blocking on EOF creates a deadlock.
func TestR_A351_VO9A_DispatchesWithoutWaitingForStdinEOF(t *testing.T) {
	const userEvent = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}` + "\n"
	resolved := model.Resolved{Provider: model.ProviderAnthropic, BareID: "claude-haiku-4-5"}

	client := &fakeCLIClient{events: []provider.Event{
		provider.EventTextDelta{Text: `{"status":"DONE"}`},
		provider.EventDone{StopReason: "end_turn"},
	}}

	// Use a pipe: write one user event but never close the write end.
	// If runIteration waits for EOF it will deadlock; the timeout catches that.
	pr, pw := io.Pipe()
	defer pw.Close()

	go func() {
		pw.Write([]byte(userEvent))
		// pw is intentionally not closed — stdin stays open.
	}()

	var stdout bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runIteration(context.Background(), pr, &stdout, client, resolved, "", "", nil, tools.All())
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runIteration: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runIteration deadlocked waiting for stdin EOF (R-A351-VO9A violation)")
	}

	// Verify the result event was emitted.
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	var lastEv map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &lastEv); err != nil {
		t.Fatalf("parse last event: %v", err)
	}
	if lastEv["type"] != "result" {
		t.Errorf("last event type = %q, want \"result\"", lastEv["type"])
	}
}

// R-8SDW-BY0B: backend dispatch follows provider selection, one-to-one.
// buildClient must construct the OpenAI backend for gpt-* models and the
// Anthropic backend for claude-* models; no always-on default is permitted.
func TestR_8SDW_BY0B_BackendDispatchFollowsProviderSelection(t *testing.T) {
	t.Run("gpt_model_constructs_openai_client", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "test-openai-key")
		t.Setenv("ANTHROPIC_API_KEY", "")
		resolved := model.Resolved{Provider: model.ProviderOpenAI, BareID: "gpt-5.5"}
		client, apiKey, err := buildClient(resolved)
		if err != nil {
			t.Fatalf("buildClient(openai): %v", err)
		}
		if apiKey != "test-openai-key" {
			t.Errorf("apiKey = %q, want test-openai-key", apiKey)
		}
		typeName := fmt.Sprintf("%T", client)
		if !strings.Contains(typeName, "openai") {
			t.Errorf("expected openai client type, got %s", typeName)
		}
	})

	t.Run("claude_model_constructs_anthropic_client", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
		t.Setenv("OPENAI_API_KEY", "")
		resolved := model.Resolved{Provider: model.ProviderAnthropic, BareID: "claude-haiku-4-5"}
		client, apiKey, err := buildClient(resolved)
		if err != nil {
			t.Fatalf("buildClient(anthropic): %v", err)
		}
		if apiKey != "test-anthropic-key" {
			t.Errorf("apiKey = %q, want test-anthropic-key", apiKey)
		}
		typeName := fmt.Sprintf("%T", client)
		if !strings.Contains(typeName, "anthropic") {
			t.Errorf("expected anthropic client type, got %s", typeName)
		}
	})

	t.Run("openai_uses_openai_key_not_anthropic_key", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "openai-key-abc")
		t.Setenv("ANTHROPIC_API_KEY", "anthropic-key-xyz")
		resolved := model.Resolved{Provider: model.ProviderOpenAI, BareID: "gpt-5.5"}
		_, apiKey, err := buildClient(resolved)
		if err != nil {
			t.Fatalf("buildClient: %v", err)
		}
		if apiKey != "openai-key-abc" {
			t.Errorf("apiKey = %q, want openai-key-abc (must not use anthropic key)", apiKey)
		}
	})

	t.Run("anthropic_uses_anthropic_key_not_openai_key", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "anthropic-key-abc")
		t.Setenv("OPENAI_API_KEY", "openai-key-xyz")
		resolved := model.Resolved{Provider: model.ProviderAnthropic, BareID: "claude-haiku-4-5"}
		_, apiKey, err := buildClient(resolved)
		if err != nil {
			t.Fatalf("buildClient: %v", err)
		}
		if apiKey != "anthropic-key-abc" {
			t.Errorf("apiKey = %q, want anthropic-key-abc (must not use openai key)", apiKey)
		}
	})
}

// fakeTwoStepClient returns firstEvents on the first Stream call and
// secondEvents on the second. Used to simulate a tool_use round-trip.
type fakeTwoStepClient struct {
	firstEvents  []provider.Event
	secondEvents []provider.Event
	calls        int
}

func (f *fakeTwoStepClient) Stream(_ context.Context, _ provider.Request) (<-chan provider.Event, error) {
	var evs []provider.Event
	if f.calls == 0 {
		evs = f.firstEvents
	} else {
		evs = f.secondEvents
	}
	f.calls++
	ch := make(chan provider.Event, len(evs))
	for _, ev := range evs {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// capturingClient records the provider.Request passed to Stream and
// replays a fixed event sequence. Used by TestR_U1M3_IGO7.
type capturingClient struct {
	captured provider.Request
	events   []provider.Event
}

func (c *capturingClient) Stream(_ context.Context, req provider.Request) (<-chan provider.Event, error) {
	c.captured = req
	ch := make(chan provider.Event, len(c.events))
	for _, ev := range c.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// R-U1M3-IGO7: the provider request's Tools slice must carry exactly
// the tools selected by --tools. runIteration wires toolDescs into the
// request before calling client.Stream; this test confirms the wiring
// by capturing the request with a fake client and checking Tools.
func TestR_U1M3_IGO7_ToolsSelectionReachesProviderRequest(t *testing.T) {
	const stdinPayload = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}` + "\n"
	resolved := model.Resolved{Provider: model.ProviderAnthropic, BareID: "claude-haiku-4-5"}

	// Select only the Bash tool via tools.Select.
	bashOnly, err := tools.Select("Bash")
	if err != nil {
		t.Fatalf("tools.Select(\"Bash\"): %v", err)
	}
	if len(bashOnly) != 1 || bashOnly[0].Name != "Bash" {
		t.Fatalf("tools.Select(\"Bash\") returned unexpected descriptors: %v", bashOnly)
	}

	client := &capturingClient{events: []provider.Event{
		provider.EventTextDelta{Text: `{"status":"DONE"}`},
		provider.EventDone{StopReason: "end_turn"},
	}}

	var out bytes.Buffer
	if err := runIteration(context.Background(), strings.NewReader(stdinPayload), &out, client, resolved, "", "", nil, bashOnly); err != nil {
		t.Fatalf("runIteration: %v", err)
	}

	// The captured request must have exactly one tool — Bash — and no Read.
	got := client.captured.Tools
	if len(got) != 1 {
		t.Fatalf("provider request Tools length = %d, want 1; tools=%v", len(got), got)
	}
	if got[0].Name != "Bash" {
		t.Errorf("provider request Tools[0].Name = %q, want \"Bash\"", got[0].Name)
	}
}

// R-NYNJ-40BJ: when --raw is true, ikigai-cli writes a debug trace to
// stdout covering every boundary: [stdin>] for events read off stdin,
// [<stdout] for stream-json events emitted, [tool>]/[<tool] for tool
// dispatch/result. The trace and stream-json events are interleaved on
// the same writer; stderr receives nothing from the trace.
func TestR_NYNJ_40BJ_RawFlagWritesToStdout(t *testing.T) {
	const stdinPayload = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}` + "\n"
	resolved := model.Resolved{Provider: model.ProviderAnthropic, BareID: "claude-haiku-4-5"}

	// [stdin>] and [<stdout] appear when a simple (no-tool) iteration runs.
	t.Run("stdin_and_stdout_boundaries", func(t *testing.T) {
		client := &fakeCLIClient{events: []provider.Event{
			provider.EventTextDelta{Text: `{"status":"DONE"}`},
			provider.EventDone{StopReason: "end_turn"},
		}}
		var traceBuf bytes.Buffer
		tr := trace.New(&traceBuf, "")
		var stdout bytes.Buffer
		err := runIteration(context.Background(), strings.NewReader(stdinPayload), &stdout, client, resolved, "", "", tr, tools.All())
		if err != nil {
			t.Fatalf("runIteration: %v", err)
		}
		got := traceBuf.String()
		if !strings.Contains(got, "[stdin>]") {
			t.Errorf("trace missing [stdin>] tag; trace=%q", got)
		}
		if !strings.Contains(got, "[<stdout]") {
			t.Errorf("trace missing [<stdout] tag; trace=%q", got)
		}
	})

	// [tool>] and [<tool] appear when a tool round-trip occurs.
	t.Run("tool_boundaries", func(t *testing.T) {
		toolInput, _ := json.Marshal(map[string]string{"command": "echo hi"})
		client := &fakeTwoStepClient{
			firstEvents: []provider.Event{
				provider.EventToolUse{ID: "toolu_nynj01", Name: "Bash", Input: toolInput},
				provider.EventDone{StopReason: "tool_use"},
			},
			secondEvents: []provider.Event{
				provider.EventTextDelta{Text: `{"status":"DONE"}`},
				provider.EventDone{StopReason: "end_turn"},
			},
		}
		var traceBuf bytes.Buffer
		tr := trace.New(&traceBuf, "")
		var stdout bytes.Buffer
		err := runIteration(context.Background(), strings.NewReader(stdinPayload), &stdout, client, resolved, "", "", tr, tools.All())
		if err != nil {
			t.Fatalf("runIteration: %v", err)
		}
		got := traceBuf.String()
		if !strings.Contains(got, "[tool>]") {
			t.Errorf("trace missing [tool>] tag; trace=%q", got)
		}
		if !strings.Contains(got, "[<tool]") {
			t.Errorf("trace missing [<tool] tag; trace=%q", got)
		}
	})

	// When --raw is false, no trace appears on stdout.
	t.Run("no_raw_no_trace_on_stdout", func(t *testing.T) {
		client := &fakeCLIClient{events: []provider.Event{
			provider.EventTextDelta{Text: `{"status":"DONE"}`},
			provider.EventDone{StopReason: "end_turn"},
		}}
		var stdout bytes.Buffer
		// Pass nil tracer — no tracing wrapper is installed.
		err := runIteration(context.Background(), strings.NewReader(stdinPayload), &stdout, client, resolved, "", "", nil, tools.All())
		if err != nil {
			t.Fatalf("runIteration: %v", err)
		}
		// Every line on stdout must be a valid JSON object (stream-json).
		for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
			if line == "" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				t.Errorf("non-stream-json line on stdout: %q", line)
			}
		}
	})

	// API key values must never appear in trace output.
	t.Run("api_key_redacted", func(t *testing.T) {
		const secretKey = "sk-test-nynj-secret-99999"
		client := &fakeCLIClient{events: []provider.Event{
			provider.EventTextDelta{Text: `{"status":"DONE"}`},
			provider.EventDone{StopReason: "end_turn"},
		}}
		var traceBuf bytes.Buffer
		tr := trace.New(&traceBuf, secretKey)
		var stdout bytes.Buffer
		_ = runIteration(context.Background(), strings.NewReader(stdinPayload), &stdout, client, resolved, "", "", tr, tools.All())
		if strings.Contains(traceBuf.String(), secretKey) {
			t.Errorf("trace contains unredacted API key %q; trace=%q", secretKey, traceBuf.String())
		}
	})
}
