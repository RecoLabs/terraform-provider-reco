# Terraform Provider for Reco

Manage [Reco](https://www.reco.ai) configuration as code: users, roles, API keys, posture checks, and threat detection policies.

Documentation is published on the [Terraform Registry](https://registry.terraform.io/providers/recolabs/reco/latest/docs).

## Requirements

- Terraform 1.8+ or OpenTofu 1.7+
- A Reco API key, created in the Reco platform under Settings → API Keys

## Usage

```hcl
terraform {
  required_providers {
    reco = {
      source  = "recolabs/reco"
      version = "~> 0.1"
    }
  }
}

provider "reco" {
  api_key  = var.reco_api_key
  base_url = var.reco_base_url
}

resource "reco_role" "soc_analyst" {
  name        = "SOC Analyst"
  permissions = ["PERM_ALERTS_READ", "PERM_EVENT_READ"]
}

resource "reco_user" "alice" {
  email_address = "alice@example.com"
  name          = "Alice Smith"
  user_roles    = [reco_role.soc_analyst.name]
}
```

`api_key` and `base_url` can also be supplied through the `RECO_API_KEY` and `RECO_BASE_URL` environment variables. `base_url` is the URL you use to sign in to the Reco platform and must use `https`.

## Resources

| Resource | Description |
|---|---|
| `reco_user` | Reco platform user |
| `reco_role` | Custom RBAC role |
| `reco_api_key` | API key |
| `reco_posture_check` | Posture check policy |
| `reco_threat_detection_policy` | Behavioral threat detection policy |

## Data Sources

| Data Source | Description |
|---|---|
| `reco_users` | All users |
| `reco_roles` | All roles |
| `reco_api_keys` | All API keys (without secrets) |
| `reco_policies` | All threat detection policies |
| `reco_posture_checks` | All posture checks |
| `reco_integrations` | All connected integrations |

## Development

Requires Go (see `go.mod`).

```bash
make build     # build the provider binary
make test      # unit tests
make testacc   # acceptance tests against a live tenant (needs RECO_API_KEY and RECO_BASE_URL)
make generate  # regenerate docs/
make lint
```

To run a local build with Terraform, point a `dev_overrides` block at your `GOBIN`:

```hcl
provider_installation {
  dev_overrides {
    "recolabs/reco" = "/path/to/go/bin"
  }
  direct {}
}
```

## Security

See [SECURITY.md](SECURITY.md) to report a vulnerability.

## License

[MPL-2.0](LICENSE)
