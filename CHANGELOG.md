## 0.1.0 (Unreleased)

NOTES:
First version with workflow def and task def support.

## 0.2.0

NOTES:
* provider "endpoint" schema change, endpoint url should include the "/api" route prefix
* added debug tflogs for http calls

## 0.3.0

NOTES:
* Bug: Fix small leak bug. not closing http response body.
* Support for 2 versioning modes in workflow def, manual and auto inc.
