resource "conductor_workflowdef" "this" {
  manifest = <<EOF
  {
    "name": "name",
    "description": "desc",
    "tasks": [
      {
        "name": "task1",
        "taskReferenceName": "task1",
        "inputParameters": {
          "input1": "$${workflow.input.input1}"
        },
        "type": "SIMPLE",
        "decisionCases": {},
        "defaultCase": [],
        "forkTasks": [],
        "startDelay": 0,
        "joinOn": [],
        "optional": false,
        "defaultExclusiveJoinTask": [],
        "asyncComplete": false,
        "loopOver": [],
        "onStateChange": {},
        "permissive": false
      }
    ],
    "inputParameters": [],
    "outputParameters": {},
    "schemaVersion": 2,
    "restartable": true,
    "workflowStatusListenerEnabled": false,
    "ownerEmail": "owner@example.com",
    "timeoutPolicy": "ALERT_ONLY",
    "timeoutSeconds": 0
  }
  EOF
}
