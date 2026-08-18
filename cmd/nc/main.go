package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		return
	}

	switch os.Args[1] {
	case "due":
		fs := flag.NewFlagSet("due", flag.ExitOnError)
		limit := fs.Int("limit", 5, "max problems to show")
		fs.Parse(os.Args[2:])
		fmt.Println(*limit)
	case "log":
		fmt.Println("log")
	case "list":
		fmt.Println("list")
	default:
		fmt.Fprintln(os.Stderr, "Command not recognized")
	}
}
