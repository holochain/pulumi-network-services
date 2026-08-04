// Package inputs resolves optional Pulumi Inputs to Outputs, substituting a
// default when the caller left the field unset.
//
// A nil Input means "not set". These helpers exist so every component treats that
// the same way, rather than each one re-deciding what an omitted argument means.
package inputs

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

// StringOr returns in, or fallback when in is unset.
func StringOr(in pulumi.StringInput, fallback string) pulumi.StringOutput {
	if in == nil {
		return pulumi.String(fallback).ToStringOutput()
	}
	return in.ToStringOutput()
}

// IntOr returns in, or fallback when in is unset.
func IntOr(in pulumi.IntInput, fallback int) pulumi.IntOutput {
	if in == nil {
		return pulumi.Int(fallback).ToIntOutput()
	}
	return in.ToIntOutput()
}

// BoolPtrOr returns in, or fallback when in is unset.
//
// The result is a pointer output because the DigitalOcean SDK models optional
// booleans that way, and distinguishing "false" from "unset" matters there.
func BoolPtrOr(in pulumi.BoolInput, fallback bool) pulumi.BoolPtrOutput {
	if in == nil {
		return pulumi.Bool(fallback).ToBoolPtrOutput()
	}
	return in.ToBoolOutput().ToBoolPtrOutput()
}

// StringArrayOrEmpty returns in, or an empty array when in is unset.
//
// Empty rather than nil, so a component never sends "no opinion" where the
// provider would read it as "leave whatever is there".
func StringArrayOrEmpty(in pulumi.StringArrayInput) pulumi.StringArrayOutput {
	if in == nil {
		return pulumi.StringArray{}.ToStringArrayOutput()
	}
	return in.ToStringArrayOutput()
}
