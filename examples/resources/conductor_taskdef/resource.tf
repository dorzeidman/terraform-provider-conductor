resource "conductor_taskdef" "this" {
  manifest = <<EOF
  {
    "name": "name",
    "description": "",
    "retryCount": 4,
    "timeoutSeconds": 3600,
    "inputKeys": [],
    "outputKeys": [],
    "timeoutPolicy": "TIME_OUT_WF",
    "retryLogic": "FIXED",
    "retryDelaySeconds": 30,
    "responseTimeoutSeconds": 600,
    "inputTemplate": {},
    "rateLimitPerFrequency": 0,
    "rateLimitFrequencyInSeconds": 1,
    "ownerEmail": "owner@example.com",
    "backoffScaleFactor": 1,
    "enforceSchema": false
    }
    EOF
}
