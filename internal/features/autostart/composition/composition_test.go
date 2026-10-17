package composition

import (
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	autostart "raven/internal/features/autostart/application"
	configyaml "raven/internal/features/configuration/adapters/yaml"
	configuration "raven/internal/features/configuration/domain"
)

type detector struct{ callback func(bool) }

func (native *detector) PrepareLoginDetection(callback func(bool)) { native.callback = callback }

func TestLaunchDetectionQueuesAnExplicitLaunch(t *testing.T) {
	native := &detector{}
	requests := autostart.NewRequests()
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
		configurationFixture(), nil,
	)
	if controller == nil {
		t.Fatal("missing autostart controller")
	}
}

func configurationFixture() configuration.Settings {
	snapshot, err := (configyaml.Loader{}).Load()
	if err != nil {
		panic(err)
	}
	return snapshot.Settings
}
