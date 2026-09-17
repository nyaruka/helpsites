package utils

import (
	"crypto/subtle"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// Validate validates the given struct against its validate tags
func Validate(obj any) error {
	return validate.Struct(obj)
}

// SecretEqual compares two secrets in constant time
func SecretEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
