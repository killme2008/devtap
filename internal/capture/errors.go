package capture

import (
	"errors"
	"fmt"
	"os"
)

var errNoCommand = errors.New("no command specified")

// Scanner buffer sizes for reading subprocess output.
const (
	ScannerInitBuf = 64 * 1024   // 64 KB initial buffer
	ScannerMaxBuf  = 1024 * 1024 // 1 MB max buffer
)

// warnStoreWrite logs a store write error to stderr without failing the capture.
func warnStoreWrite(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "devtap: store write: %v\n", err)
	}
}
