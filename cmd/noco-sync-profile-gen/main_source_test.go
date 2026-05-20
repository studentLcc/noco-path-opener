package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"noco-path-opener/internal/profilegen"
)

func TestRunUsesDefaultFlags(t *testing.T) {
	in := strings.NewReader("input")
	var out bytes.Buffer
	var errOut bytes.Buffer
	var got profilegen.Options

	overrideRunProfileGenerator(t, func(ctx context.Context, opts profilegen.Options) error {
		if ctx == nil {
			t.Fatalf("context is nil")
		}
		got = opts
		return nil
	})

	code := run(nil, in, &out, &errOut)
	if code != 0 {
		t.Fatalf("run() code = %d, want 0", code)
	}
	if got.ConfigPath != "config.json" {
		t.Fatalf("ConfigPath = %q, want config.json", got.ConfigPath)
	}
	if got.Write {
		t.Fatalf("Write = true, want false")
	}
	if got.In != in {
		t.Fatalf("In was not passed through")
	}
	if got.Out != &out {
		t.Fatalf("Out was not passed through")
	}
	if got.Err != &errOut {
		t.Fatalf("Err was not passed through")
	}
}

func TestRunPassesCustomFlagsToProfileGenerator(t *testing.T) {
	in := strings.NewReader("input")
	var out bytes.Buffer
	var errOut bytes.Buffer
	var got profilegen.Options

	overrideRunProfileGenerator(t, func(ctx context.Context, opts profilegen.Options) error {
		if ctx == nil {
			t.Fatalf("context is nil")
		}
		got = opts
		return nil
	})

	code := run([]string{"-config", "custom.json", "-write"}, in, &out, &errOut)
	if code != 0 {
		t.Fatalf("run() code = %d, want 0", code)
	}
	if got.ConfigPath != "custom.json" {
		t.Fatalf("ConfigPath = %q, want custom.json", got.ConfigPath)
	}
	if !got.Write {
		t.Fatalf("Write = false, want true")
	}
	if got.In != in {
		t.Fatalf("In was not passed through")
	}
	if got.Out != &out {
		t.Fatalf("Out was not passed through")
	}
	if got.Err != &errOut {
		t.Fatalf("Err was not passed through")
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestRunRejectsFlagParseError(t *testing.T) {
	var errOut bytes.Buffer
	called := false

	overrideRunProfileGenerator(t, func(context.Context, profilegen.Options) error {
		called = true
		return nil
	})

	code := run([]string{"-unknown"}, strings.NewReader(""), io.Discard, &errOut)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if called {
		t.Fatalf("runner was called")
	}
	if !strings.Contains(errOut.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %q, want flag parse error", errOut.String())
	}
}

func TestRunRejectsUnexpectedPositionalArgs(t *testing.T) {
	var errOut bytes.Buffer
	called := false

	overrideRunProfileGenerator(t, func(context.Context, profilegen.Options) error {
		called = true
		return nil
	})

	code := run([]string{"extra", "args"}, strings.NewReader(""), io.Discard, &errOut)
	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if called {
		t.Fatalf("runner was called")
	}
	if !strings.Contains(errOut.String(), "error: unexpected arguments:") {
		t.Fatalf("stderr = %q, want unexpected arguments error", errOut.String())
	}
}

func TestRunReportsProfileGeneratorError(t *testing.T) {
	var errOut bytes.Buffer
	runErr := errors.New("boom")

	overrideRunProfileGenerator(t, func(context.Context, profilegen.Options) error {
		return runErr
	})

	code := run(nil, strings.NewReader(""), io.Discard, &errOut)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if got, want := errOut.String(), "error: boom\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func overrideRunProfileGenerator(t *testing.T, fn func(context.Context, profilegen.Options) error) {
	t.Helper()
	original := runProfileGenerator
	runProfileGenerator = fn
	t.Cleanup(func() {
		runProfileGenerator = original
	})
}
