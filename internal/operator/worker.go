package operator

import (
	"context"
	"log"
	"time"

	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/bottlerocket"
	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/config"
	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/k8s"
	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/version"
)

type Worker struct {
	cfg  config.Config
	kube *k8s.Client
	br   *bottlerocket.Client
}

func NewWorker(cfg config.Config) (*Worker, error) {
	kubeClient, err := k8s.New(cfg.Namespace)
	if err != nil {
		return nil, err
	}
	return &Worker{
		cfg:  cfg,
		kube: kubeClient,
		br:   bottlerocket.New(cfg.APIClientBin, cfg.SignpostBin),
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.WorkerPollInterval)
	defer ticker.Stop()

	for {
		if err := w.reconcile(ctx); err != nil {
			log.Printf("worker reconcile failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) reconcile(ctx context.Context) error {
	info, err := w.br.OSInfo(ctx)
	if err != nil {
		return err
	}
	if err := w.kube.PatchNodeAnnotations(ctx, w.cfg.NodeName, map[string]string{
		k8s.AnnotationCurrentVersion: info.VersionID,
		k8s.AnnotationVariantID:      info.VariantID,
		k8s.AnnotationArch:           info.Arch,
	}); err != nil {
		return err
	}

	node, err := w.kube.GetNode(ctx, w.cfg.NodeName)
	if err != nil {
		return err
	}
	annotations := node.Annotations
	targetVersion := annotations[k8s.AnnotationTargetVersion]
	state := annotations[k8s.AnnotationState]

	if targetVersion == "" {
		return nil
	}
	if version.Equal(info.VersionID, targetVersion) {
		if state != k8s.StateCompleted {
			log.Printf("node=%s already running target=%s; uncordoning", w.cfg.NodeName, targetVersion)
			if err := w.kube.SetNodeUnschedulable(ctx, w.cfg.NodeName, false); err != nil {
				return err
			}
			return w.kube.PatchNodeAnnotations(ctx, w.cfg.NodeName, map[string]string{
				k8s.AnnotationState:       k8s.StateCompleted,
				k8s.AnnotationCompletedAt: time.Now().Format(time.RFC3339),
				k8s.AnnotationLastError:   "",
			})
		}
		return nil
	}

	switch state {
	case k8s.StateRequested:
		return w.updateNode(ctx, targetVersion)
	case k8s.StateRebooting:
		return w.handlePostRebootMismatch(ctx, targetVersion, info.VersionID)
	default:
		return nil
	}
}

func (w *Worker) updateNode(ctx context.Context, targetVersion string) error {
	log.Printf("starting update on node=%s target=%s", w.cfg.NodeName, targetVersion)
	if err := w.setState(ctx, k8s.StateCordoning, ""); err != nil {
		return err
	}
	if err := w.kube.SetNodeUnschedulable(ctx, w.cfg.NodeName, true); err != nil {
		return w.failBeforeReboot(ctx, err)
	}

	if err := w.setState(ctx, k8s.StateDraining, ""); err != nil {
		return err
	}
	if err := w.kube.DrainNode(ctx, w.cfg.NodeName, w.cfg.DrainTimeout, w.cfg.PodGracePeriodSeconds); err != nil {
		return w.failBeforeReboot(ctx, err)
	}

	if w.cfg.ExcludeFromLBWaitDuration > 0 {
		log.Printf("waiting after drain before update; duration=%s", w.cfg.ExcludeFromLBWaitDuration)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(w.cfg.ExcludeFromLBWaitDuration):
		}
	}

	if err := w.setState(ctx, k8s.StateUpdating, ""); err != nil {
		return err
	}
	if err := w.br.SetVersionLock(ctx, targetVersion); err != nil {
		return w.failBeforeReboot(ctx, err)
	}
	if err := w.br.CheckUpdate(ctx); err != nil {
		return w.failBeforeReboot(ctx, err)
	}
	if err := w.br.ApplyUpdate(ctx); err != nil {
		return w.failBeforeReboot(ctx, err)
	}

	if err := w.setState(ctx, k8s.StateRebooting, ""); err != nil {
		return err
	}
	log.Printf("rebooting node=%s target=%s", w.cfg.NodeName, targetVersion)
	if err := w.br.Reboot(ctx); err != nil {
		log.Printf("reboot command returned an error after state was set to rebooting: %v", err)
	}
	return nil
}

func (w *Worker) handlePostRebootMismatch(ctx context.Context, targetVersion, currentVersion string) error {
	node, err := w.kube.GetNode(ctx, w.cfg.NodeName)
	if err != nil {
		return err
	}
	startedAt, err := time.Parse(time.RFC3339, node.Annotations[k8s.AnnotationStartedAt])
	if err != nil {
		startedAt = time.Now()
	}
	if time.Since(startedAt) < w.cfg.PostRebootGracePeriod {
		return nil
	}

	message := "node rebooted without reaching target version; current=" + currentVersion + " target=" + targetVersion
	log.Print(message)
	if err := w.kube.SetNodeUnschedulable(ctx, w.cfg.NodeName, false); err != nil {
		return err
	}
	return w.kube.PatchNodeAnnotations(ctx, w.cfg.NodeName, map[string]string{
		k8s.AnnotationState:     k8s.StateFailed,
		k8s.AnnotationLastError: message,
	})
}

func (w *Worker) failBeforeReboot(ctx context.Context, cause error) error {
	log.Printf("update failed before reboot on node=%s: %v", w.cfg.NodeName, cause)
	if w.cfg.RollbackOnFailure {
		if err := w.br.DeactivatePreparedUpdate(ctx); err != nil {
			log.Printf("failed to deactivate prepared update: %v", err)
		}
	}
	if err := w.kube.SetNodeUnschedulable(ctx, w.cfg.NodeName, false); err != nil {
		return err
	}
	if err := w.kube.PatchNodeAnnotations(ctx, w.cfg.NodeName, map[string]string{
		k8s.AnnotationState:     k8s.StateFailed,
		k8s.AnnotationLastError: cause.Error(),
	}); err != nil {
		return err
	}
	return cause
}

func (w *Worker) setState(ctx context.Context, state, message string) error {
	annotations := map[string]string{
		k8s.AnnotationState: state,
	}
	if message != "" {
		annotations[k8s.AnnotationLastError] = message
	}
	return w.kube.PatchNodeAnnotations(ctx, w.cfg.NodeName, annotations)
}
