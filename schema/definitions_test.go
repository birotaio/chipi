package schema

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/franela/goblin"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/schmurfy/chipi/shared"
)

type jsonSchemaResolver struct {
	schema    *openapi3.Schema
	createRef bool
}

func (r *jsonSchemaResolver) SchemaResolver(_ shared.AttributeInfo, _ string, _ reflect.Type, _ shared.GenerateSchemaCallbackType) (*openapi3.Schema, bool) {
	return r.schema, r.createRef
}

type WithJsonSchema struct {
	Config map[string]any `chipi:"as:config"`
}

func TestHoistDefinitions(t *testing.T) {
	g := goblin.Goblin(t)

	g.Describe("cast resolver returning a JSON Schema with definitions", func() {
		var ctx context.Context
		var s *Schema
		var doc *openapi3.T
		var resolver *jsonSchemaResolver

		generateConfig := func() map[string]any {
			_, err := s.GenerateFilteredSchemaFor(ctx, doc, reflect.TypeOf(&WithJsonSchema{}), shared.NewChipiCallbacks(resolver))
			require.NoError(g, err)

			data, err := json.Marshal(doc.Components.Schemas[typeName(reflect.TypeOf(WithJsonSchema{}))])
			require.NoError(g, err)

			var parsed struct {
				Properties map[string]map[string]any `json:"properties"`
			}
			require.NoError(g, json.Unmarshal(data, &parsed))
			return parsed.Properties["Config"]
		}

		componentJSON := func(name string) string {
			require.Contains(g, doc.Components.Schemas, name)
			data, err := json.Marshal(doc.Components.Schemas[name])
			require.NoError(g, err)
			return string(data)
		}

		g.BeforeEach(func() {
			var err error
			ctx = context.Background()
			doc = &openapi3.T{}
			s, err = New()
			require.NoError(g, err)

			resolver = &jsonSchemaResolver{schema: &openapi3.Schema{}}
			require.NoError(g, json.Unmarshal([]byte(`{
				"type": "object",
				"definitions": {
					"localized": {"type": ["object", "null"], "additionalProperties": {"type": "string"}}
				},
				"$defs": {
					"link": {"type": "object", "properties": {"label": {"$ref": "#/definitions/localized"}}}
				},
				"properties": {
					"url": {"$ref": "#/definitions/localized"},
					"title": {"description": "see #/definitions/localized", "oneOf": [{"$ref": "#/definitions/localized"}]},
					"links": {"type": "array", "items": {"$ref": "#/$defs/link"}}
				}
			}`), resolver.schema))
		})

		g.It("should move the definitions to components and rewrite the local refs", func() {
			config := generateConfig()
			data, err := json.Marshal(config)
			require.NoError(g, err)

			assert.JSONEq(g, `{
				"type": "object",
				"properties": {
					"url": {"$ref": "#/components/schemas/custom_config.localized"},
					"title": {"description": "see #/definitions/localized", "oneOf": [{"$ref": "#/components/schemas/custom_config.localized"}]},
					"links": {"type": "array", "items": {"$ref": "#/components/schemas/custom_config.link"}}
				}
			}`, string(data))
			assert.JSONEq(g, `{"type": ["object", "null"], "additionalProperties": {"type": "string"}}`, componentJSON("custom_config.localized"))
			assert.JSONEq(g, `{"type": "object", "properties": {"label": {"$ref": "#/components/schemas/custom_config.localized"}}}`, componentJSON("custom_config.link"))
		})

		g.It("should rewrite the refs of a schema stored as a component", func() {
			resolver.createRef = true

			config := generateConfig()
			assert.Equal(g, "#/components/schemas/custom_config", config["$ref"])
			assert.JSONEq(g, `{"$ref": "#/components/schemas/custom_config.localized"}`, mustMarshal(g, doc.Components.Schemas["custom_config"].Value.Properties["url"]))
			assert.Contains(g, doc.Components.Schemas, "custom_config.link")
		})

		g.It("should leave the resolver schema untouched so every generation hoists again", func() {
			first := generateConfig()

			doc = &openapi3.T{}
			second := generateConfig()

			assert.Equal(g, first, second)
			assert.Contains(g, doc.Components.Schemas, "custom_config.localized")
			assert.Contains(g, resolver.schema.Extensions, "definitions")
			assert.Equal(g, "#/definitions/localized", resolver.schema.Properties["url"].Ref)
		})

		g.It("should return the resolver schema as is without definitions", func() {
			resolver.schema = openapi3.NewStringSchema()

			config := generateConfig()
			assert.Equal(g, map[string]any{"type": "string"}, config)
			assert.Len(g, doc.Components.Schemas, 1)
		})
	})
}

func mustMarshal(g *goblin.G, v any) string {
	data, err := json.Marshal(v)
	require.NoError(g, err)
	return string(data)
}
