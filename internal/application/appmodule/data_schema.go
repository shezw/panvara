/*
   Panvara
   internal/application/appmodule/data_schema.go    2026-07-15
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package appmodule

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

// DataSchemaFormatVersion identifies the canonical data-schema projection.
// It is intentionally independent from the complete Module IR format.
const DataSchemaFormatVersion = 1

type dataSchemaIR struct {
	FormatVersion int              `json:"formatVersion"`
	Module        string           `json:"module"`
	Resources     []dataResourceIR `json:"resources"`
}

type dataResourceIR struct {
	Name   string        `json:"name"`
	Fields []dataFieldIR `json:"fields"`
}

type dataFieldIR struct {
	Name        string           `json:"name"`
	Kind        domain.FieldKind `json:"kind"`
	Required    bool             `json:"required"`
	Unique      bool             `json:"unique"`
	Target      string           `json:"target,omitempty"`
	Options     []string         `json:"options"`
	Constraints constraintsIR    `json:"constraints"`
}

func marshalDataSchemaProjection(descriptor domain.Descriptor) ([]byte, error) {
	resources := append([]domain.Resource(nil), descriptor.Resources...)
	sort.Slice(resources, func(left, right int) bool {
		return resources[left].Name < resources[right].Name
	})
	projection := dataSchemaIR{
		FormatVersion: DataSchemaFormatVersion,
		Module:        descriptor.Name,
		Resources:     make([]dataResourceIR, 0, len(resources)),
	}
	for _, resource := range resources {
		fields := append([]domain.Field(nil), resource.Fields...)
		sort.Slice(fields, func(left, right int) bool {
			return fields[left].Name < fields[right].Name
		})
		projected := dataResourceIR{
			Name: resource.Name, Fields: make([]dataFieldIR, 0, len(fields)),
		}
		for _, field := range fields {
			options := append([]string(nil), field.Options...)
			sort.Strings(options)
			if options == nil {
				options = []string{}
			}
			projected.Fields = append(projected.Fields, dataFieldIR{
				Name: field.Name, Kind: field.Kind, Required: field.Required,
				Unique: field.Unique, Target: field.Target, Options: options,
				Constraints: makeConstraintsIR(field.Constraints),
			})
		}
		projection.Resources = append(projection.Resources, projected)
	}
	return json.Marshal(projection)
}

func fingerprintDataSchema(projection []byte) string {
	digest := sha256.Sum256(projection)
	return "sha256:" + hex.EncodeToString(digest[:])
}
