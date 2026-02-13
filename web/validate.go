package web

import (
	"reflect"
	"strings"

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

// Validate that the provided model against it's declared tags.
func Validate(val any) error {
	if err := validate.Struct(val); err != nil {
		verrors, ok := err.(validator.ValidationErrors)
		if !ok {
			return err
		}

		var fields FieldErrors
		for _, verror := range verrors {
			field := FieldError{
				Field: verror.Field(),
				Err:   customErrForTag(verror.Tag(), verror),
			}
			fields = append(fields, field)
		}
		return fields
	}

	return nil
}

// FieldError represents a single validation error for a specific field.
type FieldError struct {
	Field string `json:"field"`
	Err   string `json:"error"`
}

// FieldErrors represents a collection of field errors.
type FieldErrors []FieldError

// Error implements the error interface, returning a human-readable
// summary of all field errors.
func (fe FieldErrors) Error() string {
	parts := make([]string, len(fe))
	for i, f := range fe {
		parts[i] = f.Field + ": " + f.Err
	}
	return strings.Join(parts, "; ")
}

func customErrForTag(tag string, verror validator.FieldError) string {
	switch tag {
	case "required":
		return "This field is required"
	default:
		return verror.Translate(translator)
	}
}
