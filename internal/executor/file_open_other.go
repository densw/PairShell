//go:build !unix

package executor

import "os"

const regularFileOpenFlags = os.O_RDONLY
