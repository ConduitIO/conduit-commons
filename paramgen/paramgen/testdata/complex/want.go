// Code generated by paramgen. DO NOT EDIT.
// Source: github.com/ConduitIO/conduit-commons/tree/main/paramgen

package example

import (
	"github.com/conduitio/conduit-commons/config"
)

const (
	SourceConfigCustomType                = "customType"
	SourceConfigGlobalDuration            = "global.duration"
	SourceConfigGlobalRenamed             = "global.renamed.*"
	SourceConfigGlobalWildcardStrings     = "global.wildcardStrings.*"
	SourceConfigGlobalWildcardStructsName = "global.wildcardStructs.*.name"
	SourceConfigNestMeHereAnotherNested   = "nestMeHere.anotherNested"
	SourceConfigNestMeHereFormatThisName  = "nestMeHere.formatThisName"
)

func (SourceConfig) Parameters() map[string]config.Parameter {
	return map[string]config.Parameter{
		SourceConfigCustomType: {
			Default:     "",
			Description: "CustomType uses a custom type that is convertible to a supported type. Line comments are allowed.",
			Type:        config.ParameterTypeDuration,
			Validations: []config.Validation{},
		},
		SourceConfigGlobalDuration: {
			Default:     "1s",
			Description: "Duration does not have a name so the type name is used.",
			Type:        config.ParameterTypeDuration,
			Validations: []config.Validation{},
		},
		SourceConfigGlobalRenamed: {
			Default:     "1s",
			Description: "",
			Type:        config.ParameterTypeDuration,
			Validations: []config.Validation{},
		},
		SourceConfigGlobalWildcardStrings: {
			Default:     "foo",
			Description: "",
			Type:        config.ParameterTypeString,
			Validations: []config.Validation{
				config.ValidationRequired{},
			},
		},
		SourceConfigGlobalWildcardStructsName: {
			Default:     "",
			Description: "",
			Type:        config.ParameterTypeString,
			Validations: []config.Validation{},
		},
		SourceConfigNestMeHereAnotherNested: {
			Default:     "",
			Description: "AnotherNested is also nested under nestMeHere.\nThis is a block comment.",
			Type:        config.ParameterTypeInt,
			Validations: []config.Validation{},
		},
		SourceConfigNestMeHereFormatThisName: {
			Default:     "this is not a float",
			Description: "FORMATThisName should stay \"FORMATThisName\". Default is not a float\nbut that's not a problem, paramgen does not validate correctness.",
			Type:        config.ParameterTypeFloat,
			Validations: []config.Validation{},
		},
	}
}