//go:build unix

package executor

import "golang.org/x/sys/unix"

// In particular, opening a FIFO that replaced a regular file cannot wait for a writer.
const regularFileOpenFlags = unix.O_RDONLY | unix.O_NONBLOCK | unix.O_NOCTTY
