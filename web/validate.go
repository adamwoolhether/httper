package web

import (
	"reflect"
	"strings"

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
	if err := validate.Struct(val); err != nil {
		verrors, ok := err.(validator.ValidationErrors)
		if !ok {
			return err
		}

		var fields errs.FieldErrors
		for _, verror := range verrors {
			field := errs.FieldError{
				Field: verror.Field(),
				Err:   customErrForTag(verror.Tag(), verror),
			}
			fields = append(fields, field)
		}
		return fields
	}

	return nil
}

func customErrForTag(tag string, verror validator.FieldError) string {
	switch tag {
	case "required":
		return "This field is required"
	default:
		return verror.Translate(translator)
	}
}
