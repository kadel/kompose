/*
Copyright 2016 Skippbox, Ltd All rights reserved.

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

package kubernetes

import (
	"fmt"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/skippbox/kompose/pkg/kobject"
	"github.com/skippbox/kompose/pkg/transformer"

	"k8s.io/client-go/1.5/kubernetes"
	v1 "k8s.io/client-go/1.5/pkg/api/v1"
	v1beta1 "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/util/intstr"
	"k8s.io/client-go/1.5/tools/clientcmd"
)

type Kubernetes struct {
}

// Init RC object
func InitRC(name string, service kobject.ServiceConfig, replicas int32) *v1.ReplicationController {
	rc := &v1.ReplicationController{

		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		Spec: v1.ReplicationControllerSpec{
			Replicas: &replicas,
			Template: &v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: transformer.ConfigLabels(name),
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  name,
							Image: service.Image,
						},
					},
				},
			},
		},
	}
	return rc
}

// Init Svc object
func InitSvc(name string, service kobject.ServiceConfig) *v1.Service {
	svc := &v1.Service{

		ObjectMeta: v1.ObjectMeta{
			Name:   name,
			Labels: transformer.ConfigLabels(name),
		},
		Spec: v1.ServiceSpec{
			Selector: transformer.ConfigLabels(name),
		},
	}
	return svc
}

// Init Deployment
func InitD(name string, service kobject.ServiceConfig, replicas int32) *v1beta1.Deployment {
	dc := &v1beta1.Deployment{

		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		Spec: v1beta1.DeploymentSpec{
			Replicas: &replicas,
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  name,
							Image: service.Image,
						},
					},
				},
			},
		},
	}
	return dc
}

// Init DS object
func InitDS(name string, service kobject.ServiceConfig) *v1beta1.DaemonSet {
	ds := &v1beta1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		Spec: v1beta1.DaemonSetSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  name,
							Image: service.Image,
						},
					},
				},
			},
		},
	}
	return ds
}

// Configure the container ports.
func ConfigPorts(name string, service kobject.ServiceConfig) []v1.ContainerPort {
	ports := []v1.ContainerPort{}
	for _, port := range service.Port {
		ports = append(ports, v1.ContainerPort{
			ContainerPort: port.ContainerPort,
			Protocol:      v1.Protocol(port.Protocol),
		})
	}

	return ports
}

// Configure the container service ports.
func ConfigServicePorts(name string, service kobject.ServiceConfig) []v1.ServicePort {
	servicePorts := []v1.ServicePort{}
	for _, port := range service.Port {
		if port.HostPort == 0 {
			port.HostPort = port.ContainerPort
		}
		var targetPort intstr.IntOrString
		targetPort.IntVal = port.ContainerPort
		targetPort.StrVal = strconv.Itoa(int(port.ContainerPort))
		servicePorts = append(servicePorts, v1.ServicePort{
			Name:       strconv.Itoa(int(port.HostPort)),
			Protocol:   v1.Protocol(port.Protocol),
			Port:       port.HostPort,
			TargetPort: targetPort,
		})
	}
	return servicePorts
}

// Configure the container volumes.
func ConfigVolumes(service kobject.ServiceConfig) ([]v1.VolumeMount, []v1.Volume) {
	volumesMount := []v1.VolumeMount{}
	volumes := []v1.Volume{}
	volumeSource := v1.VolumeSource{}
	for _, volume := range service.Volumes {
		name, host, container, mode, err := transformer.ParseVolume(volume)
		if err != nil {
			logrus.Warningf("Failed to configure container volume: %v", err)
			continue
		}

		// if volume name isn't specified, set it to a random string of 20 chars
		if len(name) == 0 {
			name = transformer.RandStringBytes(20)
		}
		// check if ro/rw mode is defined, default rw
		readonly := len(mode) > 0 && mode == "ro"

		volumesMount = append(volumesMount, v1.VolumeMount{Name: name, ReadOnly: readonly, MountPath: container})

		if len(host) > 0 {
			logrus.Warningf("Volume mount on the host %q isn't supported - ignoring path on the host", host)
		}
		volumeSource = v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}

		volumes = append(volumes, v1.Volume{Name: name, VolumeSource: volumeSource})
	}
	return volumesMount, volumes
}

// Configure the environment variables.
func ConfigEnvs(name string, service kobject.ServiceConfig) []v1.EnvVar {
	envs := []v1.EnvVar{}
	for _, v := range service.Environment {
		envs = append(envs, v1.EnvVar{
			Name:  v.Name,
			Value: v.Value,
		})
	}

	return envs
}

// Generate a Kubernetes artifact for each input type service
func CreateKubernetesObjects(name string, service kobject.ServiceConfig, opt kobject.ConvertOptions) []runtime.Object {
	var objects []runtime.Object

	if opt.CreateD {
		objects = append(objects, InitD(name, service, opt.Replicas))
	}
	if opt.CreateDS {
		objects = append(objects, InitDS(name, service))
	}
	if opt.CreateRC {
		objects = append(objects, InitRC(name, service, opt.Replicas))
	}

	return objects
}

// Transform maps komposeObject to k8s objects
// returns object that are already sorted in the way that Services are first
func (k *Kubernetes) Transform(komposeObject kobject.KomposeObject, opt kobject.ConvertOptions) []runtime.Object {
	// this will hold all the converted data
	var allobjects []runtime.Object

	for name, service := range komposeObject.ServiceConfigs {
		objects := CreateKubernetesObjects(name, service, opt)

		// If ports not provided in configuration we will not make service
		if PortsExist(name, service) {
			svc := CreateService(name, service, objects)
			objects = append(objects, svc)
		}

		UpdateKubernetesObjects(name, service, objects)

		allobjects = append(allobjects, objects...)
	}
	// sort all object so Services are first
	SortServicesFirst(&allobjects)
	return allobjects
}

// Updates the given object with the given pod template update function and ObjectMeta update function
func UpdateController(obj runtime.Object, updateTemplate func(*v1.PodTemplateSpec), updateMeta func(meta *v1.ObjectMeta)) {
	switch t := obj.(type) {
	case *v1.ReplicationController:
		if t.Spec.Template == nil {
			t.Spec.Template = &v1.PodTemplateSpec{}
		}
		updateTemplate(t.Spec.Template)
		updateMeta(&t.ObjectMeta)
	case *v1beta1.Deployment:
		updateTemplate(&t.Spec.Template)
		updateMeta(&t.ObjectMeta)
	case *v1beta1.DaemonSet:
		updateTemplate(&t.Spec.Template)
		updateMeta(&t.ObjectMeta)
	}
}

// Submit deployment and svc to k8s endpoint
func (k *Kubernetes) Deploy(komposeObject kobject.KomposeObject, opt kobject.ConvertOptions) error {
	//Convert komposeObject
	objects := k.Transform(komposeObject, opt)

	fmt.Println("We are going to create Kubernetes deployments and services for your Dockerized application. \n" +
		"If you need different kind of resources, use the 'kompose convert' and 'kubectl create -f' commands instead. \n")

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// if you want to change the loading rules (which files in which order), you can do so here

	configOverrides := &clientcmd.ConfigOverrides{}
	// if you want to change override values or bind them to flags, there are methods to help you

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return err
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	client := clientset.Core()

	for _, v := range objects {
		switch t := v.(type) {
		case *v1beta1.Deployment:
			_, err := client.Deployments(namespace).Create(t)
			if err != nil {
				return err
			}
			logrus.Infof("Successfully created deployment: %s", t.Name)
		case *v1.Service:
			_, err := client.Services(namespace).Create(t)
			if err != nil {
				return err
			}
			logrus.Infof("Successfully created service: %s", t.Name)
		}
	}
	fmt.Println("\nYour application has been deployed to Kubernetes. You can run 'kubectl get deployment,svc,pods' for details.")

	return nil
}

func (k *Kubernetes) Undeploy(komposeObject kobject.KomposeObject, opt kobject.ConvertOptions) error {

	//	factory := cmdutil.NewFactory(nil)
	//	clientConfig, err := factory.ClientConfig()
	//	if err != nil {
	//		return err
	//	}
	//	namespace, _, err := factory.DefaultNamespace()
	//	if err != nil {
	//		return err
	//	}
	//	client := client.NewOrDie(clientConfig)
	//
	//	// delete objects  from kubernetes
	//	for name := range komposeObject.ServiceConfigs {
	//		//delete svc
	//		rpService, err := kubectl.ReaperFor(v1.Kind("Service"), client)
	//		if err != nil {
	//			return err
	//		}
	//		//FIXME: timeout = 300s, gracePeriod is nil
	//		err = rpService.Stop(namespace, name, 300*time.Second, nil)
	//		if err != nil {
	//			return err
	//		} else {
	//			logrus.Infof("Successfully deleted service: %s", name)
	//		}
	//
	//		//delete deployment
	//		rpDeployment, err := kubectl.ReaperFor(v1beta1.Kind("Deployment"), client)
	//		if err != nil {
	//			return err
	//		}
	//		//FIXME: timeout = 300s, gracePeriod is nil
	//		err = rpDeployment.Stop(namespace, name, 300*time.Second, nil)
	//		if err != nil {
	//			return err
	//		} else {
	//			logrus.Infof("Successfully deleted deployment: %s", name)
	//		}
	//	}
	return nil
}
