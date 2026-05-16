package profile

import "errors"

var (
	ErrInvalidConfig           = errors.New("invalid profile config")
	ErrCustomProfileNotMarked  = errors.New("custom profile requires explicit config marker")
	ErrUnknownCapability       = errors.New("unknown capability")
	ErrUnknownAuthoritySurface = errors.New("unknown authority surface")
)
