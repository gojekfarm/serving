/*
Copyright 2023 The Knative Authors

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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	vpa "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	vpainformers "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions"
	networkingclient "knative.dev/networking/pkg/client/injection/client"
	"knative.dev/pkg/logging"

	"knative.dev/pkg/injection"
	servingclient "knative.dev/serving/pkg/client/injection/client"
	painformer "knative.dev/serving/pkg/client/injection/informers/autoscaling/v1alpha1/podautoscaler"
	pareconciler "knative.dev/serving/pkg/client/injection/reconciler/autoscaling/v1alpha1/podautoscaler"
	"knative.dev/serving/pkg/deployment"

	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	pkgreconciler "knative.dev/pkg/reconciler"
	"knative.dev/serving/pkg/apis/autoscaling"
	autoscalingv1alpha1 "knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	"knative.dev/serving/pkg/autoscaler/config/autoscalerconfig"
	areconciler "knative.dev/serving/pkg/reconciler/autoscaling"
	"knative.dev/serving/pkg/reconciler/autoscaling/config"
)

// NewController returns a new VPA reconcile controller.
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)
	paInformer := painformer.Get(ctx)

	cfg := injection.GetConfig(ctx)
	vpaClient := vpa.NewForConfigOrDie(cfg)
	vpaInformerFactory := vpainformers.NewSharedInformerFactory(vpaClient, time.Second*30)
	vpaInformer := vpaInformerFactory.Autoscaling().V1().VerticalPodAutoscalers()

	onlyVPAClass := func(obj interface{}) bool {
		if mo, ok := obj.(metav1.Object); ok {
			_, ok := mo.GetAnnotations()[autoscaling.VPAAnnotationKey]
			return ok
		}
		return false
	}

	c := &Reconciler{
		Base: &areconciler.Base{
			Client:           servingclient.Get(ctx),
			NetworkingClient: networkingclient.Get(ctx),
		},
		vpaClient: vpaClient,
	}
	impl := pareconciler.NewImpl(ctx, c, autoscaling.KPA, func(impl *controller.Impl) controller.Options {
		logger.Info("Setting up ConfigMap receivers")
		configsToResync := []interface{}{
			&autoscalerconfig.Config{},
			&deployment.Config{},
		}
		resync := configmap.TypeFilter(configsToResync...)(func(string, interface{}) {
			impl.FilteredGlobalResync(onlyVPAClass, paInformer.Informer())
		})
		configStore := config.NewStore(logger.Named("config-store"), resync)
		configStore.WatchConfigs(cmw)
		return controller.Options{ConfigStore: configStore}
	})

	logger.Info("Setting up vpa-class event handlers")

	paInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: onlyVPAClass,
		Handler:    controller.HandleAll(impl.Enqueue),
	})

	onlyPAControlled := controller.FilterController(&autoscalingv1alpha1.PodAutoscaler{})
	handleMatchingControllers := cache.FilteringResourceEventHandler{
		FilterFunc: pkgreconciler.ChainFilterFuncs(onlyVPAClass, onlyPAControlled),
		Handler:    controller.HandleAll(impl.EnqueueControllerOf),
	}
	vpaInformer.Informer().AddEventHandler(handleMatchingControllers)

	return impl
}
