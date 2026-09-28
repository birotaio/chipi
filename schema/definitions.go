package schema

import (
	"encoding/json"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// A resolver may return a standalone JSON Schema whose local refs (#/definitions/x, #/$defs/x) would
// resolve against the document root: its definitions are moved to components and the refs rewritten.
func hoistDefinitions(doc *openapi3.T, prefix string, schema *openapi3.Schema) (*openapi3.Schema, error) {
	if schema == nil || (len(schema.Defs) == 0 && schema.Extensions["definitions"] == nil) {
		return schema, nil
	}

	// the resolver may return a shared schema, work on a copy to keep it untouched
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}

	hoisted := &openapi3.Schema{}
	if err := json.Unmarshal(data, hoisted); err != nil {
		return nil, err
	}

	var draft07 struct {
		Definitions openapi3.Schemas `json:"definitions"`
	}
	if err := json.Unmarshal(data, &draft07); err != nil {
		return nil, err
	}

	definitions := openapi3.Schemas{}
	for _, defs := range []openapi3.Schemas{draft07.Definitions, hoisted.Defs} {
		for name, def := range defs {
			definitions[name] = def
		}
	}
	hoisted.Defs = nil
	delete(hoisted.Extensions, "definitions")

	rewriteLocalRefs(&openapi3.SchemaRef{Value: hoisted}, prefix)
	for name, def := range definitions {
		rewriteLocalRefs(def, prefix)
		doc.Components.Schemas[prefix+"."+name] = def
	}

	return hoisted, nil
}

func rewriteLocalRefs(ref *openapi3.SchemaRef, prefix string) {
	if ref == nil {
		return
	}

	for _, local := range []string{"#/definitions/", "#/$defs/"} {
		if name, found := strings.CutPrefix(ref.Ref, local); found {
			ref.Ref = "#/components/schemas/" + prefix + "." + name
			return
		}
	}

	s := ref.Value
	if s == nil {
		return
	}

	for _, refs := range []openapi3.SchemaRefs{s.OneOf, s.AnyOf, s.AllOf, s.PrefixItems} {
		for _, child := range refs {
			rewriteLocalRefs(child, prefix)
		}
	}
	for _, schemas := range []openapi3.Schemas{s.Properties, s.PatternProperties, s.DependentSchemas} {
		for _, child := range schemas {
			rewriteLocalRefs(child, prefix)
		}
	}
	for _, child := range []*openapi3.SchemaRef{
		s.Not, s.Items, s.AdditionalProperties.Schema, s.Contains, s.PropertyNames,
		s.If, s.Then, s.Else, s.ContentSchema,
	} {
		rewriteLocalRefs(child, prefix)
	}
}
