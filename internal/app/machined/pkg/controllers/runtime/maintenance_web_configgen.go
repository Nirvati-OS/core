// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package runtime

import (
	"fmt"

	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/maintenanceweb/manifests"
)

// GenerateSingleNodeConfig generates a complete Talos control-plane machine config for a new,
// self-bootstrapping single-node cluster: Cilium as CNI, kube-proxy disabled, Traefik as the
// ingress controller, the Nirvati init Job applied automatically, and a marker manifest that
// lets AutoBootstrapController recognize this config as safe to automatically bootstrap after
// install+reboot.
//
// Config is generated pinned to the pre-multidoc-Kubernetes-config version contract so that the
// CNI/kube-proxy/inline-manifest settings below - all applied via the legacy v1alpha1.Config
// fields - are the sole source of truth, with no competing multidoc Flannel/kube-proxy document
// generated alongside them (see the package-level comment in maintenance_web_configgen_test.go
// for how this was verified).
func GenerateSingleNodeConfig(clusterName, installDisk, endpointIP string) (config.Provider, error) {
	endpoint := fmt.Sprintf("https://%s:%d", endpointIP, constants.DefaultControlPlanePort)

	input, err := generate.NewInput(clusterName, endpoint, constants.DefaultKubernetesVersion,
		generate.WithInstallDisk(installDisk),
		generate.WithVersionContract(config.TalosVersion1_13),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare config generation input: %w", err)
	}

	provider, err := input.Config(machine.TypeControlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to generate control plane config: %w", err)
	}

	cfg := provider.RawV1Alpha1()

	cfg.ClusterConfig.ClusterNetwork.CNI = &v1alpha1.CNIConfig{
		CNIName: constants.CustomCNI,
	}
	cfg.ClusterConfig.ProxyConfig = &v1alpha1.ProxyConfig{
		Disabled: pointer(true),
	}
	cfg.ClusterConfig.ClusterInlineManifests = v1alpha1.ClusterInlineManifests{
		{
			InlineManifestName:     "cilium",
			InlineManifestContents: manifests.CiliumYAML,
		},
		{
			InlineManifestName:     "traefik",
			InlineManifestContents: manifests.TraefikYAML,
		},
		{
			InlineManifestName:     "nirvati-init",
			InlineManifestContents: manifests.NirvatiInitYAML,
		},
		{
			InlineManifestName:     manifests.AutoBootstrapMarkerName,
			InlineManifestContents: manifests.AutoBootstrapMarkerYAML,
		},
	}

	return provider, nil
}

func pointer[T any](v T) *T {
	return &v
}
