package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tsinling0525/rivulet/dsl"
)

// Code runs a snippet of local code, matching Dify's code node contract: the
// snippet defines main(...) taking the mapped variables and returns a dict.
//
// SECURITY: the snippet executes as a local process with the caller's
// privileges. Only run workflow files you trust.
type Code struct{}

func (Code) Type() string { return dsl.NodeCode }

const (
	codeDefaultLanguage = "python3"
	codeDefaultTimeout  = 60 * time.Second
)

// runnerSource is appended to the user's snippet. It reads the mapped variables
// as JSON on stdin, calls main with them by name, and prints the returned dict.
const runnerSource = `
import json as _rivulet_json
import sys as _rivulet_sys

def _rivulet_entry():
    return main(**_rivulet_json.loads(_rivulet_sys.stdin.read() or "{}"))

_rivulet_result = _rivulet_entry()
if not isinstance(_rivulet_result, dict):
    raise SystemExit("code node main() must return a dict, got %s" % type(_rivulet_result).__name__)
_rivulet_sys.stdout.write(_rivulet_json.dumps(_rivulet_result))
`

type codeVariable struct {
	Variable      string       `yaml:"variable"`
	ValueSelector dsl.Selector `yaml:"value_selector"`
}

type codeConfig struct {
	Code         string                 `yaml:"code"`
	CodeLanguage string                 `yaml:"code_language"`
	Variables    []codeVariable         `yaml:"variables"`
	Outputs      map[string]any         `yaml:"outputs"`
	Extra        map[string]interface{} `yaml:",inline"`
}

func (Code) Validate(node dsl.Node) []dsl.Problem {
	var cfg codeConfig
	if problems := decodeExtra(node, &cfg); problems != nil {
		return problems
	}
	var problems []dsl.Problem
	if strings.TrimSpace(cfg.Code) == "" {
		problems = append(problems, problem(node, "code must not be empty"))
	}
	language := strings.ToLower(strings.TrimSpace(cfg.CodeLanguage))
	switch language {
	case "", codeDefaultLanguage:
	case "javascript", "python2":
		problems = append(problems, problem(node, "code_language %q is not implemented; only python3 is supported", cfg.CodeLanguage))
	default:
		problems = append(problems, problem(node, "code_language %q is not supported; only python3 is supported", cfg.CodeLanguage))
	}
	for index, variable := range cfg.Variables {
		if strings.TrimSpace(variable.Variable) == "" {
			problems = append(problems, problem(node, "variables[%d] has no name", index))
		}
	}
	return problems
}

func (Code) Run(ctx context.Context, req Request) (Output, error) {
	var cfg codeConfig
	if err := req.Node.Data.Decode(&cfg); err != nil {
		return Output{}, nodeError(req.Node, "invalid configuration: %v", err)
	}

	inputs := make(map[string]any, len(cfg.Variables))
	for _, variable := range cfg.Variables {
		value, ok := req.Resolve(variable.ValueSelector)
		if !ok {
			return Output{}, nodeError(req.Node,
				"variable %q references %s, which produced no value", variable.Variable, variable.ValueSelector)
		}
		inputs[variable.Variable] = value
	}

	payload, err := json.Marshal(inputs)
	if err != nil {
		return Output{}, nodeError(req.Node, "encode inputs: %v", err)
	}

	language := strings.ToLower(strings.TrimSpace(cfg.CodeLanguage))
	if language == "" {
		language = codeDefaultLanguage
	}
	if language != codeDefaultLanguage {
		return Output{}, nodeError(req.Node, "code_language %q is not supported; only python3 is supported", cfg.CodeLanguage)
	}
	interpreter := interpreterFor(language, req.WorkDir)

	dir := req.WorkDir
	if dir == "" {
		dir = os.TempDir()
	}
	script := filepath.Join(dir, fmt.Sprintf(".rivulet-code-%s.py", sanitize(req.Node.ID)))
	if err := os.WriteFile(script, []byte(cfg.Code+"\n"+runnerSource), 0o600); err != nil {
		return Output{}, nodeError(req.Node, "write script: %v", err)
	}
	defer os.Remove(script)

	runCtx, cancel := context.WithTimeout(ctx, codeDefaultTimeout)
	defer cancel()
	command := exec.CommandContext(runCtx, interpreter, script)
	command.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		detail := summarizeStderr(stderr.String())
		if runCtx.Err() != nil {
			return Output{}, nodeError(req.Node, "code timed out after %s", codeDefaultTimeout)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return Output{}, nodeError(req.Node, "code failed (exit %d): %s", exitErr.ExitCode(), detail)
		}
		return Output{}, nodeError(req.Node, "run %s: %v", interpreter, err)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return Output{}, nodeError(req.Node, "code produced no output")
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(output), &fields); err != nil {
		return Output{}, nodeError(req.Node, "code output is not a JSON object: %v (output: %s)",
			err, truncateForError(output))
	}
	if len(cfg.Outputs) > 0 {
		for name := range cfg.Outputs {
			if _, ok := fields[name]; !ok {
				req.Log("declared output %q is missing from the code result", name)
			}
		}
	}
	return Output{Fields: fields}, nil
}

// interpreterFor looks the interpreter up on PATH so the failure mode is an
// actionable error rather than exec's bare "file not found".
func interpreterFor(language, _ string) string {
	if language == codeDefaultLanguage {
		if path, err := exec.LookPath("python3"); err == nil {
			return path
		}
		return "python3"
	}
	return language
}

// summarizeStderr keeps the failing line, which is where python puts the actual
// error, while still showing where it happened.
func summarizeStderr(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "no error output"
	}
	lines := splitLines(trimmed)
	last := lines[len(lines)-1]
	if len(lines) == 1 {
		return truncateForError(last)
	}
	return fmt.Sprintf("%s (after %d earlier line(s))", truncateForError(last), len(lines)-1)
}

func sanitize(id string) string {
	replaced := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, id)
	if replaced == "" {
		return "node"
	}
	return replaced
}
