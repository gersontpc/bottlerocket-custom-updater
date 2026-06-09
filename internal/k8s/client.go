package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	Clientset kubernetes.Interface
	Namespace string
}

func New(namespace string) (*Client, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	return &Client{Clientset: clientset, Namespace: namespace}, nil
}

func (c *Client) GetNode(ctx context.Context, nodeName string) (*corev1.Node, error) {
	return c.Clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
}

func (c *Client) ListNodes(ctx context.Context, selector string) ([]corev1.Node, error) {
	listOptions := metav1.ListOptions{}
	if selector != "" {
		parsed, err := labels.Parse(selector)
		if err != nil {
			return nil, err
		}
		listOptions.LabelSelector = parsed.String()
	}
	nodes, err := c.Clientset.CoreV1().Nodes().List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

func (c *Client) PatchNodeAnnotations(ctx context.Context, nodeName string, annotations map[string]string) error {
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": annotations,
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = c.Clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.MergePatchType, data, metav1.PatchOptions{})
	return err
}

func (c *Client) SetNodeUnschedulable(ctx context.Context, nodeName string, unschedulable bool) error {
	patch := map[string]any{
		"spec": map[string]any{
			"unschedulable": unschedulable,
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = c.Clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.MergePatchType, data, metav1.PatchOptions{})
	return err
}

func (c *Client) GetState(ctx context.Context, name string) (map[string]string, error) {
	cm, err := c.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return cm.Data, nil
}

func (c *Client) PutState(ctx context.Context, name string, data map[string]string) error {
	cm, err := c.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.Namespace},
			Data:       data,
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	cm.Data = data
	_, err = c.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

func (c *Client) DrainNode(ctx context.Context, nodeName string, timeout time.Duration, gracePeriodSeconds int64) error {
	deadline := time.Now().Add(timeout)
	for {
		pods, err := c.evictablePods(ctx, nodeName)
		if err != nil {
			return err
		}
		if len(pods) == 0 {
			return nil
		}
		for _, pod := range pods {
			if err := c.evictPod(ctx, pod, gracePeriodSeconds); err != nil {
				if apierrors.IsNotFound(err) || apierrors.IsTooManyRequests(err) {
					continue
				}
				return err
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out draining node %s; %d evictable pods remain", nodeName, len(pods))
		}
		time.Sleep(5 * time.Second)
	}
}

func (c *Client) evictablePods(ctx context.Context, nodeName string) ([]corev1.Pod, error) {
	pods, err := c.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName).String(),
	})
	if err != nil {
		return nil, err
	}
	evictable := make([]corev1.Pod, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if skipPodForDrain(pod) {
			continue
		}
		evictable = append(evictable, pod)
	}
	return evictable, nil
}

func (c *Client) evictPod(ctx context.Context, pod corev1.Pod, gracePeriodSeconds int64) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds,
		},
	}
	return c.Clientset.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction)
}

func skipPodForDrain(pod corev1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return true
	}
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return true
	}
	if _, ok := pod.Annotations["kubernetes.io/config.mirror"]; ok {
		return true
	}
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

func NodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
