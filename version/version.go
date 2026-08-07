package version

import (
	"fmt"
	"io"
)

var (
	Version  = "unknown"
	Revision = "unknown"
)

func Write(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Version: %s\n", Version); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "Revision: %s\n", Revision)
	return err
}
