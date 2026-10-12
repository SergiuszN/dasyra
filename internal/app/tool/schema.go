package tool

import (
	"bytes"
	"encoding/json"

	"dasyra/internal/app/llm"
)

// Param is one argument of a tool in its JSON schema.
type Param struct {
	name     string
	schema   map[string]any
	required bool
}

func Str(name, description string) Param  { return param(name, "string", description) }
func Int(name, description string) Param  { return param(name, "integer", description) }
func Bool(name, description string) Param { return param(name, "boolean", description) }
func Enum(name, description string, values ...string) Param {
	p := Str(name, description)
	p.schema["enum"] = values
	return p
}

// List is an array of items of the given type, optionally limited to values.
func List(name, itemType, description string, values ...string) Param {
	items := map[string]any{"type": itemType}
	if len(values) > 0 {
		items["enum"] = values
	}
	p := param(name, "array", description)
	p.schema["items"] = items
	return p
}

// Objects is an array of objects with the given fields.
func Objects(name, description string, fields ...Param) Param {
	props, required := properties(fields)
	p := param(name, "array", description)
	p.schema["items"] = map[string]any{"type": "object", "properties": props, "required": required}
	return p
}

func param(name, typ, description string) Param {
	return Param{name: name, schema: map[string]any{"type": typ, "description": description}}
}

// Required marks the argument as one the model must pass.
func (p Param) Required() Param {
	p.required = true
	return p
}

// Spec describes a tool to the model. Properties keep their order, so the prompt stays the same between runs.
func Spec(name, description string, params ...Param) llm.Tool {
	props, required := properties(params)
	parameters := toJSON(map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	})
	return llm.Tool{Name: name, Description: description, Parameters: json.RawMessage(parameters)}
}

func properties(params []Param) (json.RawMessage, []string) {
	var props bytes.Buffer
	props.WriteByte('{')
	required := []string{}
	for i, p := range params {
		if i > 0 {
			props.WriteByte(',')
		}
		props.WriteString(toJSON(p.name))
		props.WriteByte(':')
		props.WriteString(toJSON(p.schema))
		if p.required {
			required = append(required, p.name)
		}
	}
	props.WriteByte('}')
	return json.RawMessage(props.Bytes()), required
}
