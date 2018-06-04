package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/atlassian/smith/pkg/resources"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
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

	switch *printBundle {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		err := enc.Encode(resources.BundleCrd())
		if err != nil {
			return errors.Wrap(err, "failed to marshal Bundle CRD into JSON")
		}
	case "yaml":
		data, err := yaml.Marshal(resources.BundleCrd())
		if err != nil {
			return errors.Wrap(err, "failed to marshal Bundle CRD into YAML")
		}
		_, err = os.Stdout.Write(data)
		if err != nil {
			return errors.Wrap(err, "failed to write Bundle CRD YAML to stdout")
		}
	default:
		return errors.Errorf("unsupported Bundle CRD output format %q", *printBundle)
	}
	return nil
}
