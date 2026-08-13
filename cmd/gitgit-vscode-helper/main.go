package main

import (
	"context"
	"fmt"
	"os"

	"github.com/yunsang/gitgit/internal/vscodehelper"
)

func main() {
	if err := os.Setenv("GIT_NO_LAZY_FETCH", "1"); err != nil {
		fmt.Fprintf(os.Stderr, "configure offline Git behavior: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("GIT_TERMINAL_PROMPT", "0"); err != nil {
		fmt.Fprintf(os.Stderr, "disable Git prompts: %v\n", err)
		os.Exit(1)
	}

	server := vscodehelper.NewServer()
	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "gitgit-vscode-helper: %v\n", err)
		os.Exit(1)
	}
}
