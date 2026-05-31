// Command mealie is a fast, scriptable, agent-friendly CLI for Mealie.
package main

import (
	"os"

	"github.com/gloos/mealie-cli/internal/cli"
)

func main() {
	os.Exit(cli.Main())
}
