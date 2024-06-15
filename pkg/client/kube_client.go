package client

import (
	"fmt"

	admiral "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"

	argo "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned"
	log "github.com/sirupsen/logrus"
	istio "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type kubeClientLoader struct{}

// Singleton
var kubeClient = &kubeClientLoader{}

func GetKubeClientLoader() ClientLoader {
	return kubeClient
}

func (loader *kubeClientLoader) LoadAdmiralClientFromPath(kubeConfigPath string) (admiral.Interface, error) {
	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}

	return loader.LoadAdmiralClientFromConfig(config)
}

func (*kubeClientLoader) LoadAdmiralClientFromConfig(config *rest.Config) (admiral.Interface, error) {
	return admiral.NewForConfig(config)
}

func (loader *kubeClientLoader) LoadIstioClientFromPath(kubeConfigPath string) (istio.Interface, error) {
	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}

	return loader.LoadIstioClientFromConfig(config)
}

func (loader *kubeClientLoader) LoadIstioClientFromConfig(config *rest.Config) (istio.Interface, error) {
	return istio.NewForConfig(config)
}

func (loader *kubeClientLoader) LoadArgoClientFromPath(kubeConfigPath string) (argo.Interface, error) {
	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}

	return loader.LoadArgoClientFromConfig(config)
}

func (loader *kubeClientLoader) LoadArgoClientFromConfig(config *rest.Config) (argo.Interface, error) {
	return argo.NewForConfig(config)
}

func (loader *kubeClientLoader) LoadKubeClientFromPath(kubeConfigPath string) (kubernetes.Interface, error) {
	config, err := getConfig(kubeConfigPath)
	if err != nil || config == nil {
		return nil, err
	}

	return loader.LoadKubeClientFromConfig(config)
}

func (loader *kubeClientLoader) LoadKubeClientFromConfig(config *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(config)
}

func getConfig(kubeConfigPath string) (*rest.Config, error) {
	log.Infof("getting kubeconfig from: %#v", kubeConfigPath)
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)

	if err != nil || config == nil {
		return nil, fmt.Errorf("could not retrieve kubeconfig: %v", err)
	}
	return config, err
}
