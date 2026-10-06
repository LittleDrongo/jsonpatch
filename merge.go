package jsonpatch

import "fmt"

func mergeValues(dst, patch any, sr *structRules, opts Options) (any, error) {
	switch p := patch.(type) {
	case map[string]any:
		return mergeObject(dst, p, sr, opts)
	case []any:
		return mergeArray(dst, p, sr, opts)
	default:
		return p, nil
	}
}

func mergeObject(dst any, p map[string]any, sr *structRules, opts Options) (any, error) {
	d, ok := dst.(map[string]any)
	if !ok {
		return p, nil
	}
	// Without struct rules, MapReplace replaces the whole object.
	if sr == nil && opts.MapMode == MapReplace {
		return p, nil
	}

	for k, pv := range p {
		var fr *fieldRule
		if sr != nil {
			fr = sr.fields[k]
			if sr.restrict && fr == nil {
				continue
			}
		}
		if fr != nil && fr.ignore {
			continue
		}
		if fr != nil && fr.readonly {
			if opts.ErrorOnReadOnly {
				return nil, fmt.Errorf("field %q is read-only", k)
			}
			continue
		}

		if pv == nil {
			allowNull := opts.AllowNull || (fr != nil && fr.allowNull)
			if !allowNull {
				continue
			}
			d[k] = nil
			continue
		}

		fOpts := applyFieldOpts(opts, fr)
		childSR := pickChildRules(fr, pv)

		if dv, exists := d[k]; exists {
			merged, err := mergeValues(dv, pv, childSR, fOpts)
			if err != nil {
				return nil, err
			}
			d[k] = merged
		} else {
			d[k] = pv
		}
	}
	return d, nil
}

func mergeArray(dst any, p []any, sr *structRules, opts Options) (any, error) {
	d, ok := dst.([]any)
	if !ok {
		return p, nil
	}
	switch opts.SliceMode {
	case SliceAppend:
		return append(d, p...), nil
	case SliceMergeByKey:
		return mergeSliceByKey(d, p, opts.SliceKey, sr, opts)
	default:
		return p, nil
	}
}

func mergeSliceByKey(d, p []any, key string, sr *structRules, opts Options) ([]any, error) {
	if key == "" {
		return append(d, p...), nil
	}

	idx := make(map[any]int, len(d))
	for i, el := range d {
		if m, ok := el.(map[string]any); ok {
			if k, ok := m[key]; ok {
				idx[k] = i
			}
		}
	}

	for _, el := range p {
		m, ok := el.(map[string]any)
		if !ok {
			d = append(d, el)
			continue
		}
		k, ok := m[key]
		if !ok {
			d = append(d, el)
			continue
		}
		if i, exists := idx[k]; exists {
			existing, _ := d[i].(map[string]any)
			if existing == nil {
				d[i] = m
				continue
			}
			merged, err := mergeValues(existing, m, sr, opts)
			if err != nil {
				return nil, err
			}
			d[i] = merged
		} else {
			idx[k] = len(d)
			d = append(d, el)
		}
	}
	return d, nil
}

func applyFieldOpts(o Options, fr *fieldRule) Options {
	if fr == nil {
		return o
	}
	if fr.sliceMode != nil {
		o.SliceMode = *fr.sliceMode
	}
	if fr.sliceKey != "" {
		o.SliceKey = fr.sliceKey
	}
	if fr.mapMode != nil {
		o.MapMode = *fr.mapMode
	}
	return o
}

func pickChildRules(fr *fieldRule, pv any) *structRules {
	if fr == nil {
		return nil
	}
	switch pv.(type) {
	case []any:
		return fr.elem
	case map[string]any:
		return fr.nested
	default:
		return nil
	}
}
