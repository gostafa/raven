package storage

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	bolt "go.etcd.io/bbolt"
)

// Deliberately exclude active formats such as SVG and HTML.
var mediaTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".m4a":  "audio/mp4",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".flac": "audio/flac",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
	".ogv":  "video/ogg",
	".mov":  "video/quicktime",
}
var itemIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func validMediaPath(path, id string) bool {
	if !itemIDPattern.MatchString(id) {
		return false
	}
	ext := filepath.Ext(path)
	_, supported := mediaTypes[ext]
	return supported && path == "media/"+id+ext
}

func (s *Store) mediaPath(relative string) string {
	return filepath.Join(s.mediaDir, filepath.Base(relative))
}

func inspectMedia(file *os.File, kind, source string) (string, error) {
	declared := mediaTypes[strings.ToLower(filepath.Ext(source))]
	if (kind != "audio" && kind != "image" && kind != "video") ||
		!strings.HasPrefix(declared, kind+"/") {
		return "", errors.New("choose a supported file for this media type")
	}
	var header [512]byte
	n, err := file.Read(header[:])
	if err != nil && err != io.EOF {
		return "", err
	}
	if n == 0 {
		return "", errors.New("empty media file")
	}
	detected := strings.Split(http.DetectContentType(header[:n]), ";")[0]
	// Ogg, WebM, and MP4 containers can carry audio or video. AVIF/FLAC
	// may be reported as octet-stream by Go's sniffer; check their signatures.
	compatible := detected == declared ||
		(declared == "audio/wav" && detected == "audio/wave") ||
		((declared == "audio/ogg" || declared == "video/ogg") && detected == "application/ogg") ||
		(declared == "audio/mp4" && detected == "video/mp4") ||
		(declared == "video/quicktime" && n >= 12 && string(header[4:8]) == "ftyp") ||
		(declared == "image/avif" && n >= 12 && string(header[4:8]) == "ftyp" && (string(header[8:12]) == "avif" || string(header[8:12]) == "avis")) ||
		(declared == "audio/flac" && n >= 4 && string(header[:4]) == "fLaC")
	if !compatible {
		return "", fmt.Errorf("file contents do not match %s", declared)
	}
	_, err = file.Seek(0, io.SeekStart)
	return declared, err
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *Store) importMedia(item ContentItem, source string) (ContentItem, string, error) {
	file, err := os.Open(source)
	if err != nil {
		return item, "", fmt.Errorf("open selected file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return item, "", err
	}
	if !info.Mode().IsRegular() {
		return item, "", errors.New("select a regular media file")
	}
	mimeType, err := inspectMedia(file, item.Kind, source)
	if err != nil {
		return item, "", err
	}
	temp, err := os.CreateTemp(s.mediaDir, ".import-*")
	if err != nil {
		return item, "", err
	}
	defer os.Remove(temp.Name())
	size, copyErr := io.Copy(temp, file)
	if copyErr == nil {
		copyErr = temp.Sync()
	}
	closeErr := temp.Close()
	if copyErr != nil {
		return item, "", copyErr
	}
	if closeErr != nil {
		return item, "", closeErr
	}
	if size != info.Size() {
		return item, "", errors.New("selected file changed during import; try again")
	}
	item.RelativePath = "media/" + item.ID + strings.ToLower(filepath.Ext(source))
	path := s.mediaPath(item.RelativePath)
	if err := os.Rename(temp.Name(), path); err != nil {
		return item, "", err
	}
	if err := syncDirectory(s.mediaDir); err != nil {
		_ = os.Remove(path)
		return item, "", err
	}
	item.OriginalName = filepath.Base(source)
	item.MIMEType = mimeType
	item.ByteSize = size
	return item, path, nil
}

func (s *Store) cleanupMedia() error {
	// Validate all referenced records before removing anything.
	cards, err := s.ListCards()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.mediaDir, 0o700); err != nil {
		return err
	}
	referenced := make(map[string]bool)
	for _, card := range cards {
		for _, item := range card.Items {
			if item.Kind != "text" {
				referenced[filepath.Base(item.RelativePath)] = true
			}
		}
	}
	entries, err := os.ReadDir(s.mediaDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		managed := strings.HasPrefix(name, ".import-") ||
			validMediaPath("media/"+name, strings.TrimSuffix(name, filepath.Ext(name)))
		if !entry.IsDir() && managed && !referenced[name] {
			if err := os.Remove(filepath.Join(s.mediaDir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// MediaHandler serves only media records owned by the requested card.
// ServeContent implements HEAD and byte ranges without buffering the file.
func (s *Store) MediaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/media/"), "/")
		if !strings.HasPrefix(r.URL.Path, "/media/") || len(parts) != 3 ||
			!itemIDPattern.MatchString(parts[2]) {
			http.NotFound(w, r)
			return
		}
		var item ContentItem
		s.mediaMu.Lock()
		err := s.db.View(func(tx *bolt.Tx) error {
			card, err := loadCard(tx, parts[0], parts[1])
			if err != nil {
				return err
			}
			for _, candidate := range card.Items {
				if candidate.ID == parts[2] && candidate.Kind != "text" {
					item = candidate
					return nil
				}
			}
			return errors.New("media not found")
		})
		var file *os.File
		if err == nil {
			file, err = os.Open(s.mediaPath(item.RelativePath))
		}
		s.mediaMu.Unlock()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", item.MIMEType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, item.OriginalName, info.ModTime(), file)
	})
}
