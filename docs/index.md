---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "conductor Provider"
subcategory: ""
description: |-
  The Conductor Provider used create resource on conductor platform
  This is an unofficial Terraform provider for Conducotr
  See Conductor OSS reference: https://github.com/conductor-oss/conductor
---

# conductor Provider

The Conductor Provider used create resource on conductor platform
This is an unofficial Terraform provider for Conducotr
See Conductor OSS reference: https://github.com/conductor-oss/conductor

## Example Usage

```terraform
provider "conductor" {
  endpoint = "http://localhost:8080/api"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `endpoint` (String) Endpoint of the Conductor API, the endpoint should include the /api prefix. e.g. - http://localhost:6251/api

### Optional

- `custom_headers` (Map of String) Custom http headers to send for every request
