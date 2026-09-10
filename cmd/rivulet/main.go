package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "agent":
		if err := runAgentCLI(os.Args[2:]); err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
	case "trigger":
		if err := runTriggerCLI(os.Args[2:]); err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  rivulet agent [--once goal]              # run the coding agent loop")
	fmt.Println("  rivulet trigger --url URL [--data JSON]  # trigger an external n8n/Dify workflow")
}
