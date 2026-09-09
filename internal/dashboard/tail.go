package dashboard

import (
	"io"
	"os"
	"strings"
)

// tailBytes bounds how much of catalina.out one refresh reads. 64 KiB is a
// few hundred lines — more than a log viewport can show.
const tailBytes = 64 * 1024

// tailFile returns the file's content from max(minOffset, size-tailBytes) to
// the end, as whole lines. minOffset lets the drawer's "clear" key hide
// everything written before it; pass 0 for the plain last-tailBytes view. The
// first (likely partial) line of a read that does not start at byte 0 is
// dropped. minOffset past the end of file (e.g. stale, from a since-replaced
// log file) clamps to empty rather than erroring.
func tailFile(path string, minOffset int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	// capStart is where the tailBytes cap alone would start reading — a
	// byte offset that lands mid-line in practice, so that read is marked
	// partial and its first (broken) line gets dropped below. minOffset,
	// by contrast, is always a boundary a caller captured between whole
	// lines (the file size at the moment the drawer was cleared), so it
	// never needs that treatment.
	capStart := int64(0)
	partial := false
	if info.Size() > tailBytes {
		capStart = info.Size() - tailBytes
		partial = true
	}

	start := capStart
	if minOffset > start {
		start = minOffset
		partial = false
	}
	if start > info.Size() {
		start = info.Size()
		partial = false
	}

	if start > 0 {
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return "", err
		}
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
