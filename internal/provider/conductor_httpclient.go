package provider

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
)

type conductorHttpClient struct {
	httpClient *http.Client
	endpoint   string
	headers    map[string]string
}

func (client *conductorHttpClient) createRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := fmt.Sprintf("%s/%s", client.endpoint, path)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	for key, value := range client.headers {
		req.Header.Add(key, value)
	}
	req.Header.Add("Content-Type", "application/json")

	return req, nil
}

func createConductorHttpClient(data ConductorProviderModel) *conductorHttpClient {
	endpointStr := data.Endpoint.ValueString()
	endpointStr = strings.TrimSuffix(endpointStr, "/")

	conductorClient := conductorHttpClient{
		httpClient: http.DefaultClient,
		endpoint:   endpointStr,
		headers:    make(map[string]string),
	}

	if !data.CustomHeaders.IsNull() {
		for key, value := range data.CustomHeaders.Elements() {
			if !value.IsNull() && !value.IsUnknown() {
				stringVal, ok := value.(tftypes.String)
				if ok {
					conductorClient.headers[key] = stringVal.ValueString()
				}
			}
		}
	}
	return &conductorClient
}

func (client *conductorHttpClient) sendRequest(req *http.Request) (*http.Response, error) {
	return client.httpClient.Do(req)
}

func (client *conductorHttpClient) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := client.createRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	return client.sendRequest(req)
}
