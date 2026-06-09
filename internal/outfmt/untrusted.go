package outfmt

import "reflect"

// Untrusted-content markers. Fields carrying externally-controlled free text
// (email bodies, subjects, sender names, file names, …) are wrapped with these
// so an LLM/agent consuming `--json` output can be instructed to treat the
// enclosed span as data, never as instructions (prompt-injection defense).
const (
	UntrustedOpen  = "‹untrusted›"  // ‹untrusted›
	UntrustedClose = "‹/untrusted›" // ‹/untrusted›
)

// wrapMarker wraps a non-empty string in untrusted-content markers. Empty
// strings are left untouched so absent fields stay clean.
func wrapMarker(s string) string {
	if s == "" {
		return s
	}
	return UntrustedOpen + s + UntrustedClose
}

// wrapUntrusted returns a deep copy of v in which every field tagged
// `untrusted:"true"` has its string content wrapped with markers. The original
// is never mutated. Non-struct/slice/pointer values pass through unchanged.
func wrapUntrusted(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	return transformUntrusted(rv).Interface()
}

// transformUntrusted copies v, wrapping the strings of any `untrusted:"true"`
// field it finds (recursing through structs, pointers and slices for nested
// tagged fields).
func transformUntrusted(v reflect.Value) reflect.Value {
	switch v.Kind() { //nolint:exhaustive // composite kinds are handled; default passes scalars through
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		nv := reflect.New(v.Elem().Type())
		nv.Elem().Set(transformUntrusted(v.Elem()))
		return nv
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		return transformUntrusted(v.Elem())
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		nv := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			nv.Index(i).Set(transformUntrusted(v.Index(i)))
		}
		return nv
	case reflect.Struct:
		nv := reflect.New(v.Type()).Elem()
		nv.Set(v)
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" { // unexported
				continue
			}
			fv := nv.Field(i)
			if f.Tag.Get("untrusted") == "true" {
				fv.Set(wrapStrings(fv))
				continue
			}
			if fk := fv.Kind(); fk == reflect.Pointer || fk == reflect.Slice || fk == reflect.Struct || fk == reflect.Interface {
				fv.Set(transformUntrusted(fv))
			}
		}
		return nv
	default:
		return v
	}
}

// wrapStrings returns a copy of v with every string leaf wrapped in markers.
// Used for a field explicitly tagged untrusted (which may be a string, a
// []string, or a nested struct of strings).
func wrapStrings(v reflect.Value) reflect.Value {
	switch v.Kind() { //nolint:exhaustive // string/composite kinds are wrapped; default passes the rest through
	case reflect.String:
		nv := reflect.New(v.Type()).Elem()
		nv.SetString(wrapMarker(v.String()))
		return nv
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		nv := reflect.New(v.Elem().Type())
		nv.Elem().Set(wrapStrings(v.Elem()))
		return nv
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		nv := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			nv.Index(i).Set(wrapStrings(v.Index(i)))
		}
		return nv
	case reflect.Struct:
		nv := reflect.New(v.Type()).Elem()
		nv.Set(v)
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).PkgPath != "" {
				continue
			}
			nv.Field(i).Set(wrapStrings(nv.Field(i)))
		}
		return nv
	default:
		return v
	}
}
