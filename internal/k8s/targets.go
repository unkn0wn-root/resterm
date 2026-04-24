package k8s

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

type forwardTarget struct {
	pod  string
	port int
}

type selectedTarget struct {
	pod *corev1.Pod
	svc *corev1.Service
}

type clusterResolver struct {
	apps      appsv1client.AppsV1Interface
	core      corev1client.CoreV1Interface
	namespace string
}

func newClusterResolver(clients clusterClients, namespace string) clusterResolver {
	return clusterResolver{
		apps:      clients.apps,
		core:      clients.core,
		namespace: namespace,
	}
}

func (r clusterResolver) resolveForwardTarget(
	ctx context.Context,
	cfg execConfig,
) (forwardTarget, error) {
	target, err := r.waitTargetPod(ctx, cfg.Target, cfg.PodWait)
	if err != nil {
		return forwardTarget{}, err
	}

	port, err := resolveRemotePort(cfg, target.pod, target.svc)
	if err != nil {
		return forwardTarget{}, err
	}
	return forwardTarget{pod: target.pod.Name, port: port}, nil
}

func (r clusterResolver) waitTargetPod(
	ctx context.Context,
	target TargetRef,
	podWait time.Duration,
) (selectedTarget, error) {
	if r.core == nil {
		return selectedTarget{}, errors.New("k8s: client unavailable")
	}
	if strings.TrimSpace(r.namespace) == "" {
		return selectedTarget{}, errors.New("k8s: namespace is required")
	}

	kind := target.Kind
	name := target.Name
	switch {
	case kind == "":
		return selectedTarget{}, errors.New("k8s: target kind is required")
	case name == "":
		return selectedTarget{}, errors.New("k8s: target name is required")
	}

	var out selectedTarget
	check := func(ctx context.Context) (bool, error) {
		target, err := r.selectTargetPod(ctx, kind, name)
		if err != nil {
			return false, err
		}
		if target.pod == nil {
			return false, nil
		}

		switch target.pod.Status.Phase {
		case corev1.PodRunning:
			out = target
			return true, nil
		case corev1.PodFailed, corev1.PodSucceeded:
			if kind != TargetPod {
				return false, nil
			}
			return false, fmt.Errorf(
				"k8s: pod %s/%s is %s",
				r.namespace,
				target.pod.Name,
				strings.ToLower(string(target.pod.Status.Phase)),
			)
		default:
			return false, nil
		}
	}

	ref := targetID(kind, name)
	if podWait <= 0 {
		ok, err := check(ctx)
		if err != nil {
			return selectedTarget{}, fmt.Errorf(
				"k8s: check target %s/%s: %w",
				r.namespace,
				ref,
				err,
			)
		}
		if !ok {
			return selectedTarget{}, fmt.Errorf(
				"k8s: target %s/%s has no running pods",
				r.namespace,
				ref,
			)
		}
		return out, nil
	}

	if err := wait.PollUntilContextTimeout(
		ctx,
		podPollInterval,
		podWait,
		true,
		check,
	); err != nil {
		return selectedTarget{}, fmt.Errorf(
			"k8s: wait target %s/%s running: %w",
			r.namespace,
			ref,
			err,
		)
	}
	return out, nil
}

func (r clusterResolver) selectTargetPod(
	ctx context.Context,
	kind TargetKind,
	name string,
) (selectedTarget, error) {
	switch kind {
	case TargetPod:
		pod, err := r.core.Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return selectedTarget{}, nil
			}
			return selectedTarget{}, err
		}
		return selectedTarget{pod: pod}, nil

	case TargetService:
		svc, err := r.core.Services(r.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return selectedTarget{}, nil
			}
			return selectedTarget{}, err
		}
		pods, err := r.podsForService(ctx, svc)
		if err != nil {
			return selectedTarget{}, err
		}
		return selectedTarget{pod: pickPod(pods), svc: svc}, nil

	case TargetDeployment:
		return r.resolveWorkload(
			ctx,
			name,
			"deployment",
			func(ctx context.Context, name string) (*metav1.LabelSelector, string, error) {
				deploy, err := r.apps.Deployments(r.namespace).Get(ctx, name, metav1.GetOptions{})
				if err != nil {
					return nil, "", err
				}
				return deploy.Spec.Selector, deploy.Name, nil
			},
		)

	case TargetStatefulSet:
		return r.resolveWorkload(
			ctx,
			name,
			"statefulset",
			func(ctx context.Context, name string) (*metav1.LabelSelector, string, error) {
				sts, err := r.apps.StatefulSets(r.namespace).Get(ctx, name, metav1.GetOptions{})
				if err != nil {
					return nil, "", err
				}
				return sts.Spec.Selector, sts.Name, nil
			},
		)

	default:
		return selectedTarget{}, fmt.Errorf("k8s: unsupported target kind %q", kind)
	}
}

func (r clusterResolver) resolveWorkload(
	ctx context.Context,
	name string,
	kind string,
	getSelector func(context.Context, string) (*metav1.LabelSelector, string, error),
) (selectedTarget, error) {
	if r.apps == nil {
		return selectedTarget{}, errors.New("k8s: client unavailable")
	}

	selector, objectName, err := getSelector(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return selectedTarget{}, nil
		}
		return selectedTarget{}, err
	}
	return r.targetBySelector(ctx, selector, kind, r.namespace, objectName)
}

func (r clusterResolver) targetBySelector(
	ctx context.Context,
	selector *metav1.LabelSelector,
	kind, objectNamespace, objectName string,
) (selectedTarget, error) {
	pods, err := r.podsForLabelSelector(ctx, selector, kind, objectNamespace, objectName)
	if err != nil {
		return selectedTarget{}, err
	}
	return selectedTarget{pod: pickPod(pods)}, nil
}

func (r clusterResolver) podsForService(
	ctx context.Context,
	svc *corev1.Service,
) ([]corev1.Pod, error) {
	if svc == nil {
		return nil, errors.New("k8s: service is required")
	}
	if len(svc.Spec.Selector) == 0 {
		return nil, fmt.Errorf("k8s: service %s/%s has no selector", r.namespace, svc.Name)
	}

	selector := labels.SelectorFromSet(svc.Spec.Selector)
	return r.listPods(ctx, selector.String())
}

func (r clusterResolver) podsForLabelSelector(
	ctx context.Context,
	selector *metav1.LabelSelector,
	kind, objectNamespace, objectName string,
) ([]corev1.Pod, error) {
	if selector == nil {
		return nil, fmt.Errorf("k8s: %s %s/%s has no selector", kind, objectNamespace, objectName)
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fmt.Errorf(
			"k8s: %s %s/%s selector: %w",
			kind,
			objectNamespace,
			objectName,
			err,
		)
	}
	if labelSelector.Empty() {
		return nil, fmt.Errorf(
			"k8s: %s %s/%s has empty selector",
			kind,
			objectNamespace,
			objectName,
		)
	}
	return r.listPods(ctx, labelSelector.String())
}

func (r clusterResolver) listPods(ctx context.Context, selector string) ([]corev1.Pod, error) {
	pods, err := r.core.Pods(r.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if pods == nil || len(pods.Items) == 0 {
		return nil, nil
	}
	return pods.Items, nil
}

func pickPod(pods []corev1.Pod) *corev1.Pod {
	if len(pods) == 0 {
		return nil
	}

	active := make([]corev1.Pod, 0, len(pods))
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil {
			continue
		}
		active = append(active, pod)
	}
	if len(active) == 0 {
		return nil
	}

	slices.SortFunc(active, func(a, b corev1.Pod) int {
		left, right := podRank(a), podRank(b)
		if left != right {
			if left < right {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})

	selected := active[0]
	return &selected
}

func podRank(pod corev1.Pod) int {
	const (
		rankRunningReady = iota
		rankRunningNotReady
		rankPending
		rankUnknown
		rankOther
	)

	switch pod.Status.Phase {
	case corev1.PodRunning:
		if podReady(pod.Status.Conditions) {
			return rankRunningReady
		}
		return rankRunningNotReady
	case corev1.PodPending:
		return rankPending
	case corev1.PodUnknown:
		return rankUnknown
	default:
		return rankOther
	}
}

func podReady(conditions []corev1.PodCondition) bool {
	for _, condition := range conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func targetID(kind TargetKind, name string) string {
	return string(kind) + "/" + name
}
