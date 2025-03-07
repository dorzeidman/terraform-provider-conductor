package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type manifestNameValidator struct{}

func (m manifestNameValidator) Description(_ context.Context) string {
	return "Cleanup non-required fields from manifest"
}

func (m manifestNameValidator) MarkdownDescription(c context.Context) string {
	return m.Description(c)
}

func (m manifestNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() {
		return
	}

	var planMap map[string]interface{}
	err := json.Unmarshal([]byte(req.ConfigValue.ValueString()), &planMap)
	if err != nil {
		return
	}

	nameVal, ok := planMap["name"]
	if !ok {
		resp.Diagnostics.AddAttributeError(req.Path, "'name' parameter is missing from manifest", "")
		return
	}

	nameStr, ok := nameVal.(string)
	if !ok || nameStr == "" {
		resp.Diagnostics.AddAttributeError(req.Path, "'name' parameter must be a a non empty string", "")
		return
	}
}
