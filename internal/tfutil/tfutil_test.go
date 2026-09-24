package tfutil_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/recolabs/terraform-provider-reco/internal/client"
	"github.com/recolabs/terraform-provider-reco/internal/tfutil"
)

func TestStringPtrValue(t *testing.T) {
	s := "hello"
	if got := tfutil.StringPtrValue(&s); got.ValueString() != "hello" {
		t.Fatalf("expected hello, got %q", got.ValueString())
	}
	if got := tfutil.StringPtrValue(nil); !got.IsNull() {
		t.Fatalf("expected null, got %q", got.ValueString())
	}
}

func TestStringSliceToList_roundtrip(t *testing.T) {
	in := []string{"a", "b", "c"}
	l := tfutil.StringSliceToList(in)
	out := tfutil.ListToStringSlice(l)
	if len(out) != len(in) {
		t.Fatalf("length mismatch: got %d, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("element %d: got %q, want %q", i, out[i], in[i])
		}
	}
}

func TestStringSliceToList_nil(t *testing.T) {
	l := tfutil.StringSliceToList(nil)
	if l.IsNull() || l.IsUnknown() {
		t.Fatal("expected empty known list, got null/unknown")
	}
	if len(l.Elements()) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(l.Elements()))
	}
}

func TestListToStringSlice_null(t *testing.T) {
	if got := tfutil.ListToStringSlice(types.ListNull(types.StringType)); got != nil {
		t.Fatalf("expected nil for null list, got %v", got)
	}
}

func TestSetClient_nil(t *testing.T) {
	var c *client.Client
	var diags diag.Diagnostics
	if tfutil.SetClient(nil, &c, &diags) {
		t.Fatal("expected false for nil providerData")
	}
	if diags.HasError() {
		t.Fatal("expected no error for nil providerData")
	}
}

func TestSetClient_wrong_type(t *testing.T) {
	var c *client.Client
	var diags diag.Diagnostics
	if tfutil.SetClient("not-a-client", &c, &diags) {
		t.Fatal("expected false for wrong type")
	}
	if !diags.HasError() {
		t.Fatal("expected error diagnostic for wrong type")
	}
}

func TestSetClient_correct_type(t *testing.T) {
	expected := client.New("https://example.com", "key", "test")
	var c *client.Client
	var diags diag.Diagnostics
	if !tfutil.SetClient(expected, &c, &diags) {
		t.Fatal("expected true for correct type")
	}
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if c != expected {
		t.Fatal("client pointer mismatch")
	}
}
