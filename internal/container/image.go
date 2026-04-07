package container

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// hashFile returns the path to the image content hash cache file.
func hashFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-mux", "sandbox-image-hash")
}

// EnsureImage builds the sandbox image if it doesn't exist or the Dockerfile has changed.
// assets should be an embed.FS containing Dockerfile, init-firewall.sh, and hook-handler.sh.
func EnsureImage(runtime Runtime, assets fs.FS) error {
	contentHash, err := hashAssets(assets)
	if err != nil {
		return fmt.Errorf("hashing assets: %w", err)
	}

	// Check if image exists and hash matches
	if existingHash, err := os.ReadFile(hashFile()); err == nil {
		if strings.TrimSpace(string(existingHash)) == contentHash {
			// Verify image actually exists
			if imageExists(runtime) {
				return nil
			}
			// Image was pruned but hash file is stale — remove it to force a clean rebuild
			os.Remove(hashFile())
		}
	}

	// Extract assets to a temp dir and build
	tmpDir, err := os.MkdirTemp("", "claude-mux-sandbox-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractAssets(assets, tmpDir); err != nil {
		return fmt.Errorf("extracting assets: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Building sandbox image (this may take a few minutes on first run)...\n")

	cmd := exec.Command(string(runtime), "build", "-t", ImageName, tmpDir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building image: %w", err)
	}

	// Save hash
	os.MkdirAll(filepath.Dir(hashFile()), 0o755)
	os.WriteFile(hashFile(), []byte(contentHash+"\n"), 0o644)

	return nil
}

// BuildImage forces a rebuild of the sandbox image regardless of cache.
func BuildImage(runtime Runtime, assets fs.FS) error {
	// Remove cached hash to force rebuild
	os.Remove(hashFile())
	return EnsureImage(runtime, assets)
}

// ImageExists reports whether the sandbox image is available in the local container store.
func ImageExists(runtime Runtime) bool {
	return exec.Command(string(runtime), "image", "inspect", ImageName).Run() == nil
}

func imageExists(runtime Runtime) bool {
	return ImageExists(runtime)
}

func hashAssets(assets fs.FS) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func extractAssets(assets fs.FS, dir string) error {
	return fs.WalkDir(assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.Base(path))
		return os.WriteFile(dst, data, 0o644)
	})
}
