package validator

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

// Philippine TIN format: XXX-XXX-XXX-XXX or XXXXXXXXXXXX
var tinRegex = regexp.MustCompile(`^\d{3}-?\d{3}-?\d{3}-?\d{3}$`)

func ValidateTIN(fl validator.FieldLevel) bool {
	return tinRegex.MatchString(fl.Field().String())
}

func RegisterCustomValidators(v *validator.Validate) {
	_ = v.RegisterValidation("tin", ValidateTIN)
}
