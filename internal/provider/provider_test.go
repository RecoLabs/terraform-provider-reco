package provider_test

import (
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/recolabs/terraform-provider-reco/internal/fakereco"
	"github.com/recolabs/terraform-provider-reco/internal/provider"
)

func factories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"reco": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestProvider_registersAllResourcesAndDataSources(t *testing.T) {
	p := provider.New("test")()

	var resources []string
	for _, f := range p.Resources(t.Context()) {
		var resp resource.MetadataResponse
		f().Metadata(t.Context(), resource.MetadataRequest{ProviderTypeName: "reco"}, &resp)
		resources = append(resources, resp.TypeName)
	}
	slices.Sort(resources)
	wantResources := []string{"reco_api_key", "reco_posture_check", "reco_role", "reco_threat_detection_policy", "reco_user"}
	if !slices.Equal(resources, wantResources) {
		t.Errorf("resources = %v, want %v", resources, wantResources)
	}

	var dataSources []string
	for _, f := range p.DataSources(t.Context()) {
		var resp datasource.MetadataResponse
		f().Metadata(t.Context(), datasource.MetadataRequest{ProviderTypeName: "reco"}, &resp)
		dataSources = append(dataSources, resp.TypeName)
	}
	slices.Sort(dataSources)
	wantDataSources := []string{"reco_api_keys", "reco_integrations", "reco_policies", "reco_posture_checks", "reco_roles", "reco_users"}
	if !slices.Equal(dataSources, wantDataSources) {
		t.Errorf("data sources = %v, want %v", dataSources, wantDataSources)
	}
}

func TestProvider_schemasAreValid(t *testing.T) {
	p := provider.New("test")()

	for _, f := range p.Resources(t.Context()) {
		r := f()
		var meta resource.MetadataResponse
		r.Metadata(t.Context(), resource.MetadataRequest{ProviderTypeName: "reco"}, &meta)
		var resp resource.SchemaResponse
		r.Schema(t.Context(), resource.SchemaRequest{}, &resp)
		if diags := resp.Schema.ValidateImplementation(t.Context()); diags.HasError() {
			t.Errorf("%s schema invalid: %v", meta.TypeName, diags)
		}
	}
	for _, f := range p.DataSources(t.Context()) {
		d := f()
		var meta datasource.MetadataResponse
		d.Metadata(t.Context(), datasource.MetadataRequest{ProviderTypeName: "reco"}, &meta)
		var resp datasource.SchemaResponse
		d.Schema(t.Context(), datasource.SchemaRequest{}, &resp)
		if diags := resp.Schema.ValidateImplementation(t.Context()); diags.HasError() {
			t.Errorf("%s schema invalid: %v", meta.TypeName, diags)
		}
	}
}

const rolesDataSource = `data "reco_roles" "all" {}`

func TestProvider_configureErrors(t *testing.T) {
	srv := fakereco.New(t)

	tests := []struct {
		name      string
		envKey    string
		envURL    string
		config    string
		wantError string
	}{
		{
			name:      "missing api key",
			envURL:    srv.URL,
			config:    `provider "reco" {}`,
			wantError: "Missing API Key",
		},
		{
			name:      "missing base url",
			envKey:    fakereco.APIKey,
			config:    `provider "reco" {}`,
			wantError: "Missing Base URL",
		},
		{
			name:      "plain http to a remote host",
			envKey:    fakereco.APIKey,
			config:    `provider "reco" { base_url = "http://acme.reco.ai" }`,
			wantError: "Insecure Base URL",
		},
		{
			name:      "loopback lookalike host",
			envKey:    fakereco.APIKey,
			config:    `provider "reco" { base_url = "http://localhost.evil.com" }`,
			wantError: "Insecure Base URL",
		},
		{
			name:      "insecure url from the environment",
			envKey:    fakereco.APIKey,
			envURL:    "http://acme.reco.ai",
			config:    `provider "reco" {}`,
			wantError: "Insecure Base URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RECO_API_KEY", tt.envKey)
			t.Setenv("RECO_BASE_URL", tt.envURL)
			tfresource.UnitTest(t, tfresource.TestCase{
				ProtoV6ProviderFactories: factories(),
				Steps: []tfresource.TestStep{{
					Config:      tt.config + "\n" + rolesDataSource,
					ExpectError: regexp.MustCompile(tt.wantError),
				}},
			})
		})
	}
}

func TestProvider_configureFromEnvironment(t *testing.T) {
	srv := fakereco.New(t)
	t.Setenv("RECO_API_KEY", fakereco.APIKey)
	t.Setenv("RECO_BASE_URL", srv.URL)

	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []tfresource.TestStep{{
			Config: `provider "reco" {}` + "\n" + rolesDataSource,
			Check:  tfresource.TestCheckResourceAttr("data.reco_roles.all", "roles.#", "2"),
		}},
	})
}

func TestProvider_attributesOverrideEnvironment(t *testing.T) {
	srv := fakereco.New(t)
	t.Setenv("RECO_API_KEY", "wrong-key-from-env")
	t.Setenv("RECO_BASE_URL", "https://wrong.invalid")

	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []tfresource.TestStep{{
			Config: fmt.Sprintf(`provider "reco" {
  api_key  = %q
  base_url = %q
}
%s`, fakereco.APIKey, srv.URL, rolesDataSource),
			Check: tfresource.TestCheckResourceAttr("data.reco_roles.all", "roles.#", "2"),
		}},
	})
}

func TestProvider_wrongAPIKeyIsReportedByTheAPI(t *testing.T) {
	srv := fakereco.New(t)
	t.Setenv("RECO_API_KEY", "")
	t.Setenv("RECO_BASE_URL", "")

	tfresource.UnitTest(t, tfresource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []tfresource.TestStep{{
			Config: fmt.Sprintf(`provider "reco" {
  api_key  = "not-the-key"
  base_url = %q
}
%s`, srv.URL, rolesDataSource),
			ExpectError: regexp.MustCompile(`HTTP 401`),
		}},
	})
}
