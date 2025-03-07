package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

type conductorPartialDef struct {
	Name string `json:"name"`
}

type nameChangedModifier struct{}

func (m nameChangedModifier) Description(_ context.Context) string {
	return "If 'name' property is changed > RequiresReplace = true"
}

func (m nameChangedModifier) MarkdownDescription(c context.Context) string {
	return m.Description(c)
}

func (m nameChangedModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Do nothing if there is no state value.
	if req.StateValue.IsNull() || req.PlanValue.IsNull() {
		return
	}

	var stateDef conductorPartialDef
	err := json.Unmarshal([]byte(req.StateValue.ValueString()), &stateDef)
	if err != nil {
		return
	}

	var planDef conductorPartialDef
	err = json.Unmarshal([]byte(req.PlanValue.ValueString()), &planDef)
	if err != nil {
		return
	}

	if stateDef.Name != planDef.Name {
		resp.RequiresReplace = true
	}
}
