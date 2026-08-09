package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"daycore/internal/resources"
	"daycore/internal/version"
)

// `daycore install -fetch` — download the resource pack a lite build needs.
//
// # Why this exists at all
//
// A lite binary embeds nothing, so its content has to arrive some other way.
// There are three legitimate routes and they are all the same binary: unpack
// the release tarball next to it, point DAYCORE_DATA_DIR at a directory a full
// binary already wrote, or this. Offering only the first two would mean the
// lite build cannot bootstrap itself, which is the shape people actually expect
// from a single downloaded file.
//
// # Boundary: never on the startup path, never automatic
//
// Only `install -fetch`, only when asked. The server never reaches for the
// network to find a prompt template: a binary that silently downloaded its own
// behaviour would be unauditable, would break in exactly the air-gapped
// deployments this project cares about, and would turn a DNS outage into a
// change in what the assistant says.
//
// # Boundary: version-pinned, no "latest"
//
// The pack is fetched for THIS binary's version. A lite binary running last
// year's code against this year's prompts is a configuration nobody tested and
// nobody can reproduce from a version number.
const dataPackTimeout = 60 * time.Second

// DataPackBaseURL is where release artifacts live. A variable so a mirror can
// be substituted at build time (-ldflags -X) without a patched source tree —
// self-hosters behind a proxy are a first-class case here.
var DataPackBaseURL = "https://github.com/alex04130/daycore/releases/download"

func dataPackURL() string {
	return fmt.Sprintf("%s/v%s/daycore-data-%s.tar.gz", DataPackBaseURL, version.Version, version.Version)
}

func fetchDataPack(dir string) error {
	if !resources.Lite {
		// A full build already has everything. Downloading a second copy would
		// create a directory that shadows nothing and confuses the next person to
		// read the deployment.
		return fmt.Errorf("-fetch is for lite builds; this binary already carries its resources")
	}
	url := dataPackURL()
	fmt.Printf("  fetching %s\n", url)

	client := &http.Client{Timeout: dataPackTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("fetch data pack: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch data pack: %s returned %s", url, resp.Status)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("data pack is not gzip: %w", err)
	}
	defer gz.Close()

	n, err := untar(gz, dir)
	if err != nil {
		return err
	}
	fmt.Printf("  extracted %d files into %s\n", n, dir)
	return nil
}

// untar extracts a tarball, refusing any entry that would escape the
// destination.
//
// Path traversal is checked rather than trusted even though we publish the
// archive: -fetch takes its URL from a variable that a build flag or a mirror
// can set, so the bytes are not guaranteed to be ours. An archive member named
// ../../etc/cron.d/x is the whole attack, and it costs one filepath.Rel to
// close.
func untar(r io.Reader, dest string) (int, error) {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return 0, err
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return 0, err
	}
	tr := tar.NewReader(r)
	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return count, nil
		}
		if err != nil {
			return count, err
		}
		target := filepath.Join(abs, filepath.Clean("/"+hdr.Name))
		rel, err := filepath.Rel(abs, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return count, fmt.Errorf("data pack contains an entry outside the destination: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return count, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return count, err
			}
			f, err := os.Create(target)
			if err != nil {
				return count, err
			}
			// Bounded copy: a decompression bomb in the resource path would
			// otherwise fill the disk of a machine that was only trying to install.
			if _, err := io.Copy(f, io.LimitReader(tr, 8<<20)); err != nil {
				f.Close()
				return count, err
			}
			f.Close()
			count++
		default:
			// Symlinks and devices have no business in a pack of templates, and
			// a symlink is the other half of the traversal attack.
			return count, fmt.Errorf("data pack contains an unsupported entry type for %s", hdr.Name)
		}
	}
}
