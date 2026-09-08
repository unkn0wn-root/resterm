package k8s

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
)

// clusterAPI is the part of the Kubernetes API resterm reads while resolving a
// port-forward target.
type clusterAPI interface {
	getPod(ctx context.Context, ns, name string) (*corev1.Pod, error)
	listPods(ctx context.Context, ns, selector string) ([]corev1.Pod, error)
	getService(ctx context.Context, ns, name string) (*corev1.Service, error)
	getDeployment(ctx context.Context, ns, name string) (*appsv1.Deployment, error)
	getStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error)
	portForwardURL(ns, pod string) *url.URL
}

// Registering only the two groups above keeps client-go's generated clientsets
// out of the build. Those import the shared scheme, which registers every
// Kubernetes API group and adds about 15 MiB to the binary. metav1 belongs here
// too, otherwise API errors do not decode into *apierrors.StatusError and
// apierrors.IsNotFound stops recognising a missing pod.
var (
	scheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(scheme)
	params = runtime.NewParameterCodec(scheme)
)

func init() {
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, appsv1.AddToScheme} {
		if err := add(scheme); err != nil {
			panic(err)
		}
	}
	metav1.AddToGroupVersion(scheme, corev1.SchemeGroupVersion)
	metav1.AddToGroupVersion(scheme, appsv1.SchemeGroupVersion)
}

type restAPI struct {
	core rest.Interface
	apps rest.Interface
}

func newRESTAPI(cfg *rest.Config) (*restAPI, error) {
	if cfg == nil {
		return nil, errors.New("missing rest config")
	}

	shared := *cfg
	if shared.UserAgent == "" {
		shared.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	if shared.RateLimiter == nil && shared.QPS > 0 {
		if shared.Burst <= 0 {
			return nil, errors.New(
				"burst is required to be greater than 0 when RateLimiter is not set and QPS is set to greater than 0",
			)
		}
		shared.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(shared.QPS, shared.Burst)
	}

	// One http.Client for both group versions, matching what the generated
	// clientsets do, so connections and the rate limiter are shared.
	httpClient, err := rest.HTTPClientFor(&shared)
	if err != nil {
		return nil, err
	}
	core, err := groupClient(shared, httpClient, corev1.SchemeGroupVersion, "/api")
	if err != nil {
		return nil, err
	}
	apps, err := groupClient(shared, httpClient, appsv1.SchemeGroupVersion, "/apis")
	if err != nil {
		return nil, err
	}
	return &restAPI{core: core, apps: apps}, nil
}

// The core group lives under /api and every other group under /apis.
func groupClient(
	cfg rest.Config,
	httpClient *http.Client,
	gv schema.GroupVersion,
	apiPath string,
) (rest.Interface, error) {
	cfg.GroupVersion = &gv
	cfg.APIPath = apiPath
	cfg.NegotiatedSerializer = codecs.WithoutConversion()
	return rest.RESTClientForConfigAndClient(&cfg, httpClient)
}

func (a *restAPI) getPod(ctx context.Context, ns, name string) (*corev1.Pod, error) {
	out := &corev1.Pod{}
	return out, a.core.Get().
		UseProtobufAsDefault().
		Namespace(ns).
		Resource("pods").
		Name(name).
		Do(ctx).
		Into(out)
}

func (a *restAPI) listPods(ctx context.Context, ns, selector string) ([]corev1.Pod, error) {
	out := &corev1.PodList{}
	err := a.core.Get().
		UseProtobufAsDefault().
		Namespace(ns).
		Resource("pods").
		VersionedParams(&metav1.ListOptions{LabelSelector: selector}, params).
		Do(ctx).
		Into(out)
	if err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (a *restAPI) getService(ctx context.Context, ns, name string) (*corev1.Service, error) {
	out := &corev1.Service{}
	return out, a.core.Get().
		UseProtobufAsDefault().
		Namespace(ns).
		Resource("services").
		Name(name).
		Do(ctx).
		Into(out)
}

func (a *restAPI) getDeployment(ctx context.Context, ns, name string) (*appsv1.Deployment, error) {
	out := &appsv1.Deployment{}
	return out, a.apps.Get().
		UseProtobufAsDefault().
		Namespace(ns).
		Resource("deployments").
		Name(name).
		Do(ctx).
		Into(out)
}

func (a *restAPI) getStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error) {
	out := &appsv1.StatefulSet{}
	return out, a.apps.Get().
		UseProtobufAsDefault().
		Namespace(ns).
		Resource("statefulsets").
		Name(name).
		Do(ctx).
		Into(out)
}

func (a *restAPI) portForwardURL(ns, pod string) *url.URL {
	return a.core.Post().
		Resource("pods").
		Namespace(ns).
		Name(pod).
		SubResource("portforward").
		URL()
}
