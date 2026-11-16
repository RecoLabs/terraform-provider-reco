// Command fakereco serves the in-memory fake Reco API so the acceptance tests can be tried
// without a real tenant:
//
//	go run ./internal/fakereco/cmd/fakereco
//	TF_ACC=1 RECO_API_KEY=fake-api-key RECO_BASE_URL=<printed url> go test ./internal/... -run '^TestAcc'
package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

func main() {
	srv := fakereco.Start()
	defer srv.Close()

	fmt.Printf("fake Reco API listening on %s (RECO_API_KEY=%s)\n", srv.URL, fakereco.APIKey)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}
