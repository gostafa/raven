package composition

import (
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"raven/internal/features/autostart/adapters/launch"
	configuration "raven/internal/features/configuration/domain"
)

type detector struct{ callback func(bool) }

func (native *detector) PrepareLoginDetection(callback func(bool)) { native.callback = callback }

func TestLaunchDetectionQueuesAnExplicitLaunch(t *testing.T) {
	native := &detector{}
	requests := launch.New()
	result := newLaunch(launchParams{Detector: native, Requests: requests})
	if result.Requests != requests || result.Visibility != requests.Visibility() {
		t.Fatal("launch contracts diverged")
	}
	result.Visibility.ApplicationStarted()
	native.callback(false)
	select {
	case <-requests.Pending():
	case <-time.After(time.Second):
		t.Fatal("manual launch did not open the library")
	}
}

func TestControllerProvider(t *testing.T) {
	controller := newController(
		application.New(application.Options{}),
		&configuration.Settings{ApplicationID: "raven"},
	)
	if controller == nil {
		t.Fatal("missing autostart controller")
	}
}
