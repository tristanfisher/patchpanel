package patchpanel

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// KeyValueSeparator is used to split key/value combinations on a given tag
// e.g. for `entries:"a:b·c:d"`, KeyValueSeparator is used to create:
//
//	{
//	  "a": "b",
//	  "c": "d"
//	}
//
// with TokenSeparator set to ·
// The default is a colon due to common usage.
//
// Reminder that multiple hints can be provided for parsing, such as
// `ports:"http:80·https:443" allowed_ciphers:"1.2,1.3"`
//
// Use of key/value is optional and provided for custom parsers.
const KeyValueSeparator = `:`

// TokenSeparator separates entries inside a given tag,
// typically used in conjunction with KeyValueSeparator
// The default is a middot due to low likelihood of collision.
const TokenSeparator = `·`

var timeFormatMap = map[string]string{
	"Layout":      time.Layout,
	"ANSIC":       time.ANSIC,
	"UnixDate":    time.UnixDate,
	"RubyDate":    time.RubyDate,
	"RFC822":      time.RFC822,
	"RFC822Z":     time.RFC822Z,
	"RFC850":      time.RFC850,
	"RFC1123":     time.RFC1123,
	"RFC1123Z":    time.RFC1123Z,
	"RFC3339":     time.RFC3339,
	"RFC3339Nano": time.RFC3339Nano,
	"Kitchen":     time.Kitchen,
	"Stamp":       time.Stamp,
	"StampMilli":  time.StampMilli,
	"StampMicro":  time.StampMicro,
	"StampNano":   time.StampNano,
	"DateTime":    time.DateTime,
	"DateOnly":    time.DateOnly,
	"TimeOnly":    time.TimeOnly,
}

// Parser handles parsing a string input based on an arbitrary function body.  parserHints are provided at call time
// for interpretation within a given function (see time.Time's parser in NewPatchPanel for an example implementation)
type Parser func(value string, parserHints map[string]any) (any, error)

// KindParser is like Parser but also receives the target reflect.Type, enabling it to handle
// all types matching a given reflect.Kind (e.g. all struct types).  This is necessary because
// a single KindParser must be able to construct a zero value of any type of that kind.
type KindParser func(value string, toType reflect.Type, parserHints map[string]any) (any, error)

type PatchPanel struct {
	tokenSeparator    string
	keyValueSeparator string
	parsers           map[reflect.Type]Parser
	kindParsers       map[reflect.Kind]KindParser
	sync.RWMutex
}

// NewPatchPanel instantiates a PatchPanel
func NewPatchPanel(tokenSeparator string, keyValueSeparator string) *PatchPanel {
	pc := &PatchPanel{
		tokenSeparator:    tokenSeparator,
		keyValueSeparator: keyValueSeparator,
		// Parsers are looked up via reflect.Types instead of "standard" types as the pipeline starts at
		// StructField.Types.  Using reflect.Type vs specific reflect.Kind allows for arbitrary user
		// types to be added (reflect.TypeOf(Foo) vs being restricted to reflect.Kind).
		//
		// note that parser hints are per field
		parsers: map[reflect.Type]Parser{
			// str
			reflect.TypeOf(""): func(v string, parserHints map[string]any) (any, error) {
				return v, nil
			},

			// bool
			reflect.TypeOf(true): func(v string, parserHints map[string]any) (any, error) {
				if v == "" {
					return nil, NoValueError{Msg: "bool"}
				}
				return strconv.ParseBool(v)
			},

			// int
			reflect.TypeOf(0): func(v string, parserHints map[string]any) (any, error) {
				if v == "" {
					return nil, NoValueError{Msg: "int"}
				}
				return strconv.Atoi(v)
			},

			// time.Duration
			reflect.TypeOf(time.Duration(0)): func(v string, parserHints map[string]any) (any, error) {
				if v == "" {
					return nil, NoValueError{Msg: "time.Duration"}
				}
				val, err := time.ParseDuration(v)
				if err != nil {
					return time.Duration(0), err
				}
				return val, nil
			},

			// time.Time
			reflect.TypeOf(time.Time{}): func(v string, parserHints map[string]any) (any, error) {
				if v == "" {
					return nil, NoValueError{Msg: "time.Time"}
				}
				// timeFormatString is required as the go compiler
				// cannot infer that timeFormat will invariably become a string
				var timeFormatString string
				// did the user request a time format?
				timeFormatHint, ok := parserHints["timeFormat"]
				// if we haven't been told how to parse this time, try RFC 3339
				if !ok {
					timeFormatString = time.RFC3339
				} else {
					// any->str
					tFormatHintStr, ok := timeFormatHint.(string)
					if !ok {
						return time.Time{}, errors.New("timeFormat parser hint must be a string")
					}
					// do we have a `time` package const that corresponds with the request string?
					timeFormatString, ok = timeFormatMap[tFormatHintStr]
					if !ok {
						return time.Time{}, errors.New("unknown timeFormat provided")
					}
				}
				val, err := time.Parse(timeFormatString, v)
				if err != nil {
					return time.Time{}, err
				}
				return val, nil
			},
		},
		// kindParsers are a fallback for when no exact reflect.Type parser is registered.
		// Type-specific parsers in parsers always take precedence.
		kindParsers: map[reflect.Kind]KindParser{
			// Struct fields with no registered type parser are handled by returning a zero
			// value of the target type when a non-empty string is provided, or NoValueError
			// when the tag value is empty (no default set).  Override via AddKindParser to
			// support custom serialisation formats (e.g. JSON) for embedded struct defaults.
			reflect.Struct: func(value string, toType reflect.Type, parserHints map[string]any) (any, error) {
				if value == "" {
					return nil, NoValueError{Msg: toType.Name()}
				}
				return reflect.New(toType).Elem().Interface(), nil
			},
		},
	}
	return pc
}

// AddParser adds a parser configuration.  The ability to overwrite is intentional.
func (pc *PatchPanel) AddParser(typ reflect.Type, parser Parser) {
	pc.Lock()
	defer pc.Unlock()
	pc.parsers[typ] = parser
}

// AddKindParser adds a KindParser that is used as a fallback for all types matching the given
// reflect.Kind when no exact reflect.Type parser is registered.  The ability to overwrite is intentional.
func (pc *PatchPanel) AddKindParser(kind reflect.Kind, parser KindParser) {
	pc.Lock()
	defer pc.Unlock()
	pc.kindParsers[kind] = parser
}

// ToReflectType is a shallow wrapper around reflect.TypeOf, placed in this library for reasons of code-flow
// This library operates on types that are understood by the `reflect` library
func ToReflectType(input any) reflect.Type {
	return reflect.TypeOf(input)
}

// FieldNameById is a convenience func to get named field off of a struct.
// This is useful for being able to loop over a struct object.
// Field() lookups on structs use zero-based counting
func FieldNameById(obj reflect.Type, idx int) string {
	if obj == nil {
		return ""
	}
	return obj.Field(idx).Name
}

func parseHints(sF reflect.StructField, hints []string) map[string]any {

	// if we have parser hints, cleanup and split into a map
	// parser hints likely originate from a tag
	parserHintTable := make(map[string]any, len(hints))

	for _, hintTag := range hints {
		tagValue := sF.Tag.Get(hintTag)
		tagValue = strings.TrimSpace(tagValue)
		parserHintTable[hintTag] = strings.TrimSpace(tagValue)
	}

	return parserHintTable
}

// coerce converts an input to a desired destination type specified by toType
// We expect our input value, v, to be a string as we expect to be handling struct tags
// parserHints are optional and come in as a string from a tag name
func (pc *PatchPanel) coerce(v string, toType reflect.Type, parserHints map[string]any) (any, error) {
	pc.RLock()
	parserFunc, typeOk := pc.parsers[toType]
	kindParserFunc, kindOk := pc.kindParsers[toType.Kind()]
	pc.RUnlock()

	if typeOk {
		val, err := parserFunc(v, parserHints)
		if err != nil {
			return val, err
		}
		return val, nil
	}

	if kindOk {
		val, err := kindParserFunc(v, toType, parserHints)
		if err != nil {
			return val, err
		}
		return val, nil
	}

	return nil, UnhandledParserTypeError{Msg: fmt.Sprintf("unknown type for parser: %v", toType)}
}

// GetFieldTag loads a tag off of a given field in a struct.
// In an example struct of { A int `x:"y"` }, the fieldName is A, the tagName is x.
//
// The struct field, the tag value, and any error is returned.
func (pc *PatchPanel) GetFieldTag(fieldName string, tagName string, t reflect.Type, parserHints []string) (reflect.StructField, any, error) {

	if t == nil {
		return reflect.StructField{}, nil, errors.New("nil type provided")
	}

	// panic: reflect: FieldByName of non-struct type... guard
	if t.Kind() != reflect.Struct {
		return reflect.StructField{}, nil, fmt.Errorf("expected struct type, got %s", t.Kind().String())
	}

	sF, ok := t.FieldByName(fieldName)
	// if no such value, we intentionally return a string
	if !ok {
		return sF, nil, NoFieldError{Msg: "no such field name: " + fieldName}
	}

	// Note that tags are always strings, which then need to be converted to desired types (if applicable).
	val, err := pc.coerce(sF.Tag.Get(tagName), sF.Type, parseHints(sF, parserHints))
	if err != nil {
		// while we failed coercion, we were able to partially parse the struct field
		// return details to aid debugging
		return reflect.StructField{
			Name: fieldName,
			Type: sF.Type,
			Tag:  sF.Tag,
		}, val, err
	}

	return sF, val, nil
}

// GetDefault retrieves the field tag called 'default' and extracts the value
func (pc *PatchPanel) GetDefault(fieldName string, t reflect.Type, parserHints []string) (any, error) {

	var i any
	_, fieldValue, err := pc.GetFieldTag(fieldName, "default", t, parserHints)
	if err != nil {
		return i, err
	}

	// return a custom error to allow caller to decide how to handle no value
	if fieldValue == "" {
		return i, NoValueError{Msg: fieldName}
	}

	return fieldValue, nil
}
