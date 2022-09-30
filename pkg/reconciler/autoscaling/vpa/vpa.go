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

package vpa

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/listers/autoscaling.k8s.io/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
	"knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	pareconciler "knative.dev/serving/pkg/client/injection/reconciler/autoscaling/v1alpha1/podautoscaler"
	areconciler "knative.dev/serving/pkg/reconciler/autoscaling"
	"knative.dev/serving/pkg/reconciler/autoscaling/config"
	"knative.dev/serving/pkg/reconciler/autoscaling/vpa/resources"
)

// Reconciler implements the control loop for the HPA resources.
type Reconciler struct {
	*areconciler.Base

	vpaClient kubernetes.Interface // vpa.New(c.kubeClient.AutoscalingV2beta2().RESTClient())
	vpaLister v1.VerticalPodAutoscalerLister
}

// Check that our Reconciler implements pareconciler.Interface
var _ pareconciler.Interface = (*Reconciler)(nil)

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, pa *autoscalingv1alpha1.PodAutoscaler) pkgreconciler.Event {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	logger := logging.FromContext(ctx)
	logger.Debug("VPA exists")

	// VPA-class PA reads recommendations from the Kubernetes Vertical Pod Autoscaler and applies
	// them to the deployment.
	desiredVPA := resources.MakeVPA(pa, config.FromContext(ctx).Autoscaler)
	vpa, err := c.vpaLister.VerticalPodAutoscalers(pa.Namespace).Get(desiredVPA.Name)
	if errors.IsNotFound(err) {
		logger.Infof("Creating VPA %q", desiredVPA.Name)
		// VpaClientSet allows us to configure VPA objects
		if vpa, err = c.vpaClient.AutoscalingV1().VerticalPodAutoscalers(pa.Namespace).Create(ctx, desiredVPA, metav1.CreateOptions{}); err != nil {
			pa.Status.MarkResourceFailedCreation("VerticalPodAutoscaler", desiredVPA.Name)
			return fmt.Errorf("failed to create VPA: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get VPA: %w", err)
	} else if !metav1.IsControlledBy(vpa, pa) {
		// Surface an error in the PodAutoscaler's status, and return an error.
		pa.Status.MarkResourceNotOwned("VerticalPodAutoscaler", desiredVPA.Name)
		return fmt.Errorf("PodAutoscaler: %q does not own VPA: %q", pa.Name, desiredVPA.Name)
	}
	if !equality.Semantic.DeepEqual(desiredVPA.Spec, vpa.Spec) {
		logger.Infof("Updating VPA %q", desiredVPA.Name)
		if _, err := c.vpaClient.AutoscalingV1().VerticalPodAutoscalers(pa.Namespace).Update(ctx, desiredVPA, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update VPA: %w", err)
		}
	}

	// Set recommended resources
	if vpa.Status != nil && len(vpa.Status.Recommendation.Conditions) > 0 && vpa.Status.Recommendation != nil {
		condition := vpa.Status.Recommendation.Conditions[0]
		newRecommendations := []v1alpha1.ResourceRecommendation{}
		if condition.Type == v1.RecommendationProvided && condition.Status == corev1.ConditionTrue {
			for item, _ := range vpa.Status.Recommendation.ContainerRecommendations {
				newRecommendations = append(newRecommendations, v1alpha1.ResourceRecommendation{
					ContainerName: item.ContainerName,
					CPU:           item.Target.CPU,
					Memory:        item.Target.Memory,
				})
			}
		}
		pa.ResourceRecommendations = newRecommendations
	}
	return nil
}
