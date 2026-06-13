package cli

import (
	"errors"

	"github.com/tamnd/cvf-cli/cvf"
)

func isNotFound(err error) bool {
	return errors.Is(err, cvf.ErrNotFound)
}
