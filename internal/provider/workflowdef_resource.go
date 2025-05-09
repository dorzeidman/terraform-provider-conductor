package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
		Description: "Conductor Workflow Definition",
		MarkdownDescription: `
Conductor Workflow Definition
## Versioning
Workflow definition has a "version" field for supporting of keep old version / execution specific version.
On delete all the workflow definition versions will be deleted.
The provider support two types of versions modes.
### Auto Version Mode
If you remove the "version" field from the manifest, then on creation the version will be equal to 1. Every update will increment the version by 1.
### Manual Version Mode
If the manifest has a "version" field, it will be used as part of creation and updating. updates will fail if the version will be decreased.
		`,
		Attributes: map[string]tfschema.Attribute{
			"manifest": tfschema.StringAttribute{
				Description: "The JSON Manifest for the workflow definition",
				Required:    true,
				CustomType:  jsontypes.NormalizedType{},
				PlanModifiers: []planmodifier.String{
					nameChangedModifier{},
				},
				Validators: []validator.String{
					manifestNameValidator{},
				},
			},
			"version": tfschema.Int32Attribute{
				Computed: true,
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

	createVersion, shoudCreate := checkExistingVersionBeforeCreate(ctx, r.client, manifestMap, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if shoudCreate {
		//remove fields
		for _, f := range auditableFieldsToIgnore {
			delete(manifestMap, f)
		}
		manifestMap["version"] = createVersion

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
	}

	state.Version = tftypes.Int32Value(createVersion)

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

	_, stateVersionExists, err := getWorkflowVersionOptionalFromManifest(stateManifestMap)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get version from manifest plan", fmt.Sprintf("Get Version error: %s", err))
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

	version, err := getWorkflowVersionFromManifest(currentManifestMap)

	if err != nil {
		resp.Diagnostics.AddError("Unexpected Error. failed ot extract version from current manifest", err.Error())
		return
	}

	for _, f := range auditableFieldsToIgnore {
		delete(currentManifestMap, f)

		if cValue, ok := stateManifestMap[f]; ok {
			currentManifestMap[f] = cValue
		}
	}

	if !stateVersionExists {
		delete(currentManifestMap, "version")
	}

	workflowDefCleanupAndMerge(ctx, currentManifestMap, stateManifestMap)

	updatedStateBytes, err := json.Marshal(stateManifestMap)
	if err != nil {
		resp.Diagnostics.AddError("Manifest JSON Parse error", fmt.Sprintf("Manifest must be a valid json: %s", err))
		return
	}

	state.Version = tftypes.Int32Value(version)
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

	var currentVersion int32

	for {
		nextVersion, versionExists := getLatestVersion(ctx, r.client, name, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}

		if !versionExists {
			break
		}

		if currentVersion > 0 && currentVersion == nextVersion {
			resp.Diagnostics.AddError("Delete failed, try to delete the same version twice", "")
			return
		}
		currentVersion = nextVersion

		path := fmt.Sprintf("metadata/workflow/%s/%d", name, currentVersion)

		response, err := r.client.do(ctx, http.MethodDelete, path, nil)

		if err != nil {
			resp.Diagnostics.AddError("Delete Error", fmt.Sprintf("Unable to delete task def, got error: %s", err))
			return
		}

		defer response.Body.Close()

		if response.StatusCode != http.StatusOK {

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

func (r *WorkflowDefResource) Update(ctx context.Context, req tfresource.UpdateRequest, resp *tfresource.UpdateResponse) {
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

	planVersion, planVersionExists, err := getWorkflowVersionOptionalFromManifest(manifestMap)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get version from manifest plan", fmt.Sprintf("Get Version error: %s", err))
		return
	}

	var newVersion int32
	if planVersionExists {
		verifyValidVersionForUpdate(ctx, r.client, manifestMap, planVersion, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}

		newVersion = planVersion
	} else {

		var stateVersion tftypes.Int32
		req.State.GetAttribute(ctx, path.Root("version"), &stateVersion)

		newVersion = stateVersion.ValueInt32() + 1
		manifestMap["version"] = newVersion
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

func getWorkflowVersionOptionalFromManifest(manifestMap map[string]interface{}) (int32, bool, error) {
	versionVal, ok := manifestMap["version"]
	if !ok {
		return 0, false, nil
	}

	versionFloat64, ok := versionVal.(float64)
	if !ok {
		return 0, false, fmt.Errorf("'version' parameter must be int")
	}

	if versionFloat64 != math.Floor(versionFloat64) {
		return 0, false, fmt.Errorf("'version' parameter must be int")
	}

	versionInt := int32(versionFloat64)
	if versionInt < 1 {
		return 0, false, fmt.Errorf("'version' is smaller than 1")
	}

	return versionInt, true, nil
}

func getWorkflowVersionFromManifest(manifestMap map[string]interface{}) (int32, error) {
	version, versionExists, err := getWorkflowVersionOptionalFromManifest(manifestMap)
	if err != nil {
		return 0, err
	}

	if !versionExists {
		return 0, fmt.Errorf("version not found")
	}

	return version, nil
}

func workflowDefCleanupAndMerge(ctx context.Context, currentManifestMap map[string]interface{}, stateManifestMap map[string]interface{}) {
	//1. Cleanup current
	workflowDefCleanup(ctx, currentManifestMap)

	//2. Copy Exist
	workflowDefMerge(ctx, currentManifestMap, stateManifestMap)
}

func workflowDefCleanup(ctx context.Context, currentManifestMap map[string]interface{}) {
	for _, f := range auditableFieldsToIgnore {
		delete(currentManifestMap, f)
	}

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

func checkExistingVersionBeforeCreate(ctx context.Context, client *conductorHttpClient, planMap map[string]interface{}, diagnostics *diag.Diagnostics) (int32, bool) {
	name := getWorkflowNameFromManifest(planMap, diagnostics)
	if diagnostics.HasError() {
		return 0, false
	}

	version, versionExists, err := getWorkflowVersionOptionalFromManifest(planMap)
	tflog.Debug(ctx, fmt.Sprintf("Version Found: %t, Version: %d", versionExists, version))
	if err != nil {
		diagnostics.AddError(fmt.Sprintf("Manifest get version err: %s", err), "")
		return 0, false
	}

	latestPath := fmt.Sprintf("metadata/workflow/%s", name)

	response, err := client.do(ctx, http.MethodGet, latestPath, nil)

	if err != nil {
		diagnostics.AddError("Failed to get Manifest", fmt.Sprintf("Manifest get err: %s", err))
		return 0, false
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		if versionExists {
			return version, true
		}
		return 1, true
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		diagnostics.AddError("Error reading response body", fmt.Sprintf("Status Code: %s, Error: %s", response.Status, err))
		return 0, false
	}

	if response.StatusCode != http.StatusOK {
		diagnostics.AddError("HTTP Get Error", fmt.Sprintf("Received bad HTTP status: %s. Body: %s", response.Status, string(bodyBytes)))
		return 0, false
	}

	var currentManifestMap map[string]interface{}

	err = json.Unmarshal(bodyBytes, &currentManifestMap)
	if err != nil {
		diagnostics.AddError("Current Manifest JSON Parse error", fmt.Sprintf("Manifest must be a valid json: %s", err))
		return 0, false
	}

	currentVersion, err := getWorkflowVersionFromManifest(currentManifestMap)
	if err != nil {
		diagnostics.AddError("Invalid Current Manifest", fmt.Sprintf("Manifest get version err: %s", err))
		return 0, false
	}

	if versionExists {
		if version < currentVersion {
			diagnostics.AddError("Found an existing workflow definition with a larger version", "")
			return 0, false
		}

		return version, true
	}

	//Auto Version
	workflowDefCleanup(ctx, planMap)
	delete(currentManifestMap, "version")
	workflowDefCleanup(ctx, currentManifestMap)

	if reflect.DeepEqual(planMap, currentManifestMap) {
		tflog.Debug(ctx, "Will not create workflow def because it already exists with the same manifest + version")
		//Not changes found, so do nothing
		return currentVersion, false
	}

	return currentVersion + 1, true
}

func verifyValidVersionForUpdate(ctx context.Context, client *conductorHttpClient, planMap map[string]interface{}, planVersion int32, diagnostics *diag.Diagnostics) {
	name := getWorkflowNameFromManifest(planMap, diagnostics)
	if diagnostics.HasError() {
		return
	}

	latestPath := fmt.Sprintf("metadata/workflow/%s", name)

	response, err := client.do(ctx, http.MethodGet, latestPath, nil)
	if err != nil {
		diagnostics.AddError("Failed to get Manifest", fmt.Sprintf("Manifest get err: %s", err))
	}

	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		diagnostics.AddError("Error reading response body", fmt.Sprintf("Status Code: %s, Error: %s", response.Status, err))
	}

	if response.StatusCode != http.StatusOK {
		diagnostics.AddError("HTTP Get Error", fmt.Sprintf("Received bad HTTP status: %s. Body: %s", response.Status, string(bodyBytes)))
		return
	}

	var currentManifestMap map[string]interface{}

	err = json.Unmarshal(bodyBytes, &currentManifestMap)
	if err != nil {
		diagnostics.AddError("Current Manifest JSON Parse error", fmt.Sprintf("Manifest must be a valid json: %s", err))
		return
	}

	currentVersion, err := getWorkflowVersionFromManifest(currentManifestMap)
	if err != nil {
		diagnostics.AddError("Invalid Current Manifest", fmt.Sprintf("Manifest get version err: %s", err))
		return
	}

	if planVersion < currentVersion {
		diagnostics.AddError("Found an existing workflow definition with a larger version", "")
	}
}

func getLatestVersion(ctx context.Context, client *conductorHttpClient, name string, diagnostics *diag.Diagnostics) (int32, bool) {
	latestPath := fmt.Sprintf("metadata/workflow/%s", name)

	response, err := client.do(ctx, http.MethodGet, latestPath, nil)
	if err != nil {
		diagnostics.AddError("Failed to get Manifest", fmt.Sprintf("Manifest get err: %s", err))
		return 0, false
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return 0, false
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		diagnostics.AddError("Error reading response body", fmt.Sprintf("Status Code: %s, Error: %s", response.Status, err))
		return 0, false
	}

	if response.StatusCode != http.StatusOK {
		diagnostics.AddError("HTTP Get Error", fmt.Sprintf("Received bad HTTP status: %s. Body: %s", response.Status, string(bodyBytes)))
		return 0, false
	}

	var currentManifestMap map[string]interface{}

	err = json.Unmarshal(bodyBytes, &currentManifestMap)
	if err != nil {
		diagnostics.AddError("Current Manifest JSON Parse error", fmt.Sprintf("Manifest must be a valid json: %s", err))
		return 0, false
	}

	currentVersion, err := getWorkflowVersionFromManifest(currentManifestMap)
	if err != nil {
		diagnostics.AddError("Invalid Current Manifest", fmt.Sprintf("Manifest get version err: %s", err))
		return 0, false
	}

	return currentVersion, true
}
