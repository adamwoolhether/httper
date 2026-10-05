package web

import (
	"reflect"
	"strings"
	"time"

	"github.com/adamwoolhether/httper/web/errs"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
)

var validate *validator.Validate
var translator ut.Translator

func init() {
	validate = validator.New()
	var ok bool
	translator, ok = ut.New(en.New(), en.New()).GetTranslator("en")
	if !ok {
		panic("web: failed to get 'en' translator")
	}

	if err := en_translations.RegisterDefaultTranslations(validate, translator); err != nil {
		panic(err)
	}

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}

		return name
	})
}

// Validate that the provided model against its declared tags.
func Validate(val any) error {
	return fieldErrors(validate.Struct(val), validator.FieldError.Field)
}

// validateDecoded validates a decoded struct, or each struct in a decoded
// slice, and skips every other type. A nil pointer validates as the zero
// value, so a JSON null fails required fields for *T the same way as for T.
func validateDecoded(val any) error {
	v := reflect.ValueOf(val)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v = reflect.Zero(v.Type().Elem())
			continue
		}
		v = v.Elem()
	}

	switch {
	case isModel(v.Type()):
		return Validate(v.Interface())
	case v.Kind() == reflect.Slice && isModel(v.Type().Elem()):
		return fieldErrors(validate.Var(v.Interface(), "dive,required"), elementField)
	}

	return nil
}

// isModel reports whether t, after pointer indirection, is a struct that the
// validator accepts. The validator rejects time.Time and types convertible to it.
func isModel(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Kind() == reflect.Struct && !t.ConvertibleTo(reflect.TypeFor[time.Time]())
}

// elementField names a slice element's field error by the element index and
// the field name that Validate uses, such as "[1].name". A nil element is "[1]".
func elementField(fe validator.FieldError) string {
	index, _, found := strings.Cut(fe.Namespace(), ".")
	if !found {
		return index
	}

	return index + "." + fe.Field()
}

func fieldErrors(err error, name func(validator.FieldError) string) error {
	verrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return err
	}

	var fields errs.FieldErrors
	for _, verror := range verrors {
		field := errs.FieldError{
			Field: name(verror),
			Err:   customErrForTag(verror.Tag(), verror),
		}
		fields = append(fields, field)
	}

	return fields
}

func customErrForTag(tag string, verror validator.FieldError) string {
	switch tag {
	case "required":
		return "This field is required"
	default:
		return verror.Translate(translator)
	}
}
