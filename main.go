// Command pointless is a linter that suggests using value types instead of pointers.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/mickamy/pointless/internal/analyzer"
	"github.com/mickamy/pointless/internal/config"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	// Load config file before flag parsing
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pointless: warning: failed to load config: %v\n", err)
	}

	// Set default from config file if not overridden by flags
	if cfg.Threshold > 0 {
		// Check if -threshold flag is explicitly set
		thresholdSet := false

		for _, arg := range os.Args[1:] {
			if arg == "-threshold" || (len(arg) > 10 && arg[:11] == "-threshold=") {
				thresholdSet = true

				break
			}
		}

		if !thresholdSet {
			// Inject the config value as a flag (insert after program name, before other args)
			newArgs := make([]string, 0, len(os.Args)+1)
			newArgs = append(newArgs, os.Args[0], "-threshold="+strconv.Itoa(cfg.Threshold))
			newArgs = append(newArgs, os.Args[1:]...)
			os.Args = newArgs
		}
	}

	// Store config in analyzer for exclude pattern support
	analyzer.SetConfig(cfg.Exclude)

	singlechecker.Main(analyzer.Analyzer)
}

func init() {
	// Add version flag.
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pointless: suggests using value types instead of pointers for small structs\n\n")
		fmt.Fprintf(os.Stderr, "Usage: pointless [flags] [packages]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nConfiguration:\n")
		fmt.Fprintf(os.Stderr, "  Create .pointless.yaml in your project root:\n")
		fmt.Fprintf(os.Stderr, "    threshold: 1024  # bytes\n")
		fmt.Fprintf(os.Stderr, "    exclude:\n")
		fmt.Fprintf(os.Stderr, "      - \"*_test.go\"\n")
		fmt.Fprintf(os.Stderr, "      - \"vendor/**\"\n")
	}
}
