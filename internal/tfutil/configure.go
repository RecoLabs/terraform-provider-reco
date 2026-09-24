package tfutil

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/recolabs/terraform-provider-reco/internal/client"
)

func SetClient(providerData any, target **client.Client, diags *diag.Diagnostics) bool {
	if providerData == nil {
		return false
	}
	c, ok := providerData.(*client.Client)
	if !ok {
		diags.AddError("Unexpected Provider Data",
			fmt.Sprintf("expected *client.Client, got %T", providerData))
		return false
	}
	*target = c
	return true
}
