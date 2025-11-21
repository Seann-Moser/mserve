package nuxt3FromOpenApi

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3" // Make sure you have this dependency: go get gopkg.in/yaml.v3
)

// --- Go Structs for OpenAPI YAML Parsing ---
// These structs are simplified to capture only the necessary parts for client generation.
type OpenAPI struct {
	Info struct {
		Title string `yaml:"title"`
	} `yaml:"info"`
	Paths map[string]map[string]Operation `yaml:"paths"`
	// Components for schemas, securitySchemes etc. could be added later if needed
}

type Operation struct {
	OperationID string                `yaml:"operationId"`
	Summary     string                `yaml:"summary"`
	Description string                `yaml:"description"`
	Parameters  []Parameter           `yaml:"parameters"`
	RequestBody *RequestBody          `yaml:"requestBody"`
	Responses   map[string]Response   `yaml:"responses"`
	Security    []map[string][]string `yaml:"security"`
}

type Parameter struct {
	Name     string `yaml:"name"`
	In       string `yaml:"in"` // path, query, header, cookie
	Required bool   `yaml:"required"`
	Schema   Schema `yaml:"schema"`
	Style    string `yaml:"style"` // e.g., 'form' for query params, 'simple' for path
	Explode  *bool  `yaml:"explode"`
}

type Schema struct {
	Type        string            `yaml:"type"` // string, integer, boolean, array, object
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Properties  map[string]Schema `yaml:"properties"`
	// Format, items, properties could be added
}

type RequestBody struct {
	Content  map[string]MediaType `yaml:"content"`
	Required bool                 `yaml:"required"`
}

type MediaType struct {
	Schema Schema `yaml:"schema"`
}

type Response struct {
	Description string               `yaml:"description"`
	Content     map[string]MediaType `yaml:"content"`
}

// --- Data Structures for JavaScript Template Generation ---
type PluginTemplateData struct {
	Name      string
	ApiName   string
	Endpoints []EndpointData
}

type EndpointData struct {
	Method            string
	Path              string
	FunctionName      string
	PathParams        []Parameter
	QueryParams       []Parameter
	HeaderParams      []Parameter
	RequestBodyVar    string
	HasBody           bool
	Security          []map[string][]string // Raw security data (can be used for auth logic)
	RequestBodySchema RequestBodySchema
}

type Property struct {
	Name        string
	Schema      *Schema
	Description string
}

type RequestBodySchema struct {
	Properties []Property
}

func GenerateNuxt3Plugin(yamlFile []byte) (string, error) {
	var openapi OpenAPI
	err := yaml.Unmarshal([]byte(yamlFile), &openapi)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling OpenAPI YAML: %v", err)
	}
	pluginData := PluginTemplateData{
		Name:    ToPascalCase(openapi.Info.Title),
		ApiName: ToSnakeCase(openapi.Info.Title),
	}
	for path, methods := range openapi.Paths {
		for method, op := range methods {
			// todo check func name and add Get Post Delete Etc...
			funcName := op.OperationID
			if funcName == "" {
				funcName = generateFunctionName(method, path)
			}
			funcName = strings.ReplaceAll(funcName, " ", "")

			endpoint := EndpointData{
				Method:       strings.ToUpper(method),
				Path:         path,
				FunctionName: funcName,
				Security:     op.Security,
			}

			for _, param := range op.Parameters {
				switch param.In {
				case "path":
					endpoint.PathParams = append(endpoint.PathParams, param)
				case "query":
					endpoint.QueryParams = append(endpoint.QueryParams, param)
				case "header":
					param.Name = strings.ReplaceAll(param.Name, "-", "_")
					endpoint.HeaderParams = append(endpoint.HeaderParams, param)
				}
			}

			if op.RequestBody != nil {
				endpoint.HasBody = true
				endpoint.RequestBodyVar = "body"

				media := op.RequestBody.Content["application/json"]
				schema := media.Schema
				for propName, propSchemaRef := range schema.Properties {
					p := Property{
						Name:        propName,
						Schema:      &propSchemaRef,
						Description: propSchemaRef.Description,
					}
					endpoint.RequestBodySchema.Properties = append(endpoint.RequestBodySchema.Properties, p)
				}
			}

			pluginData.Endpoints = append(pluginData.Endpoints, endpoint)
		}
	}

	pluginContent, err := generatePluginJS(pluginData)
	if err != nil {
		return "", err
	}
	return pluginContent, nil
}

// ToPascalCase converts an input string to PascalCase.
// It first normalizes via ToSnakeCase, then upper-capitalizes each segment.
func ToPascalCase(s string) string {
	snake := ToSnakeCase(s)
	parts := strings.Split(snake, "_")
	var b strings.Builder

	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		// Uppercase first rune
		b.WriteRune(unicode.ToUpper(runes[0]))
		// Lowercase the rest
		for _, r := range runes[1:] {
			b.WriteRune(unicode.ToLower(r))
		}
	}

	return b.String()
}

// ToSnakeCase converts the input string to snake_case.
// It handles spaces, punctuation, camelCase and PascalCase,
// inserting underscores between words and lowercasing everything.
func ToSnakeCase(s string) string {
	var b strings.Builder
	prevIsLower := false

	for i, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			switch {
			// if uppercase and previous was a lower (camelCase transition): add underscore
			case unicode.IsUpper(r) && prevIsLower:
				b.WriteRune('_')
				b.WriteRune(unicode.ToLower(r))
			// if uppercase and next is lower (PascalCase like "HTMLInput"): split between acronyms
			case unicode.IsUpper(r) && i > 0:
				// look ahead to see if next rune is lower-case
				if next, _ := utf8.DecodeRuneInString(s[i+utf8.RuneLen(r):]); unicode.IsLower(next) {
					b.WriteRune('_')
				}
				b.WriteRune(unicode.ToLower(r))
			default:
				b.WriteRune(unicode.ToLower(r))
			}
			prevIsLower = unicode.IsLower(r) || unicode.IsDigit(r)
		} else {
			// any non-alnum becomes underscore (but avoid duplicate underscores)
			if b.Len() > 0 && b.String()[b.Len()-1] != '_' {
				b.WriteRune('_')
			}
			prevIsLower = false
		}
	}

	// Trim any leading/trailing underscores
	res := b.String()
	res = strings.Trim(res, "_")
	return res
}

// --- Helper Functions for Code Generation ---

func generateFunctionName(method, path string) string {
	parts := strings.Split(path, "/")
	var nameParts []string

	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			paramName := strings.Trim(part, "{}")
			nameParts = append(nameParts, "By"+Title(paramName))
		} else if part != "" {
			nameParts = append(nameParts, Title(part))
		}
	}

	name := strings.ToLower(method) + strings.Join(nameParts, "")
	// Remove all whitespace
	return regexp.MustCompile(`\s+`).ReplaceAllString(name, "")
}

func Title(s string) string {
	// Create a caser for title-casing, using Unicode rules
	caser := cases.Title(language.English)
	return caser.String(s)
}

// --- JavaScript Template Generation Functions ---

func generatePluginJS(data PluginTemplateData) (string, error) {
	// IMPORTANT CHANGE HERE: Read template content from file
	//templatePath := "template.txt" // Assuming template.txt is in the same directory as main.go
	//templateContent, err := ioutil.ReadFile(templatePath)
	//if err != nil {
	//	return "", fmt.Errorf("failed to read plugin template file '%s': %w", templatePath, err)
	//}

	tmpl := template.Must(template.New("plugin").Funcs(template.FuncMap{
		"jsParams": func(ep EndpointData) string {
			var params []string
			for _, p := range ep.PathParams {
				params = append(params, p.Name)
			}
			for _, p := range ep.QueryParams {
				p.Name = strings.ReplaceAll(p.Name, "-", "_")
				paramSig := p.Name

				//if !p.Required {
				//	paramSig = paramSig + "?"
				//}
				params = append(params, paramSig)
			}
			for _, p := range ep.HeaderParams {
				p.Name = strings.ReplaceAll(p.Name, "-", "_")
				paramSig := p.Name
				//if !p.Required {
				//	paramSig = paramSig + "?"
				//}
				params = append(params, paramSig)
			}
			if ep.HasBody {
				params = append(params, ep.RequestBodyVar)
			}
			return strings.Join(params, ", ")
		},
		"jsArgs": func(ep EndpointData) string {
			var args []string
			for _, p := range ep.PathParams {
				args = append(args, p.Name)
			}
			for _, p := range ep.QueryParams {
				args = append(args, p.Name)
			}
			for _, p := range ep.HeaderParams {
				p.Name = strings.ReplaceAll(p.Name, "-", "_")
				args = append(args, p.Name)
			}
			if ep.HasBody {
				args = append(args, ep.RequestBodyVar)
			}
			return strings.Join(args, ", ")
		},
		"interpolatePath": func(path string, pathParams []Parameter) string {
			interpolatedPath := "'" + path + "'"
			for _, p := range pathParams {
				interpolatedPath = strings.ReplaceAll(interpolatedPath, fmt.Sprintf("{%s}", p.Name), fmt.Sprintf("' + %s + '", p.Name))
			}
			interpolatedPath = strings.ReplaceAll(interpolatedPath, "'' + ", "")
			interpolatedPath = strings.ReplaceAll(interpolatedPath, " + ''", "")
			return interpolatedPath
		},
		"queryParamsString": func(queryParams []Parameter) string {
			if len(queryParams) == 0 {
				return ""
			}
			var parts []string
			parts = append(parts, "const urlParams = new URLSearchParams();")
			for _, p := range queryParams {
				parts = append(parts, fmt.Sprintf(`if (%s !== undefined && %s !== null) { urlParams.append('%s', %s); }`, p.Name, p.Name, p.Name, p.Name))
			}
			return strings.Join(parts, "\n") + "\n"
		},
		"headersObject": func(headerParams []Parameter) string {
			if len(headerParams) == 0 {
				return "{}"
			}
			var parts []string
			parts = append(parts, "const headers = {};")
			for _, p := range headerParams {
				p.Name = strings.ReplaceAll(p.Name, "-", "_")
				parts = append(parts, fmt.Sprintf(`if (%s !== undefined && %s !== null) { headers['%s'] = %s; }`, p.Name, p.Name, strings.ReplaceAll(p.Name, "_", "-"), p.Name))
			}
			parts = append(parts, "return headers;")
			return "(() => {\n" + strings.Join(parts, "\n") + "\n})()"
		},
		// MODIFIED: Only generates .append() calls, urlParams is declared in the template
		"queryParamsAppends": func(queryParams []Parameter) string {
			if len(queryParams) == 0 {
				return ""
			}
			var parts []string
			for _, p := range queryParams {
				// String() cast added for robustness, although JS usually handles primitives
				parts = append(parts, fmt.Sprintf(`if (%s !== undefined && %s !== null) { urlParams.append('%s', String(%s)); }`, p.Name, p.Name, p.Name, p.Name))
			}
			return strings.Join(parts, "\n")
		},
		"jsDocType": func(s Schema) string {
			switch s.Type {
			case "string":
				return "string"
			case "integer", "number":
				return "number"
			case "boolean":
				return "boolean"
			case "array":
				return "Array<any>"
			case "object":
				return "Object"
			default:
				return "any"
			}
		},
	}).Parse(pluginTemplate)) // Pass the content read from file
	var buf strings.Builder
	err := tmpl.Execute(&buf, data)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// --- JavaScript Template Strings ---

// Template for the API client plugin file (`api-client.js`)
//
//go:embed template.txt
var pluginTemplate string
