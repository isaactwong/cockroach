// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"testing"

	"github.com/alessio/shellescape"
	"github.com/cockroachdb/cockroach/pkg/cmd/dev/io/exec"
	"github.com/cockroachdb/cockroach/pkg/cmd/dev/io/os"
	"github.com/cockroachdb/cockroach/pkg/testutils/datapathutils"
	"github.com/cockroachdb/datadriven"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

const (
	crdbCheckoutPlaceholder    = "crdb-checkout"
	sandboxPlaceholder         = "sandbox"
	testFixturesDirPlaceholder = "crdb-mock-test-fixtures"
)

// TestDataDriven makes use of datadriven to capture all operations executed by
// individual dev invocations. The testcases are defined under
// testdata/datadriven/*.
//
// DataDriven divvies up these files as subtests, so individual "files" are
// runnable through:
//
//	 		dev test pkg/cmd/dev -f TestDataDriven/<fname> [--rewrite]
//		OR  go test ./pkg/cmd/dev -run TestDataDriven/<fname> [-rewrite]
//
// NB: See commentary on TestRecorderDriven to see how they compare.
// TestDataDriven is well suited for exercising flows that don't depend on
// reading external state in order to function (simply translating a `dev test
// <target>` to its corresponding bazel invocation for e.g.). It's not well
// suited for flows that do (reading a list of go files in the bazel generated
// sandbox and copying them over one-by-one).
func TestDataDriven(t *testing.T) {
	verbose := testing.Verbose()
	testdata := datapathutils.TestDataPath(t, "datadriven")
	datadriven.Walk(t, testdata, func(t *testing.T, path string) {
		// We'll match against printed logs for datadriven.
		var logger io.ReadWriter = bytes.NewBufferString("")
		execOpts := []exec.Option{
			exec.WithLogger(log.New(logger, "", 0)),
			exec.WithDryrun(),
			exec.WithIntercept(workspaceCmd(), crdbCheckoutPlaceholder),
			exec.WithIntercept(bazelbinCmd(), sandboxPlaceholder),
			exec.WithIntercept(bazelbinPgoCmd(), sandboxPlaceholder),
		}
		osOpts := []os.Option{
			os.WithLogger(log.New(logger, "", 0)),
			os.WithDryrun(),
			os.WithIntercept("echo $HOME/.cache", testFixturesDirPlaceholder),
		}

		if !verbose { // suppress all internal output unless told otherwise
			execOpts = append(execOpts, exec.WithStdOutErr(io.Discard, io.Discard))
		}

		devExec := exec.New(execOpts...)
		devOS := os.New(osOpts...)

		// TODO(irfansharif): Because these tests are run in dry-run mode, if
		// "accidentally" adding a test for a mixed-io command (see top-level test
		// comment), it may appear as a test failure where the output of a
		// successful shell-out attempt returns an empty response, maybe resulting
		// in NPEs. We could catch these panics/errors here and suggest a more
		// informative error to test authors.

		datadriven.RunTest(t, path, func(t *testing.T, d *datadriven.TestData) string {
			dev := makeDevCmd()
			dev.exec, dev.os = devExec, devOS
			dev.knobs.devBinOverride = "dev"
			dev.log = log.New(logger, "", 0)

			if !verbose {
				dev.cli.SetErr(io.Discard)
				dev.cli.SetOut(io.Discard)
			}

			require.Equalf(t, d.Cmd, "exec", "unknown command: %s", d.Cmd)
			tokens, err := shlex.Split(d.Input)
			require.NoError(t, err)
			require.NotEmpty(t, tokens)
			require.Equal(t, "dev", tokens[0])

			dev.cli.SetArgs(tokens[1:])

			if err := dev.cli.Execute(); err != nil {
				return fmt.Sprintf("err: %s", err)
			}
			logs, err := io.ReadAll(logger)
			require.NoError(t, err)
			return string(logs)
		})
	})
}

func workspaceCmd() string {
	return fmt.Sprintf("bazel %s", shellescape.QuoteCommand([]string{"info", "workspace", "--color=no"}))
}

func bazelbinCmd() string {
	return fmt.Sprintf("bazel %s", shellescape.QuoteCommand([]string{"info", "bazel-bin", "--color=no"}))
}

func bazelbinPgoCmd() string {
	return fmt.Sprintf("bazel %s", shellescape.QuoteCommand([]string{"info", "bazel-bin", "--color=no", "--config=pgo"}))
}
