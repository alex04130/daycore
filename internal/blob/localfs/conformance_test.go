package localfs

import (
	"testing"

	"daycore/internal/blob"
	"daycore/internal/blob/blobtest"
)

// The shared behavioural suite against a real directory. See
// internal/blob/blobtest for why it exists before the second driver does.
func TestConformance(t *testing.T) {
	blobtest.Run(t, func(t *testing.T) blob.Store {
		s, err := open(t.TempDir())
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		return s
	})
}

// Opening must fail loudly on a path that cannot be written, rather than at the
// first upload — where it would look like a bug in the upload.
func TestOpenRejectsUnwritablePath(t *testing.T) {
	if _, err := open(""); err == nil {
		t.Error("empty directory accepted")
	}
	if _, err := open("/proc/daycore-cannot-exist/blobs"); err == nil {
		t.Error("an unwritable path was accepted; the failure would surface on a user's first upload")
	}
}
