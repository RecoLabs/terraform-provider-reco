package tfutil

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func StringPtrValue(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func StringSliceToList(s []string) types.List {
	if s == nil {
		s = []string{}
	}
	elems := make([]attr.Value, len(s))
	for i, v := range s {
		elems[i] = types.StringValue(v)
	}
	return types.ListValueMust(types.StringType, elems)
}

func ListToStringSlice(l types.List) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	elems := l.Elements()
	out := make([]string, 0, len(elems))
	for _, e := range elems {
		if s, ok := e.(types.String); ok {
			out = append(out, s.ValueString())
		}
	}
	return out
}

func StringSliceToSet(s []string) types.Set {
	if s == nil {
		s = []string{}
	}
	elems := make([]attr.Value, len(s))
	for i, v := range s {
		elems[i] = types.StringValue(v)
	}
	return types.SetValueMust(types.StringType, elems)
}

func SetToStringSlice(s types.Set) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	elems := s.Elements()
	out := make([]string, 0, len(elems))
	for _, e := range elems {
		if str, ok := e.(types.String); ok {
			out = append(out, str.ValueString())
		}
	}
	return out
}
