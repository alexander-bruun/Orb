//go:build !linux

package ingest

import "os"

func openAudioFile(path string) (*os.File, error) { return os.Open(path) }

func fadviseSequential(_ *os.File) {}
