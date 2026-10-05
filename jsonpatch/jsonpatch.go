// Package jsonpatch applies JSON patches to Go values using struct tag rules.

package jsonpatch

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
)

type SliceMode int

const (
	SliceReplace SliceMode = iota
	SliceAppend
	SliceMergeByKey
)

type MapMode int

const (
	MapReplace MapMode = iota
	MapMerge
)

type Options struct {
	Strict          bool
	AllowNull       bool
	SliceMode       SliceMode
	SliceKey        string
	MapMode         MapMode
	ImmutableFields []string
}

func Merge[T any](dst *T, data []byte, opts ...Options) error {
	if dst == nil {
		return errors.New("jsonpatch: nil destination")
	}
	o := firstOpt(opts)

	original, err := json.Marshal(dst)
	if err != nil {
		return fmt.Errorf("jsonpatch: marshal dst: %w", err)
	}

	merged, err := mergeCore(original, data, rulesFromDst(dst), o)
	if err != nil {
		return err
	}
	return writeMerged(dst, merged, o)
}

// MergeFrom accepts either a struct patch or raw JSON bytes.
// A struct patch also acts as an allowlist of fields that may be changed.
func MergeFrom[T any, P any](full *T, patch P, opts ...Options) error {
	if full == nil {
		return errors.New("jsonpatch: nil destination")
	}
	o := firstOpt(opts)
	original, err := json.Marshal(full)
	if err != nil {
		return fmt.Errorf("jsonpatch: marshal dst: %w", err)
	}

	var data []byte
	var ruleSet *structRules
	if raw, ok := any(patch).([]byte); ok {
		data = raw
		ruleSet = rulesFromDst(full)
	} else {
		data, err = json.Marshal(patch)
		if err != nil {
			return fmt.Errorf("jsonpatch: marshal patch: %w", err)
		}
		ruleSet = rulesForType(reflect.TypeOf(patch))
	}
	if ruleSet == nil {
		return errors.New("jsonpatch: patch model has no exported JSON fields")
	}
	limited := &structRules{fields: ruleSet.fields}
	if _, ok := any(patch).([]byte); !ok {
		limited.restrict = true
	}
	merged, err := mergeCore(original, data, limited, o)
	if err != nil {
		return err
	}
	return writeMerged(full, merged, o)
}

func MergeRaw[T any](dst *T, original, data []byte, opts ...Options) error {
	if dst == nil {
		return errors.New("jsonpatch: nil destination")
	}
	o := firstOpt(opts)

	merged, err := mergeCore(original, data, rulesFromDst(dst), o)
	if err != nil {
		return err
	}
	return writeMerged(dst, merged, o)
}

func IsNull(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

func mergeCore(original, patchData []byte, sr *structRules, o Options) (any, error) {
	var origVal, patchVal any
	if err := json.Unmarshal(original, &origVal); err != nil {
		return nil, fmt.Errorf("jsonpatch: unmarshal original: %w", err)
	}
	if err := json.Unmarshal(patchData, &patchVal); err != nil {
		return nil, fmt.Errorf("jsonpatch: unmarshal patch: %w", err)
	}

	if len(o.ImmutableFields) > 0 {
		if m, ok := patchVal.(map[string]any); ok {
			patchVal = dropFields(m, o.ImmutableFields)
		}
	}

	merged := mergeValues(origVal, patchVal, sr, o)
	return merged, nil
}

func writeMerged[T any](dst *T, merged any, o Options) error {
	mergedBytes, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("jsonpatch: marshal merged: %w", err)
	}

	var decOpts []json.Options
	if o.Strict {
		decOpts = append(decOpts, json.RejectUnknownMembers(true))
	}
	if err := json.Unmarshal(mergedBytes, dst, decOpts...); err != nil {
		return fmt.Errorf("jsonpatch: unmarshal merged: %w", err)
	}
	return nil
}

func dropFields(m map[string]any, fields []string) map[string]any {
	skip := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		skip[field] = struct{}{}
	}
	out := make(map[string]any, len(m))
	for key, value := range m {
		if _, found := skip[key]; !found {
			out[key] = value
		}
	}
	return out
}

func firstOpt(opts []Options) Options {
	if len(opts) > 0 {
		return opts[0]
	}
	return Options{}
}
