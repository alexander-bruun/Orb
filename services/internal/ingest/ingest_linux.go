//go:build linux

package ingest

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// openAudioFile opens path for reading with O_NOATIME to avoid updating the
// access-time metadata on every read — saving a journal write on HDDs.
// Falls back to os.Open when O_NOATIME is not permitted (e.g. not owner/root).
func openAudioFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOATIME, 0)
	if err != nil {
		return os.Open(path)
	}
	return f, nil
}

// fadviseSequential hints to the kernel that we will read this file
// sequentially from start to finish, enabling aggressive readahead that
// significantly reduces effective latency on spinning disks.
func fadviseSequential(f *os.File) {
	_ = unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_SEQUENTIAL)
}
