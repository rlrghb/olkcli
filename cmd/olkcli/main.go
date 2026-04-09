package main

import (
	"os"

	"github.com/rlrghb/olkcli/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
