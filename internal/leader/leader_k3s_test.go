package leader_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/jensholdgaard/discord-dkp-bot/internal/config"
	"github.com/jensholdgaard/discord-dkp-bot/internal/leader"
)

// TestLeaderElection_K3s validates real Kubernetes Lease-based leader election
// using a k3s testcontainer. Skipped in short mode.
func TestLeaderElection_K3s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping k3s integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Start a k3s container.
	ctr, err := k3s.Run(ctx, "rancher/k3s:v1.31.6-k3s1")
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("starting k3s container: %v", err)
	}

	// Get kubeconfig from the container.
	kubeConfigYaml, err := ctr.GetKubeConfig(ctx)
	if err != nil {
		t.Fatalf("getting kubeconfig: %v", err)
	}

	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigYaml)
	if err != nil {
		t.Fatalf("building rest config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("creating kubernetes client: %v", err)
	}

	// Override the ClientFactory so leader.Run uses our test cluster.
	origFactory := leader.ClientFactory
	leader.ClientFactory = func() (kubernetes.Interface, error) {
		return clientset, nil
	}
	t.Cleanup(func() { leader.ClientFactory = origFactory })

	cfg := config.LeaderElectionConfig{
		Enabled:        true,
		LeaseName:      "dkpbot-test-leader",
		LeaseNamespace: "default",
		LeaseDuration:  5 * time.Second,
		RenewDeadline:  3 * time.Second,
		RetryPeriod:    1 * time.Second,
	}

	logger := slog.Default()

	var leaderAcquired atomic.Bool
	leaderCtx, leaderCancel := context.WithCancel(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- leader.Run(leaderCtx, cfg, logger,
			func(ctx context.Context) {
				leaderAcquired.Store(true)
				// Block until context is canceled (simulating the bot running).
				<-ctx.Done()
			},
			func() {
				// Leadership lost callback.
			},
		)
	}()

	// Wait for leadership to be acquired.
	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for !leaderAcquired.Load() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for leader election")
		case <-ticker.C:
		}
	}

	t.Log("leader election acquired successfully")

	// Cancel to release leadership.
	leaderCancel()

	select {
	case runErr := <-errCh:
		if runErr != nil {
			t.Fatalf("leader.Run() error = %v", runErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for leader.Run to return")
	}
}
