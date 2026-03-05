package k8s

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

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

func resolveForwardTarget(
	ctx context.Context,
	apps appsv1client.AppsV1Interface,
	core corev1client.CoreV1Interface,
	namespace string,
	cfg Cfg,
) (forwardTarget, error) {
	target, err := waitTargetPod(ctx, apps, core, namespace, cfg)
	if err != nil {
		return forwardTarget{}, err
	}

	port, err := resolveRemotePort(cfg, target.pod, target.svc)
	if err != nil {
		return forwardTarget{}, err
	}
	return forwardTarget{pod: target.pod.Name, port: port}, nil
}

func waitTargetPod(
	ctx context.Context,
	apps appsv1client.AppsV1Interface,
	core corev1client.CoreV1Interface,
	namespace string,
	cfg Cfg,
) (selectedTarget, error) {
	if core == nil {
		return selectedTarget{}, errors.New("k8s: client unavailable")
	}
	if namespace == "" {
		return selectedTarget{}, errors.New("k8s: namespace is required")
	}

	kind, name := cfg.target()
	if kind == "" {
		return selectedTarget{}, errors.New("k8s: target kind is required")
	}
	if name == "" {
		return selectedTarget{}, errors.New("k8s: target name is required")
	}

	var out selectedTarget
	check := func(ctx context.Context) (bool, error) {
		target, err := selectTargetPod(ctx, apps, core, namespace, kind, name)
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
			if kind != targetKindPod {
				return false, nil
			}
			return false, fmt.Errorf(
				"k8s: pod %s/%s is %s",
				namespace,
				target.pod.Name,
				strings.ToLower(string(target.pod.Status.Phase)),
			)
		default:
			return false, nil
		}
	}

	targetRef := targetID(kind, name)
	if cfg.PodWait <= 0 {
		ok, err := check(ctx)
		if err != nil {
			return selectedTarget{}, fmt.Errorf("k8s: check target %s/%s: %w", namespace, targetRef, err)
		}
		if !ok {
			return selectedTarget{}, fmt.Errorf(
				"k8s: target %s/%s has no running pods",
				namespace,
				targetRef,
			)
		}
		return out, nil
	}

	if err := wait.PollUntilContextTimeout(ctx, podPollInterval, cfg.PodWait, true, check); err != nil {
		return selectedTarget{}, fmt.Errorf(
			"k8s: wait target %s/%s running: %w",
			namespace,
			targetRef,
			err,
		)
	}
	return out, nil
}

func selectTargetPod(
	ctx context.Context,
	apps appsv1client.AppsV1Interface,
	core corev1client.CoreV1Interface,
	namespace string,
	kind TargetKind,
	name string,
) (selectedTarget, error) {
	switch kind {
	case targetKindPod:
		pod, err := core.Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return selectedTarget{}, nil
			}
			return selectedTarget{}, err
		}
		return selectedTarget{pod: pod}, nil

	case targetKindService:
		svc, err := core.Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return selectedTarget{}, nil
			}
			return selectedTarget{}, err
		}
		pods, err := podsForService(ctx, core, namespace, svc)
		if err != nil {
			return selectedTarget{}, err
		}
		return selectedTarget{pod: pickPod(pods), svc: svc}, nil

	case targetKindDeployment:
		if apps == nil {
			return selectedTarget{}, errors.New("k8s: client unavailable")
		}
		deployment, err := apps.Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return selectedTarget{}, nil
			}
			return selectedTarget{}, err
		}
		return targetBySelector(
			ctx,
			core,
			namespace,
			deployment.Spec.Selector,
			"deployment",
			namespace,
			deployment.Name,
		)

	case targetKindStatefulSet:
		if apps == nil {
			return selectedTarget{}, errors.New("k8s: client unavailable")
		}
		statefulSet, err := apps.StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return selectedTarget{}, nil
			}
			return selectedTarget{}, err
		}
		return targetBySelector(
			ctx,
			core,
			namespace,
			statefulSet.Spec.Selector,
			"statefulset",
			namespace,
			statefulSet.Name,
		)

	default:
		return selectedTarget{}, fmt.Errorf("k8s: unsupported target kind %q", kind)
	}
}

func targetBySelector(
	ctx context.Context,
	core corev1client.CoreV1Interface,
	namespace string,
	selector *metav1.LabelSelector,
	kind, objectNamespace, objectName string,
) (selectedTarget, error) {
	pods, err := podsForLabelSelector(
		ctx,
		core,
		namespace,
		selector,
		kind,
		objectNamespace,
		objectName,
	)
	if err != nil {
		return selectedTarget{}, err
	}
	return selectedTarget{pod: pickPod(pods)}, nil
}

func podsForService(
	ctx context.Context,
	core corev1client.CoreV1Interface,
	namespace string,
	svc *corev1.Service,
) ([]corev1.Pod, error) {
	if svc == nil {
		return nil, errors.New("k8s: service is required")
	}
	if len(svc.Spec.Selector) == 0 {
		return nil, fmt.Errorf("k8s: service %s/%s has no selector", namespace, svc.Name)
	}

	selector := labels.SelectorFromSet(svc.Spec.Selector)
	return listPods(ctx, core, namespace, selector.String())
}

func podsForLabelSelector(
	ctx context.Context,
	core corev1client.CoreV1Interface,
	namespace string,
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
	return listPods(ctx, core, namespace, labelSelector.String())
}

func listPods(
	ctx context.Context,
	core corev1client.CoreV1Interface,
	namespace string,
	selector string,
) ([]corev1.Pod, error) {
	pods, err := core.Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
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
