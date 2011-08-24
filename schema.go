package schema

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// The Coerce method of the Checker interface is called recursively when
// v is being validated.  If err is nil, newv is used as the new value
// at the recursion point.  If err is non-nil, v is taken as invalid and
// may be either ignored or error out depending on where in the schema
// checking process the error happened. Checkers like OneOf may continue
// with an alternative, for instance.
type Checker interface {
	Coerce(v interface{}, path []string) (newv interface{}, err os.Error)
}

type error struct {
	want string
	got interface{}
	path    []string
}

func (e error) String() string {
	var path string
	if e.path[0] == "." {
		path = strings.Join(e.path[1:], "")
	} else {
		path = strings.Join(e.path, "")
	}
	if e.want == "" {
		return fmt.Sprintf("%s: unsupported value", path)
	}
	if e.got == nil {
		return fmt.Sprintf("%s: expected %s, got nothing", path, e.want)
	}
	return fmt.Sprintf("%s: expected %s, got %#v", path, e.want, e.got)
}

// Any returns a Checker that succeeds with any input value and
// results in the value itself unprocessed.
func Any() Checker {
	return anyC{}
}

type anyC struct{}

func (c anyC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	return v, nil
}


// Const returns a Checker that only succeeds if the input matches
// value exactly.  The value is compared with reflect.DeepEqual.
func Const(value interface{}) Checker {
	return constC{value}
}

type constC struct {
	value interface{}
}

func (c constC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	if reflect.DeepEqual(v, c.value) {
		return v, nil
	}
	return nil, error{fmt.Sprintf("%#v", c.value), v, path}
}

// OneOf returns a Checker that attempts to Coerce the value with each
// of the provided checkers. The value returned by the first checker
// that succeeds will be returned by the OneOf checker itself.  If no
// checker succeeds, OneOf will return an error on coercion.
func OneOf(options ...Checker) Checker {
	return oneOfC{options}
}

type oneOfC struct {
	options []Checker
}

func (c oneOfC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	for _, o := range c.options {
		newv, err := o.Coerce(v, path)
		if err == nil {
			return newv, nil
		}
	}
	return nil, error{path: path}
}

// Bool returns a Checker that accepts boolean values only.
func Bool() Checker {
	return boolC{}
}

type boolC struct{}

func (c boolC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	if reflect.TypeOf(v).Kind() == reflect.Bool {
		return v, nil
	}
	return nil, error{"bool", v, path}
}

// Int returns a Checker that accepts any integer value, and returns
// the same value typed as an int64.
func Int() Checker {
	return intC{}
}

type intC struct{}

func (c intC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	switch reflect.TypeOf(v).Kind() {
	case reflect.Int:
	case reflect.Int8:
	case reflect.Int16:
	case reflect.Int32:
	case reflect.Int64:
	default:
		return nil, error{"int", v, path}
	}
	return reflect.ValueOf(v).Int(), nil
}

// Int returns a Checker that accepts any float value, and returns
// the same value typed as a float64.
func Float() Checker {
	return floatC{}
}

type floatC struct{}

func (c floatC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	switch reflect.TypeOf(v).Kind() {
	case reflect.Float32:
	case reflect.Float64:
	default:
		return nil, error{"float", v, path}
	}
	return reflect.ValueOf(v).Float(), nil
}


// String returns a Checker that accepts a string value only and returns
// it unprocessed.
func String() Checker {
	return stringC{}
}

type stringC struct{}

func (c stringC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	if reflect.TypeOf(v).Kind() == reflect.String {
		return reflect.ValueOf(v).String(), nil
	}
	return nil, error{"string", v, path}
}

func SimpleRegexp() Checker {
	return sregexpC{}
}

type sregexpC struct{}

func (c sregexpC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	// XXX The regexp package happens to be extremely simple right now.
	//     Once exp/regexp goes mainstream, we'll have to update this
	//     logic to use a more widely accepted regexp subset.
	if reflect.TypeOf(v).Kind() == reflect.String {
		s := reflect.ValueOf(v).String()
		_, err := regexp.Compile(s)
		if err != nil {
			return nil, error{"valid regexp", s, path}
		}
		return v, nil
	}
	return nil, error{"regexp string", v, path}
}

// String returns a Checker that accepts a slice value with values
// that are processed with the elem checker.  If any element of the
// provided slice value fails to be processed, processing will stop
// and return with the obtained error.
func List(elem Checker) Checker {
	return listC{elem}
}

type listC struct {
	elem Checker
}

func (c listC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, error{"list", v, path}
	}

	path = append(path, "[", "?", "]")

	l := rv.Len()
	out := make([]interface{}, 0, l)
	for i := 0; i != l; i++ {
		path[len(path)-2] = strconv.Itoa(i)
		elem, err := c.elem.Coerce(rv.Index(i).Interface(), path)
		if err != nil {
			return nil, err
		}
		out = append(out, elem)
	}
	return out, nil
}

// Map returns a Checker that accepts a map value. Every key and value
// in the map are processed with the respective checker, and if any
// value fails to be coerced, processing stops and returns with the
// underlying error.
func Map(key Checker, value Checker) Checker {
	return mapC{key, value}
}

type mapC struct {
	key Checker
	value Checker
}

func (c mapC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, error{"map", v, path}
	}

	vpath := append(path, ".", "?")

	l := rv.Len()
	out := make(map[interface{}]interface{}, l)
	keys := rv.MapKeys()
	for i := 0; i != l; i++ {
		k := keys[i]
		newk, err := c.key.Coerce(k.Interface(), path)
		if err != nil {
			return nil, err
		}
		vpath[len(vpath)-1] = fmt.Sprint(k.Interface())
		newv, err := c.value.Coerce(rv.MapIndex(k).Interface(), vpath)
		if err != nil {
			return nil, err
		}
		out[newk] = newv
	}
	return out, nil
}

type Fields map[string]Checker
type Optional []string

// FieldMap returns a Checker that accepts a map value with defined
// string keys. Every key has an independent checker associated,
// and processing will only succeed if all the values succeed
// individually. If a field fails to be processed, processing stops
// and returns with the underlying error.
func FieldMap(fields Fields, optional Optional) Checker {
	return fieldMapC{fields, optional}
}

type fieldMapC struct {
	fields Fields
	optional []string
}

func (c fieldMapC) isOptional(key string) bool {
	for _, k := range c.optional {
		if k == key {
			return true
		}
	}
	return false
}

func (c fieldMapC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, error{"map", v, path}
	}

	vpath := append(path, ".", "?")

	l := rv.Len()
	out := make(map[string]interface{}, l)
	for k, checker := range c.fields {
		vpath[len(vpath)-1] = k
		var value interface{}
		valuev := rv.MapIndex(reflect.ValueOf(k))
		if valuev.IsValid() {
			value = valuev.Interface()
		} else if c.isOptional(k) {
			continue
		}
		newv, err := checker.Coerce(value, vpath)
		if err != nil {
			return nil, err
		}
		out[k] = newv
	}
	return out, nil
}

// FieldMapSet returns a Checker that accepts a map value checked
// against one of several FieldMap checkers.  The actual checker
// used is the first one whose checker associated with the selector
// field processes the map correctly. If no checker processes
// the selector value correctly, an error is returned.
func FieldMapSet(selector string, maps []Checker) Checker {
	fmaps := make([]fieldMapC, len(maps))
	for i, m := range maps {
		if fmap, ok := m.(fieldMapC); ok {
			if checker, _ := fmap.fields[selector]; checker == nil {
				panic("FieldMapSet has a FieldMap with a missing selector")
			}
			fmaps[i] = fmap
		} else {
			panic("FieldMapSet got a non-FieldMap checker")
		}
	}
	return mapSetC{selector, fmaps}
}

type mapSetC struct {
	selector string
	fmaps []fieldMapC
}

func (c mapSetC) Coerce(v interface{}, path []string) (interface{}, os.Error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, error{"map", v, path}
	}

	var selector interface{}
	selectorv := rv.MapIndex(reflect.ValueOf(c.selector))
	if selectorv.IsValid() {
		selector = selectorv.Interface()
		for _, fmap := range c.fmaps {
			_, err := fmap.fields[c.selector].Coerce(selector, path)
			if err != nil {
				continue
			}
			return fmap.Coerce(v, path)
		}
	}
	return nil, error{"supported selector", selector, append(path, ".", c.selector)}
}