package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// BaseTool 工具基类
type BaseTool struct {
	name        string
	description string
	parameters  map[string]interface{}
}

// Name 返回工具名称
func (t *BaseTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *BaseTool) Description() string {
	return t.description
}

// Parameters 返回参数定义
func (t *BaseTool) Parameters() map[string]interface{} {
	return t.parameters
}

// ToOpenAISchema 转换为 OpenAI 函数格式
func (t *BaseTool) ToOpenAISchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        t.name,
			"description": t.description,
			"parameters":  t.parameters,
		},
	}
}

// ValidateParams 验证参数
func ValidateParams(schema, params map[string]interface{}) error {
	if schema == nil {
		return nil
	}

	schemaType, _ := schema["type"].(string)
	if schemaType != "object" {
		return fmt.Errorf("schema type must be 'object', got '%s'", schemaType)
	}

	properties, _ := schema["properties"].(map[string]interface{})

	// 检查必填字段 (支持 []string 和 []interface{})
	if required, ok := schema["required"].([]string); ok {
		for _, reqStr := range required {
			if _, exists := params[reqStr]; !exists {
				return fmt.Errorf("missing required parameter: %s", reqStr)
			}
		}
	} else if required, ok := schema["required"].([]interface{}); ok {
		for _, req := range required {
			reqStr, ok := req.(string)
			if !ok {
				continue
			}
			if _, exists := params[reqStr]; !exists {
				return fmt.Errorf("missing required parameter: %s", reqStr)
			}
		}
	}

	// 验证每个参数的类型
	for key, value := range params {
		propSchema, exists := properties[key]
		if !exists {
			continue // 忽略未知字段
		}
		propMap, ok := propSchema.(map[string]interface{})
		if !ok {
			continue
		}

		if err := validateValue(value, propMap, key); err != nil {
			return err
		}
	}

	return nil
}

// validateValue 验证单个值
func validateValue(value interface{}, schema map[string]interface{}, path string) error {
	propType, _ := schema["type"].(string)

	switch propType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s should be string", path)
		}
		str := value.(string)
		if minLen, ok := schema["minLength"].(float64); ok && float64(len(str)) < minLen {
			return fmt.Errorf("%s must be at least %d chars", path, int(minLen))
		}
		if maxLen, ok := schema["maxLength"].(float64); ok && float64(len(str)) > maxLen {
			return fmt.Errorf("%s must be at most %d chars", path, int(maxLen))
		}

	case "integer":
		if !isInteger(value) {
			return fmt.Errorf("%s should be integer", path)
		}
		num := toFloat64(value)
		if minimum, ok := schema["minimum"].(float64); ok && num < minimum {
			return fmt.Errorf("%s must be >= %v", path, minimum)
		}
		if maximum, ok := schema["maximum"].(float64); ok && num > maximum {
			return fmt.Errorf("%s must be <= %v", path, maximum)
		}

	case "number":
		if !isNumber(value) {
			return fmt.Errorf("%s should be number", path)
		}
		num := toFloat64(value)
		if minimum, ok := schema["minimum"].(float64); ok && num < minimum {
			return fmt.Errorf("%s must be >= %v", path, minimum)
		}
		if maximum, ok := schema["maximum"].(float64); ok && num > maximum {
			return fmt.Errorf("%s must be <= %v", path, maximum)
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s should be boolean", path)
		}

	case "array":
		arr, ok := value.([]interface{})
		if !ok {
			return fmt.Errorf("%s should be array", path)
		}
		if itemsSchema, ok := schema["items"].(map[string]interface{}); ok {
			for i, item := range arr {
				if err := validateValue(item, itemsSchema, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}

	case "object":
		obj, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s should be object", path)
		}
		if err := ValidateParams(schema, obj); err != nil {
			return fmt.Errorf("%s.%s", path, err.Error())
		}
	}

	// 检查 enum
	if enum, ok := schema["enum"].([]interface{}); ok {
		found := false
		for _, v := range enum {
			if reflect.DeepEqual(value, v) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s must be one of %v", path, enum)
		}
	}

	return nil
}

// isInteger 检查是否为整数类型
func isInteger(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return v.(float64) == float64(int64(v.(float64)))
	case float32:
		return v.(float32) == float32(int32(v.(float32)))
	default:
		return false
	}
}

// isNumber 检查是否为数字类型
func isNumber(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

// toFloat64 转换为 float64
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int8:
		return float64(n)
	case int16:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint8:
		return float64(n)
	case uint16:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

// ToolCall 工具调用
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ParseToolCalls 从 JSON 解析工具调用
func ParseToolCalls(data []byte) ([]*ToolCall, error) {
	var calls []*ToolCall
	if err := json.Unmarshal(data, &calls); err != nil {
		return nil, err
	}
	return calls, nil
}
