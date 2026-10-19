package resources_test

import (
	"context"
	"testing"

	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
)

// The sweepers can run against a production tenant, so pin down exactly what they may delete.
func TestSweepers_deleteOnlyAcceptanceTestObjects(t *testing.T) {
	srv := fakereco.New(t)
	c := client.New(srv.URL, fakereco.APIKey, "test")
	ctx := context.Background()

	srv.SeedUser(client.RecoUser{EmailAddress: "tf-acc-a" + testAccEmailDomain})
	srv.SeedUser(client.RecoUser{EmailAddress: "tf-acc-b@example.com"}) // right prefix, wrong domain
	srv.SeedUser(client.RecoUser{EmailAddress: "alice" + testAccEmailDomain})
	srv.SeedUser(client.RecoUser{EmailAddress: "alice@acme.com"})

	srv.SeedRole(client.RecoRole{Name: "tf-acc-role"})
	srv.SeedRole(client.RecoRole{Name: "prod-role"})
	srv.SeedRole(client.RecoRole{Name: "TF-ACC-upper"})
	srv.SeedRole(client.RecoRole{Name: "tf-acc-system", Type: "USER_ROLE_TYPE_SYSTEM"})

	srv.SeedAPIKey(client.ApiKey{Name: "tf-acc-key"})
	srv.SeedAPIKey(client.ApiKey{Name: "ci-deploy-key"})
	srv.SeedAPIKey(client.ApiKey{Name: "my-tf-acc-key"})

	srv.SeedPostureCheck(client.PostureCheck{Name: "tf-acc-posture"})
	srv.SeedPostureCheck(client.PostureCheck{Name: "Real posture check"})

	srv.SeedPolicy(client.ThreatDetectionPolicy{Name: "tf-acc-policy"})
	srv.SeedPolicy(client.ThreatDetectionPolicy{Name: "Real ITDR policy"})

	for name, sweep := range map[string]func(context.Context, *client.Client) error{
		"users": sweepUsers, "roles": sweepRoles, "api keys": sweepAPIKeys,
		"posture checks": sweepPostureChecks, "policies": sweepPolicies,
	} {
		if err := sweep(ctx, c); err != nil {
			t.Fatalf("sweep %s: %v", name, err)
		}
		// Sweeping twice must be harmless.
		if err := sweep(ctx, c); err != nil {
			t.Fatalf("second sweep of %s: %v", name, err)
		}
	}

	var users, roles, keys, checks, policies []string //nolint:prealloc // sizes are only known after the sweep
	for _, u := range srv.Users() {
		users = append(users, u.EmailAddress)
	}
	for _, r := range srv.Roles() {
		roles = append(roles, r.Name)
	}
	for _, k := range srv.APIKeys() {
		keys = append(keys, k.Name)
	}
	for _, p := range srv.PostureChecks() {
		checks = append(checks, p.Name)
	}
	for _, p := range srv.Policies() {
		policies = append(policies, p.Name)
	}

	assertSet(t, "users", users, "tf-acc-b@example.com", "alice"+testAccEmailDomain, "alice@acme.com")
	assertSet(t, "roles", roles, "ROLE_SECURITY_VIEWER", "ROLE_ADMIN", "prod-role", "TF-ACC-upper", "tf-acc-system")
	assertSet(t, "api keys", keys, "ci-deploy-key", "my-tf-acc-key")
	assertSet(t, "posture checks", checks, "Real posture check")
	assertSet(t, "policies", policies, "Real ITDR policy")
}

func assertSet(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	remaining := map[string]int{}
	for _, w := range want {
		remaining[w]++
	}
	for _, g := range got {
		if remaining[g] == 0 {
			t.Errorf("%s: unexpected survivor %q (got %v, want %v)", what, g, got, want)
			continue
		}
		remaining[g]--
	}
	for w, n := range remaining {
		if n > 0 {
			t.Errorf("%s: %q was deleted but must be kept (got %v)", what, w, got)
		}
	}
}

func TestIsSweepable(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"tf-acc-x", true},
		{"tf-acc-", true},
		{"", false},
		{"tf-acc", false},
		{"TF-ACC-x", false},
		{" tf-acc-x", false},
		{"prod-tf-acc-x", false},
		{"tf_acc_x", false},
	}
	for _, tt := range tests {
		if got := isSweepable(tt.name); got != tt.want {
			t.Errorf("isSweepable(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
