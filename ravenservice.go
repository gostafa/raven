package main

import (
	"context"
	"crypto/rand"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"raven/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const widgetSize = 48

func (r *RavenService) configureWidget() {
	application.InvokeSync(func() {
		if err := configureNativeWidget(r.widget); err != nil {
			r.app.Logger.Error("Could not configure Raven widget", "error", err)
		}
	})
}

// RavenService owns the windows and folder database for the lifetime of the app.
type RavenService struct {
	app          *application.App
	mainWindow   *application.WebviewWindow
	widget       *application.WebviewWindow
	reviewWindow *application.WebviewWindow
	store        *storage.Store
	previews     sync.Map
	notifier     *notifications.NotificationService
	reminders    *reminderCoordinator
	reminderMu   sync.Mutex
}

func (r *RavenService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	path, err := storage.DefaultPath()
	if err != nil {
		return err
	}
	r.store, err = storage.Open(path)
	if err != nil {
		return err
	}
	if r.notifier != nil {
		if err := r.notifier.ServiceStartup(ctx, options); err != nil {
			if r.app != nil {
				r.app.Logger.Warn("Desktop notifications unavailable", "error", err)
			}
			r.notifier = nil
		}
	}
	return nil
}

func (r *RavenService) ServiceShutdown() error {
	r.reminderMu.Lock()
	c := r.reminders
	r.reminderMu.Unlock()
	if c != nil {
		c.stop()
	}
	if r.store == nil {
		return nil
	}
	if r.notifier != nil {
		_ = r.notifier.ServiceShutdown()
	}
	return r.store.Close()
}

func (r *RavenService) ListFolders() ([]storage.Folder, error) {
	if r.store == nil {
		return nil, errors.New("folder storage is unavailable")
	}
	return r.store.ListFolders()
}

func (r *RavenService) CreateFolder(input storage.CreateFolderInput) (storage.Folder, error) {
	if r.store == nil {
		return storage.Folder{}, errors.New("folder storage is unavailable")
	}
	return r.store.CreateFolder(input)
}

func (r *RavenService) ListCards() ([]storage.Card, error) {
	if r.store == nil {
		return nil, errors.New("card storage is unavailable")
	}
	return r.store.ListCards()
}

func (r *RavenService) DeleteFolder(folderID string) error {
	if r.store == nil {
		return errors.New("folder storage is unavailable")
	}
	err := r.store.DeleteFolder(folderID)
	if err == nil {
		r.schedulesChanged()
	}
	return err
}

func (r *RavenService) DeleteCard(folderID, cardID string) error {
	if r.store == nil {
		return errors.New("card storage is unavailable")
	}
	err := r.store.DeleteCard(folderID, cardID)
	if err == nil {
		r.schedulesChanged()
	}
	return err
}

func (r *RavenService) CreateCard(input storage.CreateCardInput) (storage.Card, error) {
	if r.store == nil {
		return storage.Card{}, errors.New("card storage is unavailable")
	}
	card, err := r.store.CreateCard(input)
	if err == nil {
		r.schedulesChanged()
	}
	return card, err
}

func (r *RavenService) UpdateCard(input storage.UpdateCardInput) (storage.Card, error) {
	if r.store == nil {
		return storage.Card{}, errors.New("card storage is unavailable")
	}
	card, err := r.store.UpdateCard(input)
	if err == nil {
		r.schedulesChanged()
	}
	return card, err
}

type SelectedMedia struct {
	SourcePath   string `json:"sourcePath"`
	OriginalName string `json:"originalName"`
	ByteSize     int64  `json:"byteSize"`
	PreviewURL   string `json:"previewURL"`
}

func (r *RavenService) SelectMediaFiles(kind string) ([]SelectedMedia, error) {
	filters := map[string]string{
		"image": "*.jpg;*.jpeg;*.png;*.gif;*.webp;*.avif",
		"audio": "*.mp3;*.wav;*.m4a;*.ogg;*.oga;*.flac",
		"video": "*.mp4;*.m4v;*.webm;*.ogv;*.mov",
	}
	filter, ok := filters[kind]
	if !ok {
		return nil, errors.New("invalid media type")
	}
	if r.app == nil || r.mainWindow == nil {
		return nil, errors.New("file selection is unavailable")
	}
	paths, err := r.app.Dialog.OpenFile().AttachToWindow(r.mainWindow).CanChooseFiles(true).CanChooseDirectories(false).AllowsOtherFileTypes(false).AddFilter(kind+" files", filter).PromptForMultipleSelection()
	if err != nil {
		return nil, err
	}
	selected := make([]SelectedMedia, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, errors.New("select a nonempty media file")
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !strings.Contains(";"+filter+";", ";*"+ext+";") {
			return nil, errors.New("choose a supported file for this media type")
		}
		token := rand.Text()
		r.previews.Store(token, path)
		selected = append(selected, SelectedMedia{SourcePath: path, OriginalName: filepath.Base(path), ByteSize: info.Size(), PreviewURL: "/media-preview/" + token})
	}
	return selected, nil
}

func (r *RavenService) mediaMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/media-preview/") {
			if req.Method != http.MethodGet && req.Method != http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			path, ok := r.previews.Load(strings.TrimPrefix(req.URL.Path, "/media-preview/"))
			if !ok {
				http.NotFound(w, req)
				return
			}
			file, err := os.Open(path.(string))
			if err != nil {
				http.NotFound(w, req)
				return
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil || !info.Mode().IsRegular() {
				http.NotFound(w, req)
				return
			}
			w.Header().Set("Content-Type", mime.TypeByExtension(strings.ToLower(filepath.Ext(path.(string)))))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-store")
			http.ServeContent(w, req, info.Name(), info.ModTime(), file)
			return
		}
		if !strings.HasPrefix(req.URL.Path, "/media/") {
			next.ServeHTTP(w, req)
			return
		}
		if r.store == nil {
			http.Error(w, "media storage is unavailable", http.StatusServiceUnavailable)
			return
		}
		r.store.MediaHandler().ServeHTTP(w, req)
	})
}

func (r *RavenService) ReviewItem(folderID, cardID, itemID string, rating int) (storage.Card, error) {
	if r.store == nil {
		return storage.Card{}, errors.New("card storage is unavailable")
	}
	card, err := r.store.ReviewItem(folderID, cardID, itemID, rating)
	if err == nil {
		r.schedulesChanged()
	}
	return card, err
}

func (r *RavenService) ShowMain() {
	r.mainWindow.Show()
	r.mainWindow.Restore()
	r.mainWindow.Focus()
}

func (r *RavenService) HideMain() {
	r.widget.Show()
	r.mainWindow.Hide()
}

func (r *RavenService) ResetWidget() {
	if screen := r.app.Screen.GetPrimary(); screen != nil {
		area := screen.WorkArea
		r.widget.SetPosition(area.X+max(0, area.Width-widgetSize-24), area.Y+max(0, area.Height-widgetSize-24))
	}
	r.widget.Show()
	r.positionFloatingReview()
}

// Quit is intentionally separate from closing the main window.
func (r *RavenService) Quit() {
	r.app.Quit()
}

// ShowReviews opens the main window and requests a review session.
func (r *RavenService) ShowReviews() {
	r.ShowMain()
	r.app.Event.Emit("raven:show-reviews")
}

func (r *RavenService) RefreshReminders() {
	r.reminderMu.Lock()
	defer r.reminderMu.Unlock()
	if r.reminders != nil {
		r.reminders.wake()
	}
}

func (r *RavenService) schedulesChanged() {
	if r.app != nil {
		r.app.Event.Emit("raven:schedules-changed")
	}
	r.RefreshReminders()
}
