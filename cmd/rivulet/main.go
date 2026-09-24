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

	var err error
	switch os.Args[1] {
	case "run":
		err = runWorkflowCLI(os.Args[2:])
	case "validate":
		err = validateWorkflowCLI(os.Args[2:])
	case "nodes":
		err = listNodesCLI(os.Args[2:])
	case "agent":
		err = runAgentCLI(os.Args[2:])
	default:
		printUsage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Rivulet runs Dify-style workflow DSL files locally.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  rivulet run --file app.dify.yml [--input k=v ...]   # run a workflow")
	fmt.Println("  rivulet validate --file app.dify.yml                # check a workflow")
	fmt.Println("  rivulet nodes                                       # list node types")
	fmt.Println("  rivulet agent [--once goal]                         # coding agent loop")
}
