// Package jsonpatch applies JSON patches to Go values using struct tag rules.

package jsonpatch

import (
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
	AllowNull       bool
	ErrorOnReadOnly bool
	SliceMode       SliceMode
	SliceKey        string
	MapMode         MapMode
}

// Merge applies patch to dst in place and returns the merged value.
//
// If patch is a []byte it is treated as raw JSON and the destination model's
// rules are used. Any other patch type is marshaled to JSON and acts as an
// allowlist of fields that may be changed, using that type's rules.
//
// The returned value is *dst after the merge, so callers can write:
//
//	user, err := jsonpatch.Merge(&user, patch)
func Merge[T any, P any](dst *T, patch P, opts ...Options) (T, error) {
	var zero T
	if dst == nil {
		return zero, errors.New("nil destination")
	}
	o := firstOpt(opts)

	original, err := json.Marshal(dst)
	if err != nil {
		return zero, fmt.Errorf("marshal dst: %w", err)
	}

	var data []byte
	var sr *structRules
	if raw, ok := any(patch).([]byte); ok {
		data = raw
		sr = rulesFromDst(dst)
	} else {
		data, err = json.Marshal(patch)
		if err != nil {
			return zero, fmt.Errorf("marshal patch: %w", err)
		}
		sr = rulesForType(reflect.TypeOf(patch))
		if sr == nil {
			return zero, errors.New("patch model has no exported JSON fields")
		}
		sr = &structRules{fields: sr.fields, restrict: true}
	}

	merged, err := mergeCore(original, data, sr, o)
	if err != nil {
		return zero, err
	}
	if err := writeMerged(dst, merged); err != nil {
		return zero, err
	}
	return *dst, nil
}

func mergeCore(original, patchData []byte, sr *structRules, o Options) (any, error) {
	var origVal, patchVal any
	if err := json.Unmarshal(original, &origVal); err != nil {
		return nil, fmt.Errorf("unmarshal original: %w", err)
	}
	if err := json.Unmarshal(patchData, &patchVal); err != nil {
		return nil, fmt.Errorf("unmarshal patch: %w", err)
	}

	return mergeValues(origVal, patchVal, sr, o)
}

func writeMerged[T any](dst *T, merged any) error {
	mergedBytes, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal merged: %w", err)
	}
	if err := json.Unmarshal(mergedBytes, dst); err != nil {
		return fmt.Errorf("unmarshal merged: %w", err)
	}
	return nil
}

func firstOpt(opts []Options) Options {
	if len(opts) > 0 {
		return opts[0]
	}
	return Options{}
}
