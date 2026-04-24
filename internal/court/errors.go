// Package court provides Court runtime functionality.
package court

import "fmt"

func wrapErr(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}
