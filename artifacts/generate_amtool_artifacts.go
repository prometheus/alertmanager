package main

import (
	"github.com/spf13/cobra/doc"

	"github.com/prometheus/alertmanager/cli"
)

func main() {
	cli.RootCmd.GenBashCompletionFile("amtool_completion.sh")
	header := &doc.GenManHeader{
		Title:   "amtool",
		Section: "1",
	}

	doc.GenManTree(cli.RootCmd, header, ".")
}
