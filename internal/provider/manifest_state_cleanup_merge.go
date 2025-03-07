package provider

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func cleanupManifestDefaults(ctx context.Context, manifestMap map[string]interface{},
	defaultValues map[string]interface{}) {

	for key, value := range manifestMap {
		if value == nil {
			delete(manifestMap, key)
			continue
		}

		if isPrimitiveValue(value) {
			defaultValue := getPrimitiveDefaultValue(key, value, defaultValues)
			if defaultValue == nil {
				continue
			}

			if reflect.DeepEqual(value, defaultValue) {
				delete(manifestMap, key)
			}

			continue
		}

		//Map
		if mapVal, isMap := value.(map[string]interface{}); isMap {
			if len(mapVal) == 0 {
				delete(manifestMap, key)
			}

			continue
		}

		//Array
		if sliceVal, isSlice := value.([]interface{}); isSlice {
			if len(sliceVal) == 0 {
				delete(manifestMap, key)
			}

			continue
		}

		//Unknown
		tflog.Error(ctx, fmt.Sprintf("Key: %s, has invalid type: %T", key, value))
	}
}

func getPrimitiveDefaultValue(key string, value interface{}, defaultValues map[string]interface{}) interface{} {

	if defValue, defExist := defaultValues[key]; defExist {
		return defValue
	}

	//fallback
	if _, isBool := value.(bool); isBool {
		return false
	}

	if _, isFloat64 := value.(float64); isFloat64 {
		return float64(0)
	}

	if _, isString := value.(string); isString {
		return ""
	}

	return nil

}

func isPrimitiveValue(value interface{}) bool {

	if _, isBool := value.(bool); isBool {
		return true
	}

	if _, isFloat64 := value.(float64); isFloat64 {
		return true
	}

	if _, isString := value.(string); isString {
		return true
	}

	return false
}

func mergeManifestMaps(_ context.Context, fromMap map[string]interface{}, toMap map[string]interface{}) {
	for key, fromValue := range fromMap {
		_, exists := toMap[key]
		if !exists || isPrimitiveValue(fromValue) {
			toMap[key] = fromValue
			continue
		}
	}
}
