// Package acctestutil holds what the acceptance tests of internal/resources and internal/datasources
// share. Keeping Prefix in one place matters: the sweepers delete only objects carrying it, so every
// test that creates an object on a tenant must name it with this package.
package acctestutil

import (
	"os"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
)

const (
	// Prefix marks every object the acceptance tests create on a tenant.
	Prefix = "tf-acc-"

	// EmailDomain is the domain of every test user; the user sweeper additionally requires it.
	EmailDomain = "@reco-test.example.com"
)

// Suffix returns a random suffix so concurrent runs against the same tenant never collide.
func Suffix() string {
	return acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
}

// Permission is the permission test roles are granted; override it with RECO_TEST_PERMISSION when
// the tenant does not know the default.
func Permission() string {
	if p := os.Getenv("RECO_TEST_PERMISSION"); p != "" {
		return p
	}
	return "PERM_ALERTS_READ"
}
