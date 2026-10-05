package jsonpatch

import (
	"reflect"
	"strings"
	"sync"
)

type structRules struct {
	fields   map[string]*fieldRule
	restrict bool
}

type fieldRule struct {
	ignore    bool
	readonly  bool
	allowNull bool
	sliceMode *SliceMode
	sliceKey  string
	mapMode   *MapMode
	nested    *structRules
	elem      *structRules
}

var rulesCache sync.Map // reflect.Type -> *structRules

func rulesFromDst[T any](dst *T) *structRules {
	return rulesForType(reflect.TypeOf(dst).Elem())
}

func rulesForType(t reflect.Type) *structRules {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	if v, ok := rulesCache.Load(t); ok {
		sr := v.(*structRules)
		if len(sr.fields) == 0 {
			return nil
		}
		return sr
	}
	sr := buildStructRules(t)
	if sr == nil {
		sr = &structRules{}
	}
	rulesCache.Store(t, sr)
	if len(sr.fields) == 0 {
		return nil
	}
	return sr
}

func buildStructRules(t reflect.Type) *structRules {
	sr := &structRules{fields: make(map[string]*fieldRule)}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Anonymous {
			if emb := rulesForType(f.Type); emb != nil {
				for k, v := range emb.fields {
					sr.fields[k] = v
				}
			}
			continue
		}

		jsonName, jsonIgnored := jsonNameOf(f)
		if jsonIgnored || jsonName == "" {
			continue
		}

		fr := parseFieldTag(f.Tag.Get("jsonpatch"))

		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		switch ft.Kind() {
		case reflect.Struct:
			if nested := rulesForType(ft); nested != nil {
				if fr == nil {
					fr = &fieldRule{}
				}
				fr.nested = nested
			}
		case reflect.Slice, reflect.Array:
			et := ft.Elem()
			for et.Kind() == reflect.Ptr {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct {
				if nested := rulesForType(et); nested != nil {
					if fr == nil {
						fr = &fieldRule{}
					}
					fr.elem = nested
				}
			}
		case reflect.Map:
			et := ft.Elem()
			for et.Kind() == reflect.Ptr {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct {
				if nested := rulesForType(et); nested != nil {
					if fr == nil {
						fr = &fieldRule{}
					}
					fr.nested = nested
				}
			}
		}

		if fr != nil {
			sr.fields[jsonName] = fr
		} else {
			sr.fields[jsonName] = &fieldRule{}
		}
	}
	if len(sr.fields) == 0 {
		return nil
	}
	return sr
}

func parseFieldTag(tag string) *fieldRule {
	if tag == "" {
		return nil
	}
	fr := &fieldRule{}
	any := false

	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch {
		case part == "-":
			fr.ignore = true
			any = true
		case part == "readonly":
			fr.readonly = true
			any = true
		case part == "null":
			fr.allowNull = true
			any = true
		case strings.HasPrefix(part, "slice="):
			m, ok := parseSliceMode(strings.TrimPrefix(part, "slice="))
			if !ok {
				continue
			}
			fr.sliceMode = &m
			any = true
		case strings.HasPrefix(part, "key="):
			fr.sliceKey = strings.TrimPrefix(part, "key=")
			any = true
		case strings.HasPrefix(part, "map="):
			m, ok := parseMapMode(strings.TrimPrefix(part, "map="))
			if !ok {
				continue
			}
			fr.mapMode = &m
			any = true
		}
	}
	if !any {
		return nil
	}
	return fr
}

func parseSliceMode(s string) (SliceMode, bool) {
	switch s {
	case "replace":
		return SliceReplace, true
	case "append":
		return SliceAppend, true
	case "merge":
		return SliceMergeByKey, true
	default:
		return 0, false
	}
}

func parseMapMode(s string) (MapMode, bool) {
	switch s {
	case "replace":
		return MapReplace, true
	case "merge":
		return MapMerge, true
	default:
		return 0, false
	}
}

func jsonNameOf(f reflect.StructField) (string, bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", true
	}
	if tag == "" {
		return f.Name, false
	}
	name := strings.Split(tag, ",")[0]
	if name == "-" {
		return "", true
	}
	if name == "" {
		return f.Name, false
	}
	return name, false
}
