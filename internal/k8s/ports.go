package k8s

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func resolveRemotePort(cfg execConfig, pod *corev1.Pod, svc *corev1.Service) (int, error) {
	if pod == nil {
		return 0, errors.New("k8s: pod is required")
	}
	if svc == nil {
		return resolvePodPort(cfg, pod)
	}
	return resolveServicePort(cfg, pod, svc)
}

func resolvePodPort(cfg execConfig, pod *corev1.Pod) (int, error) {
	if cfg.Port.Number > 0 {
		return cfg.Port.Number, nil
	}
	return podPortByName(pod, cfg.Container, cfg.Port.Name)
}

func resolveServicePort(cfg execConfig, pod *corev1.Pod, svc *corev1.Service) (int, error) {
	servicePort, err := servicePortByConfig(cfg, svc)
	if err != nil {
		return 0, err
	}
	return serviceTargetPort(servicePort, pod, cfg.Container)
}

func servicePortByConfig(cfg execConfig, svc *corev1.Service) (corev1.ServicePort, error) {
	if svc == nil {
		return corev1.ServicePort{}, errors.New("k8s: service is required")
	}
	if len(svc.Spec.Ports) == 0 {
		return corev1.ServicePort{}, fmt.Errorf(
			"k8s: service %s/%s has no ports",
			svc.Namespace,
			svc.Name,
		)
	}

	if cfg.Port.Number > 0 {
		var matches []corev1.ServicePort
		for _, port := range svc.Spec.Ports {
			if int(port.Port) == cfg.Port.Number {
				matches = append(matches, port)
			}
		}
		return pickServicePort(matches, svc, strconv.Itoa(cfg.Port.Number))
	}

	if cfg.Port.Name == "" {
		return corev1.ServicePort{}, errors.New("k8s: port is required")
	}

	var byName []corev1.ServicePort
	for _, port := range svc.Spec.Ports {
		if strings.TrimSpace(port.Name) == cfg.Port.Name {
			byName = append(byName, port)
		}
	}
	if len(byName) > 0 {
		return pickServicePort(byName, svc, cfg.Port.Name)
	}

	var byTargetPort []corev1.ServicePort
	for _, port := range svc.Spec.Ports {
		if port.TargetPort.Type == intstr.String &&
			strings.TrimSpace(port.TargetPort.StrVal) == cfg.Port.Name {
			byTargetPort = append(byTargetPort, port)
		}
	}
	return pickServicePort(byTargetPort, svc, cfg.Port.Name)
}

func pickServicePort(
	ports []corev1.ServicePort,
	svc *corev1.Service,
	ref string,
) (corev1.ServicePort, error) {
	switch len(ports) {
	case 0:
		return corev1.ServicePort{}, fmt.Errorf(
			"k8s: service %s/%s does not expose port %q",
			svc.Namespace,
			svc.Name,
			ref,
		)
	case 1:
		return ports[0], nil
	default:
		return corev1.ServicePort{}, fmt.Errorf(
			"k8s: service %s/%s has multiple ports matching %q",
			svc.Namespace,
			svc.Name,
			ref,
		)
	}
}

func serviceTargetPort(sp corev1.ServicePort, pod *corev1.Pod, containerName string) (int, error) {
	switch sp.TargetPort.Type {
	case intstr.Int:
		if sp.TargetPort.IntVal > 0 {
			return int(sp.TargetPort.IntVal), nil
		}
	case intstr.String:
		if v := strings.TrimSpace(sp.TargetPort.StrVal); v != "" {
			return podPortByName(pod, containerName, v)
		}
	}
	if sp.Port <= 0 {
		return 0, fmt.Errorf("k8s: service port %q is invalid", sp.Name)
	}
	return int(sp.Port), nil
}

func podPortByName(pod *corev1.Pod, containerName, portName string) (int, error) {
	if pod == nil {
		return 0, errors.New("k8s: pod is required")
	}
	if portName == "" {
		return 0, errors.New("k8s: port is required")
	}

	containers, err := pickContainers(pod, containerName)
	if err != nil {
		return 0, err
	}

	type match struct {
		container string
		port      int32
	}

	var matches []match
	for _, container := range containers {
		for _, port := range container.Ports {
			if strings.TrimSpace(port.Name) != portName {
				continue
			}
			if port.ContainerPort <= 0 || port.ContainerPort > 65535 {
				continue
			}
			matches = append(matches, match{container: container.Name, port: port.ContainerPort})
		}
	}
	if len(matches) == 0 {
		return 0, fmt.Errorf(
			"k8s: pod %s/%s does not expose named port %q",
			pod.Namespace,
			pod.Name,
			portName,
		)
	}
	if containerName == "" && len(matches) > 1 {
		return 0, fmt.Errorf(
			"k8s: pod %s/%s has ambiguous named port %q across containers",
			pod.Namespace,
			pod.Name,
			portName,
		)
	}
	return int(matches[0].port), nil
}

func pickContainers(pod *corev1.Pod, containerName string) ([]corev1.Container, error) {
	if pod == nil {
		return nil, errors.New("k8s: pod is required")
	}
	if containerName == "" {
		return pod.Spec.Containers, nil
	}

	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			return []corev1.Container{container}, nil
		}
	}
	return nil, fmt.Errorf(
		"k8s: pod %s/%s does not contain container %q",
		pod.Namespace,
		pod.Name,
		containerName,
	)
}
