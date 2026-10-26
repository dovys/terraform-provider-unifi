//go:build ignore

// strip-spec-prose removes every string-valued `description` and `summary` from
// an OpenAPI document so the vendor's prose never reaches the generated wire
// types (oapi-codegen copies it into Go comments). Only string values go: a
// property or schema that happens to be NAMED description is a mapping and is
// kept. Works on yaml.v3 nodes so every other scalar keeps its original form.
//
// Usage: go run strip-spec-prose.go <in.yaml> <out.yaml>
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func strip(n *yaml.Node) {
	if n.Kind == yaml.MappingNode {
		kept := n.Content[:0]
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if (k.Value == "description" || k.Value == "summary") && v.Kind == yaml.ScalarNode {
				continue
			}
			kept = append(kept, k, v)
		}
		n.Content = kept
	}
	for _, c := range n.Content {
		strip(c)
	}
}

func run(in, out string) error {
	src, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", in, err)
	}
	strip(&doc)
	dst, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(out, dst, 0o600)
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run strip-spec-prose.go <in.yaml> <out.yaml>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "strip-spec-prose:", err)
		os.Exit(1)
	}
}
