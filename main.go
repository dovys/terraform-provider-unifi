package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/dovys/terraform-provider-unifi/internal/provider"
)

// version is set by the release build (goreleaser ldflags); "dev" otherwise.
var version = "dev"

//go:generate go tool tfplugindocs generate --provider-name unifi

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/dovys/unifi",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
