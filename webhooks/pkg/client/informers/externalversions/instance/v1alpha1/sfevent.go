/*
Copyright The Kubernetes Authors.

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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	time "time"

	instancev1alpha1 "github.com/cloudfoundry-incubator/service-fabrik-broker/webhooks/pkg/apis/instance/v1alpha1"
	versioned "github.com/cloudfoundry-incubator/service-fabrik-broker/webhooks/pkg/client/clientset/versioned"
	internalinterfaces "github.com/cloudfoundry-incubator/service-fabrik-broker/webhooks/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/cloudfoundry-incubator/service-fabrik-broker/webhooks/pkg/client/listers/instance/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// SfeventInformer provides access to a shared informer and lister for
// Sfevents.
type SfeventInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.SfeventLister
}

type sfeventInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewSfeventInformer constructs a new informer for Sfevent type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewSfeventInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredSfeventInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredSfeventInformer constructs a new informer for Sfevent type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredSfeventInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SamplecontrollerV1alpha1().Sfevents(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SamplecontrollerV1alpha1().Sfevents(namespace).Watch(options)
			},
		},
		&instancev1alpha1.Sfevent{},
		resyncPeriod,
		indexers,
	)
}

func (f *sfeventInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredSfeventInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *sfeventInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&instancev1alpha1.Sfevent{}, f.defaultInformer)
}

func (f *sfeventInformer) Lister() v1alpha1.SfeventLister {
	return v1alpha1.NewSfeventLister(f.Informer().GetIndexer())
}
