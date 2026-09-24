package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/recolabs/terraform-provider-reco/internal/provider"
)

//go:generate go tool tfplugindocs generate --provider-name reco

var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run in debug mode")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/recolabs/reco",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err)
	}
}
