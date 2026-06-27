package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/Tsinling0525/rivulet/nodes/echo"
	_ "github.com/Tsinling0525/rivulet/nodes/eval"
	_ "github.com/Tsinling0525/rivulet/nodes/files"
	_ "github.com/Tsinling0525/rivulet/nodes/fs"
	_ "github.com/Tsinling0525/rivulet/nodes/http"
	_ "github.com/Tsinling0525/rivulet/nodes/llmroute"
	_ "github.com/Tsinling0525/rivulet/nodes/logic"
	_ "github.com/Tsinling0525/rivulet/nodes/merge"
	_ "github.com/Tsinling0525/rivulet/nodes/ollama"
	_ "github.com/Tsinling0525/rivulet/nodes/openai"
	_ "github.com/Tsinling0525/rivulet/nodes/review"
	_ "github.com/Tsinling0525/rivulet/nodes/wasm"
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
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		file := fs.String("file", "", "Path to n8n workflow JSON")
		_ = fs.Parse(os.Args[2:])
		if *file == "" {
			fmt.Println("--file is required")
			os.Exit(2)
		}
		if err := runFlowFromFile(*file); err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
	case "sample":
		if err := runEchoSample(); err != nil {
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
	fmt.Println("  rivulet agent [--once goal] # run the coding agent loop")
	fmt.Println("  rivulet run --file path    # run workflow JSON once")
	fmt.Println("  rivulet sample             # run the built-in echo sample")
}
