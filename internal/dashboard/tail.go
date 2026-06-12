package dashboard

import (
	"io"
	"os"
	"strings"
)

// tailBytes bounds how much of catalina.out one refresh reads. 64 KiB is a
// few hundred lines — more than a log viewport can show.
const tailBytes = 64 * 1024

// tailFile returns the last tailBytes of the file as whole lines. The first
// (likely partial) line of a mid-file read is dropped.
func tailFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	partial := false
	if info.Size() > tailBytes {
		if _, err := f.Seek(-tailBytes, io.SeekEnd); err != nil {
			return "", err
		}
		partial = true
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	content := string(data)
	if partial {
		if idx := strings.IndexByte(content, '\n'); idx >= 0 {
			content = content[idx+1:]
		}
	}

	return content, nil
}
