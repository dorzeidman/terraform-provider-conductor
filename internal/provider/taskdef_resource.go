package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	tfschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var auditableFieldsToIgnore = [4]string{"createTime", "updateTime", "createdBy", "updatedBy"}

var defaultTaskDefValues = map[string]interface{}{
	"backoffScaleFactor":          float64(1),
	"rateLimitFrequencyInSeconds": float64(1),
	"responseTimeoutSeconds":      float64(3600),
	"retryCount":                  float64(3),
	"retryDelaySeconds":           float64(60),
	"retryLogic":                  "FIXED",
	"timeoutPolicy":               "TIME_OUT_WF",
}

var _ tfresource.Resource = &TaskDefResource{}
var _ tfresource.ResourceWithImportState = &TaskDefResource{}
var _ tfresource.ResourceWithModifyPlan = &TaskDefResource{}

type TaskDefResource struct {
	client *conductorHttpClient
}

type TaskDefModel struct {
	Manifest jsontypes.Normalized `tfsdk:"manifest"`
}

func NewTaskDefResource() tfresource.Resource {
	return &TaskDefResource{}
}

func (r *TaskDefResource) Metadata(ctx context.Context, req tfresource.MetadataRequest, resp *tfresource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_taskdef"
}

func (r *TaskDefResource) Schema(ctx context.Context, req tfresource.SchemaRequest, resp *tfresource.SchemaResponse) {
	resp.Schema = tfschema.Schema{
		Description:         "Conductor Task Definition",
		MarkdownDescription: "Conductor Task Definition",
		Attributes: map[string]tfschema.Attribute{
			"manifest": tfschema.StringAttribute{
				Description: "The JSON Manifest for the task definition",
				Required:    true,
				CustomType:  jsontypes.NormalizedType{},
				PlanModifiers: []planmodifier.String{
					nameChangedModifier{},
				},
				Validators: []validator.String{
					manifestNameValidator{},
				},
			},
		},
	}
}

func (r *TaskDefResource) Configure(ctx context.Context, req tfresource.ConfigureRequest, resp *tfresource.ConfigureResponse) {
	if req.ProviderData == nil { // this means the provider.go Configure method hasn't been called yet, so wait longer
		return
	}
	provider, ok := req.ProviderData.(*ConductorProvider)
	if !ok {
		resp.Diagnostics.AddError(
			"Could not create Conductor Provider",
			fmt.Sprintf("Expected *ConductorProvider, got: %T", req.ProviderData),
		)
		return
	}
	r.client = provider.client
}

func (r *TaskDefResource) ModifyPlan(ctx context.Context, req tfresource.ModifyPlanRequest, resp *tfresource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	var plan TaskDefModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Manifest.IsNull() || plan.Manifest.IsUnknown() {
		return
	}

	var state TaskDefModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Manifest.IsNull() || state.Manifest.IsUnknown() {
		return
	}

	var planDef map[string]interface{}
	err := json.Unmarshal([]byte(plan.Manifest.ValueString()), &planDef)
	if err != nil {
		return
	}

	var stateDef map[string]interface{}
	err = json.Unmarshal([]byte(state.Manifest.ValueString()), &stateDef)
	if err != nil {
		return
	}

	cleanupManifestDefaults(ctx, planDef, defaultTaskDefValues)
	cleanupManifestDefaults(ctx, stateDef, defaultTaskDefValues)

	if reflect.DeepEqual(planDef, stateDef) {
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &state)...)
	}
}

func (r *TaskDefResource) Create(ctx context.Context, req tfresource.CreateRequest, resp *tfresource.CreateResponse) {
	var state TaskDefModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var manifestMap map[string]interface{}

	resp.Diagnostics.Append(state.Manifest.Unmarshal(&manifestMap)...)
	if resp.Diagnostics.HasError() {
		return
	}

	//remove fields
	for _, f := range auditableFieldsToIgnore {
		delete(manifestMap, f)
	}

	//des manifestBack
	var requestBody [1]map[string]interface{}
	requestBody[0] = manifestMap

	requestBytes, err := json.Marshal(requestBody)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Manifest", fmt.Sprintf("Manifest Marshal error: %s", err))
		return
	}

	response, err := r.client.do(ctx, http.MethodPost, "metadata/taskdefs", bytes.NewBuffer(requestBytes))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Error sending request: %s", err))
		return
	}
	defer response.Body.Close()

	body, bodyErr := io.ReadAll(response.Body)

	if response.StatusCode != http.StatusOK {
		if bodyErr != nil {
			resp.Diagnostics.AddError("HTTP Error", fmt.Sprintf("Received non-OK HTTP status: %s. Failed to read response body: %s",
				response.Status, bodyErr))
			return
		}

		resp.Diagnostics.AddError("HTTP Error", fmt.Sprintf("Received non-OK HTTP status: %s. Body: %s", response.Status, string(body)))
		return
	}

	if bodyErr != nil {
		resp.Diagnostics.AddError("Status was OK but failed to Read Response Body", fmt.Sprintf("Could not read response body: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TaskDefResource) Read(ctx context.Context, req tfresource.ReadRequest, resp *tfresource.ReadResponse) {
	var state TaskDefModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var stateManifestMap map[string]interface{}

	resp.Diagnostics.Append(state.Manifest.Unmarshal(&stateManifestMap)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stateTaskType := getTaskTypeFromManifest(stateManifestMap, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	path := fmt.Sprintf("metadata/taskdefs/%s", stateTaskType)

	response, err := r.client.do(ctx, http.MethodGet, path, nil)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read task, got error: %s", err))
		return
	}

	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if response.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError(fmt.Sprintf("HTTP Error path: %s", path), fmt.Sprintf("Received bad HTTP status: %s", response.Status))
		return
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		resp.Diagnostics.AddError("Error reading response body", err.Error())
		return
	}

	var currentManifestMap map[string]interface{}

	err = json.Unmarshal(bodyBytes, &currentManifestMap)
	if err != nil {
		resp.Diagnostics.AddError("Manifest JSON Parse error", fmt.Sprintf("Manifest must be a valid json: %s", err))
		return
	}

	for _, f := range auditableFieldsToIgnore {
		delete(currentManifestMap, f)

		if cValue, ok := stateManifestMap[f]; ok {
			currentManifestMap[f] = cValue
		}
	}

	taskDefCleanupAndMerge(ctx, currentManifestMap, stateManifestMap)

	updatedStateBytes, err := json.Marshal(stateManifestMap)
	if err != nil {
		resp.Diagnostics.AddError("Manifest JSON Parse error", fmt.Sprintf("Manifest must be a valid json: %s", err))
		return
	}

	state.Manifest = jsontypes.NewNormalizedValue(string(updatedStateBytes))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TaskDefResource) Delete(ctx context.Context, req tfresource.DeleteRequest, resp *tfresource.DeleteResponse) {
	var state TaskDefModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var manifestMap map[string]interface{}

	resp.Diagnostics.Append(state.Manifest.Unmarshal(&manifestMap)...)
	if resp.Diagnostics.HasError() {
		return
	}

	taskType := getTaskTypeFromManifest(manifestMap, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	path := fmt.Sprintf("metadata/taskdefs/%s", taskType)

	response, err := r.client.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		resp.Diagnostics.AddError("Delete Error", fmt.Sprintf("Unable to delete task def, got error: %s", err))
		return
	}

	defer response.Body.Close()
	alreadyDeleted := false

	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusInternalServerError {
			//check if exists
			internalGetResponse, err := r.client.do(ctx, http.MethodGet, path, nil)
			if err == nil && internalGetResponse.StatusCode == http.StatusNotFound {
				alreadyDeleted = true
			}
			if err == nil {
				defer internalGetResponse.Body.Close()
			}
		}

		if !alreadyDeleted {
			bodyBytes, err := io.ReadAll(response.Body)
			var bodyStr string
			if err == nil {
				bodyStr = string(bodyBytes)
			} else {
				bodyStr = fmt.Sprintf("Read All Body Error: %s", err)
			}

			resp.Diagnostics.AddError("HTTP Error", fmt.Sprintf("Received non-OK HTTP status: %s. Body: %s", response.Status, bodyStr))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TaskDefResource) Update(ctx context.Context, req tfresource.UpdateRequest, resp *tfresource.UpdateResponse) {
	var state TaskDefModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var manifestMap map[string]interface{}
	resp.Diagnostics.Append(state.Manifest.Unmarshal(&manifestMap)...)
	if resp.Diagnostics.HasError() {
		return
	}

	//remove fields
	for _, f := range auditableFieldsToIgnore {
		delete(manifestMap, f)
	}

	putBodyBytes, err := json.Marshal(manifestMap)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Manifest", fmt.Sprintf("Manifest Marshal error: %s", err))
		return
	}

	response, err := r.client.do(ctx, http.MethodPut, "metadata/taskdefs", bytes.NewBuffer(putBodyBytes))

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Error sending request: %s", err))
		return
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			resp.Diagnostics.AddError("Failed to Read Response Body", fmt.Sprintf("Received non-OK HTTP status: %s, Could not read response body: %s", response.Status, err))
			return
		}
		resp.Diagnostics.AddError("HTTP Error", fmt.Sprintf("Received non-OK HTTP status: %s, Body: %s", response.Status, string(body)))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TaskDefResource) ImportState(ctx context.Context, req tfresource.ImportStateRequest, resp *tfresource.ImportStateResponse) {

	initialStateMap := map[string]interface{}{
		"name": req.ID,
	}

	manifestBytes, err := json.Marshal(initialStateMap)
	if err != nil {
		resp.Diagnostics.AddError("Invalid ID", fmt.Sprintf("Manifest Marshal error: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("manifest"), string(manifestBytes))...)
}

func getTaskTypeFromManifest(manifestMap map[string]interface{}, diagnostics *diag.Diagnostics) string {
	taskTypeVal, ok := manifestMap["name"]
	if !ok {
		diagnostics.AddError("Invalid Manifest", "'name' parameter is missing from manifest")
		return ""
	}

	taskType, ok := taskTypeVal.(string)
	if !ok || taskType == "" {
		diagnostics.AddError("Invalid Manifest", "'name' parameter must be string")
		return ""
	}

	return taskType
}

func taskDefCleanupAndMerge(ctx context.Context, currentManifestMap map[string]interface{}, stateManifestMap map[string]interface{}) {
	cleanupManifestDefaults(ctx, currentManifestMap, defaultTaskDefValues)
	mergeManifestMaps(ctx, currentManifestMap, stateManifestMap)
}
