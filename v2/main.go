package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/runpod/terraform-provider-runpod/v2/internal/provider"
)

// version is stamped in at build time with -ldflags -X main.version=<version>
// (see the build task in Taskfile.yml); a plain `go build` leaves it "dev".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/runpod/runpod",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
