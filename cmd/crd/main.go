package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/atlassian/smith/pkg/resources"
)

func main() {
	if err := innerMain(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%#v", err) // nolint: gas
		// not handling above error, if we can't output to stderr, what can we do?
		os.Exit(1)
	}
}

func innerMain() error {
	printBundle := flag.String("print-bundle", "yaml", "Print Bundle CRD and exit (specify format: json or yaml)")
	flag.Parse()

	return resources.PrintCleanedObject(os.Stdout, *printBundle, resources.BundleCrd())
}
