package version

import (
	"fmt"
	"io"
)

var (
	Version  = "unknown"
	Revision = "unknown"
)

func Write(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Version: %s\n", Version)
	_, _ = fmt.Fprintf(w, "Revision: %s\n", Revision)
}
