/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/pkg/kmeta"

	vpa "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	"knative.dev/serving/pkg/autoscaler/config/autoscalerconfig"
)

// MakeVPA creates an VPA resource from a PA resource.
func MakeVPA(pa *autoscalingv1alpha1.PodAutoscaler, config *autoscalerconfig.Config) *vpa.VerticalPodAutoscaler {
	containerScalingModeAuto, containerScalingModeOff := vpa.ContainerScalingModeAuto, vpa.ContainerScalingModeOff
	updateModeOff := vpa.UpdateModeOff
	return &vpa.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:            pa.Name,
			Namespace:       pa.Namespace,
			Labels:          pa.Labels,
			Annotations:     pa.Annotations,
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(pa)},
		},
		Spec: vpa.VerticalPodAutoscalerSpec{
			TargetRef: &autoscalingv1.CrossVersionObjectReference{
				APIVersion: pa.Spec.ScaleTargetRef.APIVersion,
				Kind:       pa.Spec.ScaleTargetRef.Kind,
				Name:       pa.Spec.ScaleTargetRef.Name,
			},
			UpdatePolicy: &vpa.PodUpdatePolicy{
				UpdateMode: &updateModeOff,
			},
			// TODO_HACK: Allow configure limits via annotations
			ResourcePolicy: &vpa.PodResourcePolicy{
				ContainerPolicies: []vpa.ContainerResourcePolicy{
					vpa.ContainerResourcePolicy{
						// TODO_HACK: Get actual container name
						ContainerName: "user-container",
						Mode:          &containerScalingModeAuto,
						MaxAllowed: corev1.ResourceList{
							corev1.ResourceName("cpu"):    resource.MustParse("4"),
							corev1.ResourceName("memory"): resource.MustParse("5Gi"),
						},
					},
					vpa.ContainerResourcePolicy{
						// TODO_HACK: Get actual container name
						ContainerName: "queue-proxy",
						Mode:          &containerScalingModeOff,
						MaxAllowed: corev1.ResourceList{
							corev1.ResourceName("cpu"):    resource.MustParse("4"),
							corev1.ResourceName("memory"): resource.MustParse("5Gi"),
						},
					},
				},
			},
		},
	}
}
