package k8s

import (
	"context"
	"net/http"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/cbor"
	"k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	"k8s.io/client-go/features"
	featuretesting "k8s.io/client-go/features/testing"
)

func TestRESTAPIReadsWithCBORPreferences(t *testing.T) {
	featuretesting.SetFeatureDuringTest(t, features.ClientsAllowCBOR, true)
	featuretesting.SetFeatureDuringTest(t, features.ClientsPreferCBOR, true)

	cases := []struct {
		name string
		body runtime.Object
		read func(context.Context, *restAPI) error
	}{
		{
			name: "pod",
			body: &corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}},
			read: func(ctx context.Context, api *restAPI) error {
				_, err := api.getPod(ctx, "prod", "web")
				return err
			},
		},
		{
			name: "pods",
			body: &corev1.PodList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"}},
			read: func(ctx context.Context, api *restAPI) error {
				_, err := api.listPods(ctx, "prod", "app=web")
				return err
			},
		},
		{
			name: "service",
			body: &corev1.Service{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"}},
			read: func(ctx context.Context, api *restAPI) error {
				_, err := api.getService(ctx, "prod", "web")
				return err
			},
		},
		{
			name: "deployment",
			body: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
			},
			read: func(ctx context.Context, api *restAPI) error {
				_, err := api.getDeployment(ctx, "prod", "web")
				return err
			},
		},
		{
			name: "statefulset",
			body: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"},
			},
			read: func(ctx context.Context, api *restAPI) error {
				_, err := api.getStatefulSet(ctx, "prod", "web")
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newTestRESTAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				accept := r.Header.Get("Accept")
				if want := "application/vnd.kubernetes.protobuf,application/json"; accept != want {
					t.Errorf("Accept = %q, want %q", accept, want)
				}

				var encoder runtime.Encoder
				switch {
				case strings.HasPrefix(accept, "application/cbor"):
					w.Header().Set("Content-Type", "application/cbor")
					encoder = cbor.NewSerializer(scheme, scheme)
				case strings.HasPrefix(accept, "application/vnd.kubernetes.protobuf"):
					w.Header().Set("Content-Type", "application/vnd.kubernetes.protobuf")
					encoder = protobuf.NewSerializer(scheme, scheme)
				default:
					writeJSON(t, w, http.StatusOK, tc.body)
					return
				}
				if err := encoder.Encode(tc.body, w); err != nil {
					t.Errorf("encode response: %v", err)
				}
			}))
			if err := tc.read(t.Context(), api); err != nil {
				t.Fatalf("read with CBOR preferences: %v", err)
			}
		})
	}
}
