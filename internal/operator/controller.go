package operator

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/awsparam"
	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/config"
	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/k8s"
	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/schedule"
	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/version"
	corev1 "k8s.io/api/core/v1"
)

type Controller struct {
	cfg    config.Config
	kube   *k8s.Client
	ssm    *awsparam.Reader
	window schedule.Window
}

func NewController(ctx context.Context, cfg config.Config) (*Controller, error) {
	kubeClient, err := k8s.New(cfg.Namespace)
	if err != nil {
		return nil, err
	}
	ssmReader, err := awsparam.NewReader(ctx, cfg.SSMParameterName, cfg.AWSRegion)
	if err != nil {
		return nil, err
	}
	updateWindow, err := schedule.New(cfg.UpdateWindowStart, cfg.RolloutStart, cfg.UpdateWindowEnd, cfg.TimeZone)
	if err != nil {
		return nil, err
	}
	return &Controller{
		cfg:    cfg,
		kube:   kubeClient,
		ssm:    ssmReader,
		window: updateWindow,
	}, nil
}

func (c *Controller) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.cfg.ControllerPollInterval)
	defer ticker.Stop()

	for {
		if err := c.reconcile(ctx); err != nil {
			log.Printf("controller reconcile failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Controller) reconcile(ctx context.Context) error {
	now := time.Now().In(c.window.Location)
	targetVersion, err := c.refreshTargetVersionOncePerDay(ctx, now)
	if err != nil {
		return err
	}
	if targetVersion == "" {
		log.Printf("no target version available yet")
		return nil
	}

	if !c.window.IsOpen(now) {
		log.Printf("update window is closed; target=%s", targetVersion)
		return nil
	}
	if c.window.Remaining(now) < time.Duration(c.cfg.MinRemainingMinutes)*time.Minute {
		log.Printf("update window has insufficient remaining time; remaining=%s", c.window.Remaining(now))
		return nil
	}

	return c.scheduleNodes(ctx, targetVersion)
}

func (c *Controller) refreshTargetVersionOncePerDay(ctx context.Context, now time.Time) (string, error) {
	state, err := c.kube.GetState(ctx, c.cfg.StateConfigMapName)
	if err != nil {
		return "", err
	}
	today := c.window.CurrentDate(now)
	if state[k8s.StateKeyFetchedDate] == today && state[k8s.StateKeyTargetVersion] != "" {
		return state[k8s.StateKeyTargetVersion], nil
	}

	rawVersion, err := c.ssm.Read(ctx)
	if err != nil {
		if state[k8s.StateKeyTargetVersion] != "" {
			log.Printf("failed to read SSM parameter %q; keeping last target version %q: %v", c.cfg.SSMParameterName, state[k8s.StateKeyTargetVersion], err)
			return state[k8s.StateKeyTargetVersion], nil
		}
		return "", err
	}
	targetVersion, err := version.Normalize(rawVersion)
	if err != nil {
		return "", err
	}

	state[k8s.StateKeyTargetVersion] = targetVersion
	state[k8s.StateKeyFetchedDate] = today
	state[k8s.StateKeyFetchedAt] = now.Format(time.RFC3339)
	state[k8s.StateKeyParameterName] = c.cfg.SSMParameterName
	if err := c.kube.PutState(ctx, c.cfg.StateConfigMapName, state); err != nil {
		return "", err
	}
	log.Printf("loaded target version %q from SSM parameter %q", targetVersion, c.cfg.SSMParameterName)
	return targetVersion, nil
}

func (c *Controller) scheduleNodes(ctx context.Context, targetVersion string) error {
	nodes, err := c.kube.ListNodes(ctx, c.cfg.TargetNodeLabelSelector)
	if err != nil {
		return err
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })

	active := 0
	for _, node := range nodes {
		if isActiveState(node.Annotations[k8s.AnnotationState]) {
			active++
		}
	}
	if active >= c.cfg.MaxConcurrentUpdates {
		log.Printf("max concurrent updates reached; active=%d max=%d", active, c.cfg.MaxConcurrentUpdates)
		return nil
	}

	for _, node := range nodes {
		if !k8s.NodeReady(node) {
			continue
		}
		if shouldSkipNode(node, targetVersion) {
			continue
		}
		now := time.Now().Format(time.RFC3339)
		log.Printf("requesting update for node=%s target=%s", node.Name, targetVersion)
		return c.kube.PatchNodeAnnotations(ctx, node.Name, map[string]string{
			k8s.AnnotationState:         k8s.StateRequested,
			k8s.AnnotationTargetVersion: targetVersion,
			k8s.AnnotationRequestedAt:   now,
			k8s.AnnotationStartedAt:     now,
			k8s.AnnotationLastError:     "",
		})
	}

	log.Printf("no eligible nodes found for target=%s", targetVersion)
	return nil
}

func shouldSkipNode(node corev1.Node, targetVersion string) bool {
	annotations := node.Annotations
	if annotations == nil {
		return true
	}
	currentVersion := annotations[k8s.AnnotationCurrentVersion]
	if currentVersion == "" {
		return true
	}
	if version.Equal(currentVersion, targetVersion) {
		return true
	}
	return isActiveState(annotations[k8s.AnnotationState])
}

func isActiveState(state string) bool {
	switch state {
	case k8s.StateRequested, k8s.StateCordoning, k8s.StateDraining, k8s.StateUpdating, k8s.StateRebooting:
		return true
	default:
		return false
	}
}
