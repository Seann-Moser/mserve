package extract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

type Rule struct {
	Host    string            `json:"host" bson:"host"`
	Rules   []*ExtractionRule `json:"rules" bson:"rules"`
	Mapping []*Mapping        `json:"mapping" bson:"mapping"`
}
type Mapping struct {
	Object string `json:"object" bson:"object"`

	Key         string                 `json:"key" bson:"key"`
	To          string                 `json:"to" bson:"to"`
	Format      map[string]interface{} `json:"format" bson:"format"` // replace ${value}, ${gjson.value}
	IsArray     bool                   `json:"isArray" bson:"isArray"`
	IsObject    bool                   `json:"isObject" bson:"isObject"`
	ArrayObjMap []*Mapping             `json:"arrayObjMap" bson:"arrayObjMap"`
}

func MapResultsResults(result Result, r *Rule) (map[string]interface{}, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	objMap := map[string]map[string]interface{}{}
	remappedData := make(map[string]interface{})

	for _, mapping := range r.Mapping {
		gjsonResult := gjson.GetBytes(data, mapping.Key)

		if mapping.IsArray || len(mapping.ArrayObjMap) > 0 {
			var arrayResult []interface{}
			index := 0
			gjsonResult.ForEach(func(_, value gjson.Result) bool {
				defer func() {
					index++
				}()
				if len(mapping.ArrayObjMap) > 0 {
					// Pass the entire slice of sub-mappings to the helper function
					nestedObj, err := remapSingleObject(value, mapping.ArrayObjMap, index)
					if err != nil {
						// Log error or handle it
						return true
					}
					arrayResult = append(arrayResult, nestedObj)
				} else {
					// Fallback to the original approach
					arrayResult = append(arrayResult, value.Value())
				}
				return true
			})

			if mapping.Object == "" {
				remappedData = complexPathing(remappedData, mapping.To, arrayResult)
			} else {
				// Fixed logic: ensure the object map exists, then update it.
				if _, ok := objMap[mapping.Object]; !ok {
					objMap[mapping.Object] = make(map[string]interface{})
				}
				// complexPathing modifies the map in place, no need for assignment.
				objMap[mapping.Object] = complexPathing(objMap[mapping.Object], mapping.To, arrayResult)
			}
		} else {
			if mapping.Object == "" {
				remappedData = complexPathing(remappedData, mapping.To, gjsonResult.Value())
			} else {
				// Fixed logic: ensure the object map exists, then update it.
				if _, ok := objMap[mapping.Object]; !ok {
					objMap[mapping.Object] = make(map[string]interface{})
				}
				// complexPathing modifies the map in place, no need for assignment.
				objMap[mapping.Object] = complexPathing(objMap[mapping.Object], mapping.To, gjsonResult.Value())
			}
		}
	}

	return remappedData, nil
}

// and sets the provided value at the final key.
func complexPathing(maps map[string]interface{}, path string, value interface{}) map[string]interface{} {
	// Split the path into individual keys
	keys := strings.Split(path, ".")

	// Use a pointer to the current map level to traverse the structure.
	// We'll start with the root map.
	currentMap := maps

	// Iterate through the keys in the path, creating new nested maps as needed.
	for i, key := range keys {
		// Check if we are at the last key in the path
		if i == len(keys)-1 {
			re := regexp.MustCompile(`\[[0-9]+]`)
			// If it's the last key, set the final value and break the loop.
			if v, ok := value.([]interface{}); ok {
				n, err := extractNumberFromBrackets(key)
				if err != nil || n > len(v) {
					currentMap[key] = v
				} else {
					currentMap[re.ReplaceAllString(key, "")] = v[n]
				}
			} else {
				currentMap[key] = value
			}
			break
		}

		// Check if the current key exists in the map
		if _, ok := currentMap[key]; !ok {
			// If the key doesn't exist, create a new nested map for it.
			newMap := make(map[string]interface{})
			currentMap[key] = newMap
			// Move the currentMap pointer to the newly created map to continue traversal.
			currentMap = newMap
		} else {
			// If the key exists, we need to check if its value is a map.
			// This is a type assertion to ensure we're on the right path.
			// If the value isn't a map, the path is broken and we can't continue.
			if nextMap, ok := currentMap[key].(map[string]interface{}); ok {
				// The value is a map, so we can continue traversing.
				currentMap = nextMap
			} else {
				// The path is invalid (e.g., trying to set "a.b" but "a" is a string).
				// We'll just stop here to prevent a panic.
				// In a real-world application, you might want to return an error.
				fmt.Printf("pathing failed: key '%s' is not a map\n", key)
				return maps
			}
		}
	}

	return maps
}

func extractNumberFromBrackets(s string) (int, error) {
	// Regular expression to match a number inside square brackets.
	// The `(\d+)` is a capturing group that matches one or more digits.
	re := regexp.MustCompile(`\[[0-9]+]`)

	// FindStringSubmatch returns a slice of strings containing the text
	// of the matched subexpressions.
	// The first element is the full match, and subsequent elements are the
	// captured groups.
	match := re.FindString(s)
	if len(match) < 2 {
		return 0, fmt.Errorf("no number found inside brackets in string: %s", s)
	}

	// The captured number string is the second element in the slice.
	numberStr := regexp.MustCompile(`[\[\]]`).ReplaceAllString(match, "")
	// Convert the captured string to an integer.
	// This handles cases where the value inside the brackets is not a valid number.
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return 0, fmt.Errorf("failed to convert '%s' to an integer: %w", numberStr, err)
	}

	return number, nil
}

// remapSingleObject now takes a slice of mappings
func remapSingleObject(obj gjson.Result, rules []*Mapping, index int) (map[string]interface{}, error) {
	remappedObj := make(map[string]interface{})

	for _, rule := range rules {
		rule.Key = strings.ReplaceAll(rule.Key, "[", ".")
		rule.Key = strings.ReplaceAll(rule.Key, "]", "")
		gjsonVal := obj.Get(rule.Key)
		if rule.Key == "" || rule.Key == "." {
			gjsonVal = obj
		}
		// FIX: Use a deep copy of the format map to prevent shared references.
		// This ensures each object in the array gets its own unique nested maps.
		if rule.Format == nil {
			rule.Format = map[string]interface{}{}
		}
		formatCopy := deepCopyMap(rule.Format, index)
		for k, v := range formatCopy {
			//to map format copy
			remappedObj[k] = v
		}

		if gjsonVal.Exists() {
			// complexPathing modifies the map in place, no need for re-assignment
			complexPathing(remappedObj, rule.To, gjsonVal.Value())
		}
	}
	return remappedObj, nil
}

func deepCopy(value interface{}, index int) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return deepCopyMap(v, index)
	case []interface{}:
		return deepCopySlice(v)
	default:
		return replacePlaceholdersInterface(v, index)
	}
}

func deepCopyMap(m map[string]interface{}, index int) map[string]interface{} {
	if m == nil {
		return nil
	}
	newMap := make(map[string]interface{}, len(m))
	for k, v := range m {
		newMap[k] = deepCopy(v, index)
	}
	return newMap
}

func deepCopySlice(s []interface{}) []interface{} {
	if s == nil {
		return nil
	}
	newSlice := make([]interface{}, len(s))
	for i, v := range s {
		newSlice[i] = deepCopy(v, i)
	}
	return newSlice
}
