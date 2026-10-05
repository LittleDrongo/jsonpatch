// Package partial предоставляет функции для применения JSON-патчей
// к существующим Go-значениям поверх encoding/json/v2.
//
// Поведение слияния настраивается двумя способами:
//
//  1. Через Options — глобально для всего вызова.
//  2. Через тег `partial:"..."` на полях структуры — точечно для каждого поля.
//     Теги кешируются на тип через sync.Map, поэтому рефлексия
//     выполняется один раз за всё время жизни программы.
//
// Синтаксис тега (через запятую):
//
//	partial:"-"                     — поле полностью игнорируется в патче
//	partial:"readonly"              — поле нельзя менять
//	partial:"null"                  — null из патча применяется к этому полю
//	partial:"slice=append"          — элементы патча добавляются в конец
//	partial:"slice=replace"         — срез из патча заменяет существующий
//	partial:"slice=merge,key=id"    — слияние по значению поля id
//	partial:"map=merge"             — мапа сливается по ключам
//	partial:"map=replace"           — мапа из патча заменяет существующую
//
// Поля без тега обрабатываются согласно Options.
package partial

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// ---------- Режимы ----------

// SliceMode описывает, как патч обрабатывает JSON-массивы.
type SliceMode int

const (
	// SliceReplace — срез из патча полностью заменяет существующий.
	// Значение по умолчанию.
	SliceReplace SliceMode = iota
	// SliceAppend — элементы из патча добавляются в конец существующего среза.
	SliceAppend
	// SliceMergeByKey — элементы сливаются по полю из Options.SliceKey.
	SliceMergeByKey
)

// MapMode описывает, как патч обрабатывает JSON-объекты (мапы).
type MapMode int

const (
	// MapReplace — мапа из патча полностью заменяет существующую.
	// Значение по умолчанию.
	MapReplace MapMode = iota
	// MapMerge — ключи из патча сливаются с существующими.
	MapMerge
)

// ---------- Опции ----------

// Options управляет поведением Merge-функций.
type Options struct {
	// Strict отклоняет неизвестные поля в патче.
	Strict bool

	// AllowNull разрешает применение `null` к полям.
	// По умолчанию false — `null` пропускается.
	AllowNull bool

	// SliceMode — режим слияния массивов по умолчанию.
	SliceMode SliceMode

	// SliceKey — имя поля-идентификатора для SliceMergeByKey.
	SliceKey string

	// MapMode — режим слияния объектов по умолчанию.
	MapMode MapMode

	// ImmutableFields — список ключей верхнего уровня, которые нельзя
	// менять через патч. Дополняет теги readonly на полях.
	ImmutableFields []string
}

// Change описывает одно изменение, найденное MergeWithDiff.
type Change struct {
	Path string
	Old  any
	New  any
}

// ---------- Публичные функции ----------

// Merge применяет патч (data) поверх dst.
func Merge[T any](dst *T, data []byte, opts ...Options) error {
	if dst == nil {
		return errors.New("partial: nil destination")
	}
	o := firstOpt(opts)

	original, err := json.Marshal(dst)
	if err != nil {
		return fmt.Errorf("partial: marshal dst: %w", err)
	}

	_, merged, err := mergeCore(original, data, rulesFromDst(dst), o)
	if err != nil {
		return err
	}
	return writeMerged(dst, merged, o)
}

// MergeRaw делает то же, что Merge, но принимает оригинал в виде готового JSON.
func MergeRaw[T any](dst *T, original, data []byte, opts ...Options) error {
	if dst == nil {
		return errors.New("partial: nil destination")
	}
	o := firstOpt(opts)

	_, merged, err := mergeCore(original, data, rulesFromDst(dst), o)
	if err != nil {
		return err
	}
	return writeMerged(dst, merged, o)
}

// MergeWithDiff делает то же, что Merge, но возвращает список изменений.
func MergeWithDiff[T any](dst *T, data []byte, opts ...Options) ([]Change, error) {
	if dst == nil {
		return nil, errors.New("partial: nil destination")
	}
	o := firstOpt(opts)

	original, err := json.Marshal(dst)
	if err != nil {
		return nil, fmt.Errorf("partial: marshal dst: %w", err)
	}

	origVal, merged, err := mergeCore(original, data, rulesFromDst(dst), o)
	if err != nil {
		return nil, err
	}
	if err := writeMerged(dst, merged, o); err != nil {
		return nil, err
	}
	return diffValues(origVal, merged), nil
}

// IsNull сообщает, является ли data ровно литералом JSON null.
func IsNull(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

// ---------- Ядро ----------

func mergeCore(original, patchData []byte, sr *structRules, o Options) (any, any, error) {
	var origVal, patchVal any
	if err := json.Unmarshal(original, &origVal); err != nil {
		return nil, nil, fmt.Errorf("partial: unmarshal original: %w", err)
	}
	if err := json.Unmarshal(patchData, &patchVal); err != nil {
		return nil, nil, fmt.Errorf("partial: unmarshal patch: %w", err)
	}

	if len(o.ImmutableFields) > 0 {
		if m, ok := patchVal.(map[string]any); ok {
			patchVal = dropFields(m, o.ImmutableFields)
		}
	}

	merged := mergeValues(origVal, patchVal, sr, o)
	return origVal, merged, nil
}

func writeMerged[T any](dst *T, merged any, o Options) error {
	mergedBytes, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("partial: marshal merged: %w", err)
	}

	var decOpts []json.Options
	if o.Strict {
		decOpts = append(decOpts, json.RejectUnknownMembers(true))
	}
	if err := json.Unmarshal(mergedBytes, dst, decOpts...); err != nil {
		return fmt.Errorf("partial: unmarshal merged: %w", err)
	}
	return nil
}

// mergeValues — единая точка рекурсии. sr — правила для текущего значения:
// для структуры — правила самой структуры, для среза структур — правила элемента.
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
	// Если это обычная мапа без структуры (sr == nil) и режим Replace —
	// возвращаем патч целиком.
	if sr == nil && opts.MapMode == MapReplace {
		return p
	}

	for k, pv := range p {
		var fr *fieldRule
		if sr != nil {
			fr = sr.fields[k]
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

// applyFieldOpts накладывает на переданные опции fieldRule поля.
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

// pickChildRules выбирает правила для вложенного значения исходя из его формы.
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

// ---------- Теги и кеш правил ----------

type structRules struct {
	fields map[string]*fieldRule
}

type fieldRule struct {
	ignore    bool
	readonly  bool
	allowNull bool
	sliceMode *SliceMode
	sliceKey  string
	mapMode   *MapMode
	nested    *structRules // для структуры и map[string]struct
	elem      *structRules // для []struct
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

		fr := parseFieldTag(f.Tag.Get("partial"))

		// Подтягиваем правила вложенных типов независимо от того,
		// есть ли тег на самом поле.
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

// jsonNameOf возвращает имя поля в JSON и признак «поле исключено».
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

// ---------- Вспомогательные ----------

func dropFields(m map[string]any, fields []string) map[string]any {
	skip := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		skip[f] = struct{}{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if _, found := skip[k]; found {
			continue
		}
		out[k] = v
	}
	return out
}

func diffValues(old, new any) []Change {
	var out []Change
	diffRec(old, new, "", &out)
	return out
}

func diffRec(old, new any, path string, out *[]Change) {
	switch n := new.(type) {
	case map[string]any:
		o, _ := old.(map[string]any)
		for k, nv := range n {
			child := k
			if path != "" {
				child = path + "." + k
			}
			if o != nil {
				if ov, ok := o[k]; ok {
					diffRec(ov, nv, child, out)
					continue
				}
			}
			*out = append(*out, Change{Path: child, New: nv})
		}
	default:
		if !reflect.DeepEqual(old, new) {
			*out = append(*out, Change{Path: path, Old: old, New: new})
		}
	}
}

func firstOpt(opts []Options) Options {
	if len(opts) > 0 {
		return opts[0]
	}
	return Options{}
}
