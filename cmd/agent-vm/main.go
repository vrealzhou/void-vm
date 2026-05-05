package main

import (
	"fmt"
	"os"

	"github.com/vrealzhou/agent-vm/internal/vmctl"
)

func main() {
	rootCmd, err := vmctl.NewRootCommand()
	if err != nil {
		fatalf("%v", err)
	}
	if err := rootCmd.Execute(); err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	_, _ = os.Stderr.WriteString("[agent-vm] ERROR: " + fmt.Sprintf(format, args...) + "\n")
	os.Exit(1)
}
