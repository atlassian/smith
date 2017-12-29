package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/atlassian/smith/pkg/resources"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
)

func main() {
	if err := innerMain(); err != nil {
		log.Fatal(err)
	}
}

func innerMain() error {
	printBundleSchema := flag.String("print-bundle-schema", "yaml", "Print Bundle schema and exit (specify format: json or yaml)")
	flag.Parse()

	switch *printBundleSchema {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		err := enc.Encode(resources.BundleCrd().Spec.Validation.OpenAPIV3Schema)
		if err != nil {
			return errors.Wrap(err, "failed to marshal Bundle schema into JSON")
		}
	case "yaml":
		data, err := yaml.Marshal(resources.BundleCrd().Spec.Validation.OpenAPIV3Schema)
		if err != nil {
			return errors.Wrap(err, "failed to marshal Bundle schema into YAML")
		}
		_, err = os.Stdout.Write(data)
		if err != nil {
			return errors.Wrap(err, "failed to write Bundle schema YAML to stdout")
		}
	default:
		return errors.Errorf("unsupported schema output format %q", *printBundleSchema)
	}
	return nil
}
