package launch_test

import (
	"testing"

	"raven/internal/features/autostart/adapters/launch"
)

func TestLaunchRequestsWaitForReadinessAndCoalesce(t *testing.T) {
	requests := launch.New()
	visibility := requests.Visibility()
	visibility.Classify(false)
	visibility.SecondInstance(nil)
	select {
	case <-requests.Pending():
		t.Fatal("opened before application readiness")
	default:
	}
	visibility.ApplicationStarted()
	select {
	case <-requests.Pending():
	default:
		t.Fatal("manual launch did not open")
	}
	visibility.SecondInstance([]string{"--autostart"})
	select {
	case <-requests.Pending():
		t.Fatal("automatic second launch opened main window")
	default:
	}
	for range 3 {
		visibility.RequestOpen()
	}
	select {
	case <-requests.Pending():
	default:
		t.Fatal("explicit opening request lost")
	}
	select {
	case <-requests.Pending():
		t.Fatal("requests did not coalesce")
	default:
	}
}

func TestAutomaticLaunchStaysHiddenUntilExplicitRequest(t *testing.T) {
	requests := launch.New()
	visibility := requests.Visibility()
	visibility.ApplicationStarted()
	visibility.Classify(true)
	select {
	case <-requests.Pending():
		t.Fatal("automatic launch opened main window")
	default:
	}
	visibility.RequestOpen()
	select {
	case <-requests.Pending():
	default:
		t.Fatal("explicit opening request lost")
	}
}
