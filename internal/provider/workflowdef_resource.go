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
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ tfresource.Resource = &WorkflowDefResource{}
var _ tfresource.ResourceWithImportState = &WorkflowDefResource{}
var _ tfresource.ResourceWithModifyPlan = &WorkflowDefResource{}

type WorkflowDefResource struct {
	client *conductorHttpClient
}

type WorkflowDefModel struct {
	Manifest jsontypes.Normalized `tfsdk:"manifest"`
	Version  tftypes.Int32        `tfsdk:"version"`
}

var defaultWorkflowDefValues = map[string]interface{}{
	"schemaVersion": float64(2),
	"timeoutPolicy": "ALERT_ONLY",
	"enforceSchema": true,
	"restartable":   true,
}

var defaultWorkflowDefTaskValues = map[string]interface{}{
	"type": "SIMPLE",
}

func NewWorkflowDefResource() tfresource.Resource {
	return &WorkflowDefResource{}
}

func (r *WorkflowDefResource) Metadata(ctx context.Context, req tfresource.MetadataRequest, resp *tfresource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflowdef"
}

func (r *WorkflowDefResource) Schema(ctx context.Context, req tfresource.SchemaRequest, resp *tfresource.SchemaResponse) {
	resp.Schema = tfschema.Schema{
		Description:         "Conductor Workflow Definition",
		MarkdownDescription: "Conductor Workflow Definition",
		Attributes: map[string]tfschema.Attribute{
			"manifest": tfschema.StringAttribute{
				Description: "The JSON Manifest for the workflow definition",
				Required:    true,
				CustomType:  jsontypes.NormalizedType{},
				PlanModifiers: []planmodifier.String{
					nameChangedModifier{},
					//workflowdDefNotChangedModifier{},
				},
				Validators: []validator.String{
					manifestNameValidator{},
				},
			},
			"version": tfschema.Int32Attribute{
				Computed: true,
				// PlanModifiers: []planmodifier.Int32{
				// 	workflowdDefVersionModifier{},
				// },
			},
		},
	}
}

func (r *WorkflowDefResource) Configure(ctx context.Context, req tfresource.ConfigureRequest, resp *tfresource.ConfigureResponse) {
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

func (r *WorkflowDefResource) ModifyPlan(ctx context.Context, req tfresource.ModifyPlanRequest, resp *tfresource.ModifyPlanResponse) {

	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	var plan WorkflowDefModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Manifest.IsNull() || plan.Manifest.IsUnknown() {
		return
	}

	var state WorkflowDefModel
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

	workflowDefCleanup(ctx, planDef)
	workflowDefCleanup(ctx, stateDef)

	if reflect.DeepEqual(planDef, stateDef) {
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &state)...)
	}
}

func (r *WorkflowDefResource) Create(ctx context.Context, req tfresource.CreateRequest, resp *tfresource.CreateResponse) {
	var state WorkflowDefModel

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

	var requestBody [1]map[string]interface{}
	requestBody[0] = manifestMap

	requestBytes, err := json.Marshal(requestBody)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Manifest", fmt.Sprintf("Manifest Marshal error: %s", err))
		return
	}

	response, err := r.client.do(ctx, http.MethodPut, "metadata/workflow", bytes.NewBuffer(requestBytes))

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

	state.Version = tftypes.Int32Value(1)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WorkflowDefResource) Read(ctx context.Context, req tfresource.ReadRequest, resp *tfresource.ReadResponse) {
	var state WorkflowDefModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var stateManifestMap map[string]interface{}

	resp.Diagnostics.Append(state.Manifest.Unmarshal(&stateManifestMap)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := getWorkflowNameFromManifest(stateManifestMap, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	response, err := r.client.do(ctx, http.MethodGet, fmt.Sprintf("metadata/workflow/%s", name), nil)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read workflow, got error: %s", err))
		return
	}

	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}

	if response.StatusCode != http.StatusOK {
		resp.Diagnostics.AddError("HTTP Get Error", fmt.Sprintf("Received bad HTTP status: %s", response.Status))
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

	version, versionIsFloat64 := currentManifestMap["version"].(float64)
	if !versionIsFloat64 {
		resp.Diagnostics.AddError("Unexpected Error. version isn't float64", "")
		return
	}

	for _, f := range auditableFieldsToIgnore {
		delete(currentManifestMap, f)

		if cValue, ok := stateManifestMap[f]; ok {
			currentManifestMap[f] = cValue
		}
	}

	workflowDefCleanupAndMerge(ctx, currentManifestMap, stateManifestMap)

	updatedStateBytes, err := json.Marshal(stateManifestMap)
	if err != nil {
		resp.Diagnostics.AddError("Manifest JSON Parse error", fmt.Sprintf("Manifest must be a valid json: %s", err))
		return
	}

	state.Version = tftypes.Int32Value(int32(version))
	state.Manifest = jsontypes.NewNormalizedValue(string(updatedStateBytes))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WorkflowDefResource) Delete(ctx context.Context, req tfresource.DeleteRequest, resp *tfresource.DeleteResponse) {
	var state WorkflowDefModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var manifestMap map[string]interface{}

	resp.Diagnostics.Append(state.Manifest.Unmarshal(&manifestMap)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := getWorkflowNameFromManifest(manifestMap, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	version := state.Version.ValueInt32()

	for version > 0 {

		path := fmt.Sprintf("metadata/workflow/%s/%d", name, version)

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
				response, err := r.client.do(ctx, http.MethodGet, path, nil)
				if err == nil && response.StatusCode == http.StatusNotFound {
					alreadyDeleted = true
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
		version--
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WorkflowDefResource) Update(ctx context.Context, req tfresource.UpdateRequest, resp *tfresource.UpdateResponse) {
	var state WorkflowDefModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var stateVersion tftypes.Int32
	req.State.GetAttribute(ctx, path.Root("version"), &stateVersion)

	var manifestMap map[string]interface{}
	resp.Diagnostics.Append(state.Manifest.Unmarshal(&manifestMap)...)
	if resp.Diagnostics.HasError() {
		return
	}

	//remove fields
	for _, f := range auditableFieldsToIgnore {
		delete(manifestMap, f)
	}

	newVersion := stateVersion.ValueInt32() + 1

	manifestMap["version"] = newVersion

	var requestBody [1]map[string]interface{}
	requestBody[0] = manifestMap

	requestBytes, err := json.Marshal(requestBody)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Manifest", fmt.Sprintf("Manifest Marshal error: %s", err))
		return
	}

	response, err := r.client.do(ctx, http.MethodPut, "metadata/workflow", bytes.NewBuffer(requestBytes))

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

	state.Version = tftypes.Int32Value(newVersion)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WorkflowDefResource) ImportState(ctx context.Context, req tfresource.ImportStateRequest, resp *tfresource.ImportStateResponse) {

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

func getWorkflowNameFromManifest(manifestMap map[string]interface{}, diagnostics *diag.Diagnostics) string {
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

func workflowDefCleanupAndMerge(ctx context.Context, currentManifestMap map[string]interface{}, stateManifestMap map[string]interface{}) {
	//1. Cleanup current
	workflowDefCleanup(ctx, currentManifestMap)

	//2. Copy Exist
	workflowDefMerge(ctx, currentManifestMap, stateManifestMap)
}

func workflowDefCleanup(ctx context.Context, currentManifestMap map[string]interface{}) {
	cleanupManifestDefaults(ctx, currentManifestMap, defaultWorkflowDefValues)

	//tasks
	currentTasksVal, ok := currentManifestMap["tasks"]
	if !ok {
		tflog.Error(ctx, "current map 'tasks' key not found")
		return
	}

	currentTasksArr, ok := currentTasksVal.([]interface{})
	if !ok {
		tflog.Error(ctx, fmt.Sprintf("current map 'tasks' key is not valid a slice. type: %T", currentTasksVal))
		return
	}

	for i := 0; i < len(currentTasksArr); i++ {

		currentTask, ok := currentTasksArr[i].(map[string]interface{})
		if !ok {
			tflog.Error(ctx, fmt.Sprintf("current map 'task' index: %d, is not valid a map. type: %T", i, currentTasksArr[i]))
			continue
		}

		cleanupManifestDefaults(ctx, currentTask, defaultWorkflowDefTaskValues)
	}
}

func workflowDefMerge(ctx context.Context, currentManifestMap map[string]interface{}, stateManifestMap map[string]interface{}) {
	mergeManifestMaps(ctx, currentManifestMap, stateManifestMap)

	currentTasksVal, ok := currentManifestMap["tasks"]
	if !ok {
		tflog.Error(ctx, "current map 'tasks' key not found")
		return
	}

	currentTasksArr, ok := currentTasksVal.([]interface{})
	if !ok {
		tflog.Error(ctx, fmt.Sprintf("current map 'tasks' key is not valid a slice. type: %T", currentTasksVal))
		return
	}

	stateTasksVal, ok := stateManifestMap["tasks"]
	if !ok {
		tflog.Error(ctx, "state map 'tasks' key not found")
		return
	}

	stateTasksArr, ok := stateTasksVal.([]interface{})
	if !ok {
		tflog.Error(ctx, fmt.Sprintf("state map 'tasks' key is not valid a slice. type: %T", stateTasksVal))
		return
	}

	for i := 0; i < len(currentTasksArr); i++ {
		if i >= len(stateTasksArr) {
			break
		}

		currentTask, ok := currentTasksArr[i].(map[string]interface{})
		if !ok {
			tflog.Error(ctx, fmt.Sprintf("current map 'task' index: %d, is not valid a map. type: %T", i, currentTasksArr[i]))
			continue
		}

		stateTask, ok := stateTasksArr[i].(map[string]interface{})
		if !ok {
			tflog.Error(ctx, fmt.Sprintf("state map 'task' index: %d, is not valid a map. type: %T", i, stateTasksArr[i]))
			continue
		}

		mergeManifestMaps(ctx, currentTask, stateTask)
	}
}
