package jsonpatch

func mergeValues(dst, patch any, sr *structRules, opts Options) any {
	switch p := patch.(type) {
	case map[string]any:
		return mergeObject(dst, p, sr, opts)
	case []any:
		return mergeArray(dst, p, sr, opts)
	default:
		return p
	}
}

func mergeObject(dst any, p map[string]any, sr *structRules, opts Options) any {
	d, ok := dst.(map[string]any)
	if !ok {
		return p
	}
	// Without struct rules, MapReplace replaces the whole object.
	if sr == nil && opts.MapMode == MapReplace {
		return p
	}

	for k, pv := range p {
		var fr *fieldRule
		if sr != nil {
			fr = sr.fields[k]
			if sr.restrict && fr == nil {
				continue
			}
		}
		if fr != nil && (fr.ignore || fr.readonly) {
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
			d[k] = mergeValues(dv, pv, childSR, fOpts)
		} else {
			d[k] = pv
		}
	}
	return d
}

func mergeArray(dst any, p []any, sr *structRules, opts Options) any {
	d, ok := dst.([]any)
	if !ok {
		return p
	}
	switch opts.SliceMode {
	case SliceAppend:
		return append(d, p...)
	case SliceMergeByKey:
		return mergeSliceByKey(d, p, opts.SliceKey, sr, opts)
	default:
		return p
	}
}

func mergeSliceByKey(d, p []any, key string, sr *structRules, opts Options) []any {
	if key == "" {
		return append(d, p...)
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
			d[i] = mergeValues(existing, m, sr, opts)
		} else {
			idx[k] = len(d)
			d = append(d, el)
		}
	}
	return d
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
