// Package leader provides Kubernetes Lease-based leader election
// so that only one replica of the bot actively processes commands.
package leader

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// Config holds leader election settings.
type Config struct {
	// Enabled turns leader election on/off.
	Enabled bool `yaml:"enabled"`
	// LeaseName is the name of the Kubernetes Lease resource.
	LeaseName string `yaml:"lease_name"`
	// LeaseNamespace is the namespace of the Lease resource.
	LeaseNamespace string `yaml:"lease_namespace"`
	// LeaseDuration is how long a leader holds the lease.
	LeaseDuration time.Duration `yaml:"lease_duration"`
	// RenewDeadline is how long the leader tries to renew before giving up.
	RenewDeadline time.Duration `yaml:"renew_deadline"`
	// RetryPeriod is the time between attempts to acquire/renew leadership.
	RetryPeriod time.Duration `yaml:"retry_period"`
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		Enabled:        false,
		LeaseName:      "dkpbot-leader",
		LeaseNamespace: "default",
		LeaseDuration:  15 * time.Second,
		RenewDeadline:  10 * time.Second,
		RetryPeriod:    2 * time.Second,
	}
}

// identity returns a unique identity for this instance.
// It uses the POD_NAME env var if set, otherwise the hostname.
func identity() string {
	if name := os.Getenv("POD_NAME"); name != "" {
		return name
	}
	host, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return host
}

// ClientFactory creates a Kubernetes clientset.
// Extracted as a variable for testing.
var ClientFactory = func() (kubernetes.Interface, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("building in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}
	return client, nil
}

// Run starts leader election. The onStartedLeading callback is invoked when
// this instance becomes the leader; it should block until ctx is done.
// The onStoppedLeading callback runs when leadership is lost.
// Run itself blocks until the election loop exits.
func Run(ctx context.Context, cfg Config, logger *slog.Logger, onStartedLeading func(ctx context.Context), onStoppedLeading func()) error {
	id := identity()
	logger.Info("starting leader election",
		slog.String("identity", id),
		slog.String("lease", cfg.LeaseName),
		slog.String("namespace", cfg.LeaseNamespace),
	)

	client, err := ClientFactory()
	if err != nil {
		return fmt.Errorf("leader election client: %w", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LeaseName,
			Namespace: cfg.LeaseNamespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   cfg.LeaseDuration,
		RenewDeadline:   cfg.RenewDeadline,
		RetryPeriod:     cfg.RetryPeriod,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				logger.Info("acquired leadership", slog.String("identity", id))
				onStartedLeading(ctx)
			},
			OnStoppedLeading: func() {
				logger.Info("lost leadership", slog.String("identity", id))
				onStoppedLeading()
			},
			OnNewLeader: func(newID string) {
				if newID == id {
					return
				}
				logger.Info("new leader elected", slog.String("leader", newID))
			},
		},
	})

	return nil
}
