package providers

import "fmt"

func errExecutionDisabled(provider string) error {
	return fmt.Errorf("%s: execution unavailable in internal/providers runtime; use pkg/providers manager-backed runtime", provider)
}
