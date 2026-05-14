package replay

import "errors"

var (
	ErrInvalidCase               = errors.New("replay: invalid case")
	ErrMissingTrajectory         = errors.New("replay: missing trajectory")
	ErrUnsupportedCandidate      = errors.New("replay: unsupported candidate")
	ErrReplaySideEffectForbidden = errors.New("replay: side effects are forbidden")
	ErrInconclusive              = errors.New("replay: inconclusive")
)
