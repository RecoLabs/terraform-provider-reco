package resources_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/recolabs/terraform-provider-reco/internal/acctestutil"
	"github.com/recolabs/terraform-provider-reco/internal/client"
)

// Sweepers delete objects leaked by acceptance runs that were killed before they could clean up.
// Run them with:
//
//	RECO_API_KEY=... RECO_BASE_URL=... go test ./internal/resources -sweep=reco -timeout 30m
//
// They may run against a production tenant, so they only ever delete objects whose name carries
// acctestutil.Prefix (users additionally need the test email domain) and never touch system roles.
const customRoleType = "USER_ROLE_TYPE_CUSTOM"

func TestMain(m *testing.M) {
	resource.TestMain(m)
}

func init() {
	resource.AddTestSweepers("reco_user", &resource.Sweeper{Name: "reco_user", F: sweeperFunc(sweepUsers)})
	resource.AddTestSweepers("reco_api_key", &resource.Sweeper{Name: "reco_api_key", F: sweeperFunc(sweepAPIKeys)})
	resource.AddTestSweepers("reco_role", &resource.Sweeper{
		Name: "reco_role", F: sweeperFunc(sweepRoles),
		Dependencies: []string{"reco_user", "reco_api_key"}, // roles can't be deleted while in use
	})
	resource.AddTestSweepers("reco_posture_check", &resource.Sweeper{Name: "reco_posture_check", F: sweeperFunc(sweepPostureChecks)})
	resource.AddTestSweepers("reco_threat_detection_policy", &resource.Sweeper{
		Name: "reco_threat_detection_policy", F: sweeperFunc(sweepPolicies),
	})
}

func sweeperFunc(sweep func(context.Context, *client.Client) error) func(string) error {
	return func(string) error {
		baseURL, apiKey := os.Getenv("RECO_BASE_URL"), os.Getenv("RECO_API_KEY")
		if baseURL == "" || apiKey == "" {
			return fmt.Errorf("RECO_BASE_URL and RECO_API_KEY must be set to run sweepers")
		}
		return sweep(context.Background(), client.New(baseURL, apiKey, "sweeper"))
	}
}

func isSweepable(name string) bool {
	return strings.HasPrefix(name, acctestutil.Prefix)
}

// Each sweeper keeps going when a delete fails (for example a role still assigned to a user) and
// reports every failure at the end, so one stuck object cannot hide the rest of its type.

func sweepUsers(ctx context.Context, c *client.Client) error {
	users, err := c.ListAllUsers(ctx, "")
	if err != nil {
		return err
	}
	var errs []error
	for i := range users {
		u := &users[i]
		if isSweepable(u.EmailAddress) && strings.HasSuffix(u.EmailAddress, acctestutil.EmailDomain) {
			if err := c.DeleteUser(ctx, u.UserID); err != nil && !client.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("delete user %q: %w", u.EmailAddress, err))
			}
		}
	}
	return errors.Join(errs...)
}

func sweepAPIKeys(ctx context.Context, c *client.Client) error {
	keys, err := c.ListAllApiKeys(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, k := range keys {
		if isSweepable(k.Name) {
			if err := c.DeleteApiKey(ctx, k.ID); err != nil && !client.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("delete API key %q: %w", k.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

func sweepRoles(ctx context.Context, c *client.Client) error {
	roles, err := c.ListAllRoles(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range roles {
		if isSweepable(r.Name) && r.Type == customRoleType {
			if err := c.DeleteRole(ctx, r.Name); err != nil && !client.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("delete role %q: %w", r.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

func sweepPostureChecks(ctx context.Context, c *client.Client) error {
	checks, err := c.ListAllPostureChecks(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for i := range checks {
		if isSweepable(checks[i].Name) {
			if err := c.DeletePostureCheck(ctx, checks[i].ID); err != nil && !client.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("delete posture check %q: %w", checks[i].Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

func sweepPolicies(ctx context.Context, c *client.Client) error {
	policies, err := c.ListAllPolicies(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for i := range policies {
		if isSweepable(policies[i].Name) {
			if err := c.DeleteThreatDetectionPolicy(ctx, policies[i].ID); err != nil && !client.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("delete policy %q: %w", policies[i].Name, err))
			}
		}
	}
	return errors.Join(errs...)
}
