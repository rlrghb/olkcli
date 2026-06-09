package outfmt

import "reflect"

// conciseTagKey marks result-struct fields that --concise drops to shrink
// output: large free-text like message/event/task bodies, previews, and
// attendee lists. A tagged field must also carry json:",omitempty" so that
// zeroing it removes it from the marshaled JSON entirely.
const conciseTagKey = "concise"

// dropConcise returns a copy of v with every field tagged `concise:"omit"`
// zeroed, so JSON marshaling (with omitempty) drops it. It handles a struct, a
// pointer to a struct, or a slice of either, and never mutates its input — each
// scrubbed value is a fresh copy, so the caller's data is untouched.
func dropConcise(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() { //nolint:exhaustive // only struct/pointer/slice carry result data; all else passes through
	case reflect.Pointer:
		if rv.IsNil() {
			return v
		}
		cp := reflect.New(rv.Elem().Type())
		cp.Elem().Set(rv.Elem())
		scrubConcise(cp.Elem())
		return cp.Interface()
	case reflect.Slice:
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		reflect.Copy(out, rv)
		for i := 0; i < out.Len(); i++ {
			el := out.Index(i)
			if el.Kind() == reflect.Pointer {
				if el.IsNil() {
					continue
				}
				cp := reflect.New(el.Elem().Type())
				cp.Elem().Set(el.Elem())
				scrubConcise(cp.Elem())
				el.Set(cp)
			} else {
				scrubConcise(el)
			}
		}
		return out.Interface()
	case reflect.Struct:
		cp := reflect.New(rv.Type())
		cp.Elem().Set(rv)
		scrubConcise(cp.Elem())
		return cp.Elem().Interface()
	default:
		return v
	}
}

// scrubConcise zeroes fields tagged concise:"omit" on an addressable struct
// value. Non-struct values are left untouched.
func scrubConcise(sv reflect.Value) {
	if sv.Kind() != reflect.Struct {
		return
	}
	t := sv.Type()
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Tag.Get(conciseTagKey) != "omit" {
			continue
		}
		if f := sv.Field(i); f.CanSet() {
			f.Set(reflect.Zero(f.Type()))
		}
	}
}
