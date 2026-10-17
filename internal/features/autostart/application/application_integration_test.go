package application_test

import (
	"testing"
	"time"

	autostart "raven/internal/features/autostart/application"
)

type launchDetector struct{ automatic bool }

func (detector launchDetector) PrepareLoginDetection(detected func(bool)) {
	detected(detector.automatic)
}

type classificationChannel chan bool

func (classified classificationChannel) Classify(automatic bool) { classified <- automatic }

func TestPrepareLaunchCombinesNativeDetectionAndArgumentsWithoutBlocking(t *testing.T) {
	for _, test := range []struct {
		name   string
		native bool
		args   []string
		want   bool
	}{
		{"manual", false, nil, false},
		{"native login", true, nil, true},
		{"argument", false, []string{"--autostart"}, true},
		{"both", true, []string{"--autostart"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			classified := make(classificationChannel)
			autostart.PrepareLaunch(launchDetector{automatic: test.native}, classified, test.args)
			select {
			case automatic := <-classified:
				if automatic != test.want {
					t.Fatalf("automatic=%v, want %v", automatic, test.want)
				}
			case <-time.After(time.Second):
				t.Fatal("native launch classification was lost")
			}
		})
	}
}

func TestLaunchRequestsWaitForReadinessAndCoalesce(t *testing.T) {
	requests := autostart.NewRequests()
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
	requests := autostart.NewRequests()
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
