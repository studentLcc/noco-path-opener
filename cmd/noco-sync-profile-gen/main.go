package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"noco-path-opener/internal/profilegen"
)

var runProfileGenerator = profilegen.Run

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	var configPath string
	var write bool

	flags := flag.NewFlagSet("noco-sync-profile-gen", flag.ContinueOnError)
	flags.SetOutput(errOut)
	flags.StringVar(&configPath, "config", "config.json", "path to config file")
	flags.BoolVar(&write, "write", false, "append generated profile to config file")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if flags.NArg() > 0 {
		fmt.Fprintf(errOut, "error: unexpected arguments: %v\n", flags.Args())
		return 2
	}

	if err := runProfileGenerator(context.Background(), profilegen.Options{
		ConfigPath: configPath,
		Write:      write,
		In:         in,
		Out:        out,
		Err:        errOut,
	}); err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	return 0
}
