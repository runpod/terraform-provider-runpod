package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// The GraphQL reads and the one write behind the deprecated pod, template and
// endpoint attributes, and the billing values, that REST v2 does not report.
// Every operation, argument and field name below is declared in the GraphQL
// schema: Query.pod, Query.podTemplate, Query.myself, User.endpoint,
// User.billing, Mutation.saveTemplate, and the PodFilter, Pod, PodTemplate,
// Endpoint, SaveTemplateInput, UserBillingInput, UserBilling,
// UserServerlessBilling and UserGpuCloudBilling types.
//
// A null object in an answer is "no data for this object", not an error:
// Query.podTemplate answers null for a template the caller cannot see, and
// the attributes it feeds go null the same way.

// legacyPodFieldsQuery reads the two pod fields REST v2 has no counterpart
// for. Both are non-null on Pod, so a pod the query answers for always
// reports them.
const legacyPodFieldsQuery = `query PodLegacyFields($podId: String!) {
  pod(input: { podId: $podId }) { machineId volumeEncrypted }
}`

// legacyTemplateFieldsQuery reads the four template fields REST v2 has no
// counterpart for. earned and runtimeInMin resolve to 0, not null, for a
// template the caller does not own.
const legacyTemplateFieldsQuery = `query TemplateLegacyFields($id: String!) {
  podTemplate(id: $id) { readme earned isRunpod runtimeInMin }
}`

// legacyEndpointOwnerQuery reads the endpoint owner the 1.x users attribute
// carried.
const legacyEndpointOwnerQuery = `query EndpointOwner($id: String!) {
  myself { endpoint(id: $id) { userId } }
}`

// legacyServerlessBilledTimeQuery reads the billed time of every Serverless
// endpoint of the account, one row per endpoint and time bucket.
// UserServerlessBilling.timeBilledSeconds is the bucket's billed time in whole
// seconds, and time is the start of the bucket the granularity sets.
const legacyServerlessBilledTimeQuery = `query ServerlessBilledTime($input: UserBillingInput!) {
  myself { billing(input: $input) { serverless(input: { groupBy: ENDPOINT }) { endpointId time timeBilledSeconds } } }
}`

// legacyGPUCloudBillingQuery reads GPU pod spend and billed time per GPU type
// and time bucket (UserBilling.gpuCloud, UserGpuCloudBilling). It carries no
// pod id.
const legacyGPUCloudBillingQuery = `query GPUCloudBilling($input: UserBillingInput!) {
  myself { billing(input: $input) { gpuCloud { gpuTypeId time timeBilledSeconds amount } } }
}`

// legacyTemplateForSaveQuery reads every field saveTemplate rewrites when it
// is absent from the input, so a readme-only write can send the template back
// unchanged. imageName is not read: when it is sent the resolver re-verifies
// the image against its registry, and when it is absent it keeps the stored
// one.
const legacyTemplateForSaveQuery = `query TemplateForSave($id: String!) {
  podTemplate(id: $id) {
    id name containerDiskInGb volumeInGb volumeMountPath dockerArgs env { key value }
    ports portsConfig { port name } startJupyter startSsh startScript advancedStart
    isPublic isServerless category config containerRegistryAuthId
    recommendedGPUIds incompatibleGPUIds compatibleGPUIds minVram minRam
    allowedCudaVersions networkVolumeIds
  }
}`

// legacySaveTemplateReadmeMutation writes the readme and reads back the four
// fields legacyTemplateFieldsQuery reads.
const legacySaveTemplateReadmeMutation = `mutation SaveTemplateReadme($input: SaveTemplateInput) {
  saveTemplate(input: $input) { readme earned isRunpod runtimeInMin }
}`

// legacyPodFields is what legacyPodFieldsQuery answers for one pod.
type legacyPodFields struct {
	MachineID       *string `json:"machineId"`
	VolumeEncrypted *bool   `json:"volumeEncrypted"`
}

// legacyTemplateFields is what legacyTemplateFieldsQuery and
// legacySaveTemplateReadmeMutation answer for one template.
type legacyTemplateFields struct {
	Readme       *string  `json:"readme"`
	Earned       *float64 `json:"earned"`
	IsRunpod     *bool    `json:"isRunpod"`
	RuntimeInMin *int64   `json:"runtimeInMin"`
}

// legacyServerlessBilledTime is one row legacyServerlessBilledTimeQuery
// answers.
type legacyServerlessBilledTime struct {
	EndpointID        *string    `json:"endpointId"`
	Time              *time.Time `json:"time"`
	TimeBilledSeconds *int64     `json:"timeBilledSeconds"`
}

// legacyGPUCloudBilling is one row legacyGPUCloudBillingQuery answers.
type legacyGPUCloudBilling struct {
	GpuTypeID         *string    `json:"gpuTypeId"`
	Time              *time.Time `json:"time"`
	TimeBilledSeconds *int64     `json:"timeBilledSeconds"`
	Amount            *float64   `json:"amount"`
}

// errLegacyBillingWithheld is a billing read the GraphQL API answered with a
// null User.billing: the caller may not read the account's billing.
var errLegacyBillingWithheld = errors.New("the GraphQL API does not report this account's billing to the caller")

// errLegacyTemplateNotVisible is a readme write against a template the
// GraphQL host does not answer for.
var errLegacyTemplateNotVisible = errors.New("the GraphQL API does not report this template to the caller")

// fetchLegacyPodFields returns nil without an error when the GraphQL API has
// no pod by that id.
func fetchLegacyPodFields(ctx context.Context, client *legacy.Client, podID string) (*legacyPodFields, error) {
	data, err := client.Query(ctx, legacyPodFieldsQuery, map[string]any{"podId": podID})
	if err != nil {
		return nil, err
	}
	var pod *legacyPodFields
	if err := decodeGraphQLField(data, "pod", &pod); err != nil {
		return nil, err
	}
	return pod, nil
}

// fetchLegacyTemplateFields returns nil without an error when the GraphQL API
// does not report the template to the caller.
func fetchLegacyTemplateFields(ctx context.Context, client *legacy.Client, templateID string) (*legacyTemplateFields, error) {
	data, err := client.Query(ctx, legacyTemplateFieldsQuery, map[string]any{"id": templateID})
	if err != nil {
		return nil, err
	}
	var template *legacyTemplateFields
	if err := decodeGraphQLField(data, "podTemplate", &template); err != nil {
		return nil, err
	}
	return template, nil
}

// fetchLegacyEndpointOwner returns nil without an error when the endpoint or
// its owner is absent from the answer.
func fetchLegacyEndpointOwner(ctx context.Context, client *legacy.Client, endpointID string) (*string, error) {
	data, err := client.Query(ctx, legacyEndpointOwnerQuery, map[string]any{"id": endpointID})
	if err != nil {
		return nil, err
	}
	var myself *struct {
		Endpoint *struct {
			UserID *string `json:"userId"`
		} `json:"endpoint"`
	}
	if err := decodeGraphQLField(data, "myself", &myself); err != nil {
		return nil, err
	}
	if myself == nil || myself.Endpoint == nil {
		return nil, nil
	}
	return myself.Endpoint.UserID, nil
}

// fetchLegacyServerlessBilledTime reads the per-endpoint billed time for the
// window [start, end) at granularity, a BillingGranularity value. It returns
// no rows without an error when the API withholds the account's billing:
// User.billing answers null for a caller that may not read it.
func fetchLegacyServerlessBilledTime(ctx context.Context, client *legacy.Client, granularity string, start, end time.Time) ([]legacyServerlessBilledTime, error) {
	data, err := client.Query(ctx, legacyServerlessBilledTimeQuery, map[string]any{
		"input": map[string]any{
			"granularity": granularity,
			"startDate":   start.UTC().Format(time.RFC3339),
			"endDate":     end.UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		return nil, err
	}
	var myself *struct {
		Billing *struct {
			Serverless []legacyServerlessBilledTime `json:"serverless"`
		} `json:"billing"`
	}
	if err := decodeGraphQLField(data, "myself", &myself); err != nil {
		return nil, err
	}
	if myself == nil || myself.Billing == nil {
		return nil, nil
	}
	return myself.Billing.Serverless, nil
}

// fetchLegacyGPUCloudBilling reads GPU pod billing per GPU type for the window
// [start, end) at granularity, a BillingGranularity value. A null
// User.billing is errLegacyBillingWithheld: the rows are the whole answer of
// the read that asks for them.
func fetchLegacyGPUCloudBilling(ctx context.Context, client *legacy.Client, granularity string, start, end time.Time) ([]legacyGPUCloudBilling, error) {
	data, err := client.Query(ctx, legacyGPUCloudBillingQuery, map[string]any{
		"input": map[string]any{
			"granularity": granularity,
			"startDate":   start.UTC().Format(time.RFC3339),
			"endDate":     end.UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		return nil, err
	}
	var myself *struct {
		Billing *struct {
			GpuCloud []legacyGPUCloudBilling `json:"gpuCloud"`
		} `json:"billing"`
	}
	if err := decodeGraphQLField(data, "myself", &myself); err != nil {
		return nil, err
	}
	if myself == nil || myself.Billing == nil {
		return nil, errLegacyBillingWithheld
	}
	return myself.Billing.GpuCloud, nil
}

// saveLegacyTemplateReadme sets a template's readme through saveTemplate and
// returns the four fields it reads back.
//
// SaveTemplateInput requires the template's whole body, and on an update the
// saveTemplate resolver writes a default for a field the input leaves out: config becomes {}, category becomes NVIDIA, the
// volume mount path becomes /workspace, and env and dockerArgs become what was
// sent. So the template is read first and sent back with only the readme
// changed. The same resolver stores readme as trim(readme) and skips an empty
// one, so an empty readme cannot clear the text; callers reject "" before it
// gets here.
func saveLegacyTemplateReadme(ctx context.Context, client *legacy.Client, templateID, readme string) (*legacyTemplateFields, error) {
	data, err := client.Query(ctx, legacyTemplateForSaveQuery, map[string]any{"id": templateID})
	if err != nil {
		return nil, err
	}
	stored, ok := data["podTemplate"].(map[string]any)
	if !ok {
		return nil, errLegacyTemplateNotVisible
	}
	input, err := legacySaveTemplateInput(templateID, stored, readme)
	if err != nil {
		return nil, err
	}

	data, err = client.Query(ctx, legacySaveTemplateReadmeMutation, map[string]any{"input": input})
	if err != nil {
		return nil, err
	}
	var saved *legacyTemplateFields
	if err := decodeGraphQLField(data, "saveTemplate", &saved); err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, errors.New("saveTemplate answered without a template")
	}
	return saved, nil
}

// legacySaveTemplateEchoedFields are the optional SaveTemplateInput fields
// copied from the stored template as they were read. Each name is the same on
// PodTemplate and SaveTemplateInput.
var legacySaveTemplateEchoedFields = []string{
	"advancedStart", "allowedCudaVersions", "compatibleGPUIds", "config", "containerRegistryAuthId",
	"incompatibleGPUIds", "isPublic", "isServerless", "minRam", "minVram", "networkVolumeIds",
	"ports", "recommendedGPUIds", "startJupyter", "startScript", "startSsh", "volumeMountPath",
}

// legacyTemplateCategories is the TemplateCategory enum. PodTemplate.category
// reads as a String, so a stored value outside the enum cannot be sent back,
// and leaving it out would let the resolver default it to NVIDIA.
var legacyTemplateCategories = map[string]struct{}{"AMD": {}, "CPU": {}, "GPU": {}, "NVIDIA": {}}

// legacySaveTemplateInput builds the SaveTemplateInput that writes readme and
// leaves the rest of stored as it is. A required field the read did not carry
// stops the write rather than sending a value that would change the template.
func legacySaveTemplateInput(templateID string, stored map[string]any, readme string) (map[string]any, error) {
	input := map[string]any{"id": templateID, "readme": readme}

	for _, key := range legacySaveTemplateEchoedFields {
		if value, ok := stored[key]; ok && value != nil {
			input[key] = value
		}
	}

	switch category := stored["category"].(type) {
	case nil:
		return nil, errors.New("the stored template reports no category, and saveTemplate would set it to NVIDIA")
	case string:
		if _, ok := legacyTemplateCategories[category]; !ok {
			return nil, fmt.Errorf("the stored template category %q is not one saveTemplate accepts", category)
		}
		input["category"] = category
	default:
		return nil, fmt.Errorf("the stored template category has an unexpected type %T", category)
	}

	for _, key := range []string{"name", "containerDiskInGb", "volumeInGb"} {
		value, ok := stored[key]
		if !ok || value == nil {
			return nil, fmt.Errorf("the stored template reports no %s, which saveTemplate requires", key)
		}
		input[key] = value
	}

	// dockerArgs and env are required inputs that a template may store as
	// null; the empty value is what null means for both.
	dockerArgs, _ := stored["dockerArgs"].(string)
	input["dockerArgs"] = dockerArgs

	env := []map[string]any{}
	if entries, ok := stored["env"].([]any); ok {
		for _, entry := range entries {
			pair, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			key, ok := pair["key"].(string)
			if !ok {
				continue
			}
			value, _ := pair["value"].(string)
			env = append(env, map[string]any{"key": key, "value": value})
		}
	}
	input["env"] = env

	if ports, ok := stored["portsConfig"].([]any); ok {
		input["portsConfig"] = ports
	}

	return input, nil
}

// decodeGraphQLField decodes data[key] into out, a pointer to a pointer, so
// an absent or null field leaves out nil.
func decodeGraphQLField(data map[string]any, key string, out any) error {
	value, ok := data[key]
	if !ok || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("re-encoding the GraphQL %s field failed: %w", key, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decoding the GraphQL %s field failed: %w", key, err)
	}
	return nil
}
