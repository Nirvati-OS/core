// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package runtime_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	machinedruntime "github.com/siderolabs/talos/internal/app/machined/pkg/runtime"
	runtimecontrollers "github.com/siderolabs/talos/internal/app/machined/pkg/controllers/runtime"
	"github.com/siderolabs/talos/pkg/machinery/config/validation"
	"github.com/siderolabs/talos/pkg/maintenanceweb/manifests"
)

func TestGenerateSingleNodeConfig(t *testing.T) {
	cfg, err := runtimecontrollers.GenerateSingleNodeConfig("nirvati", "/dev/sda", "10.0.0.5")
	require.NoError(t, err)

	// The generated config must not carry a competing multidoc Flannel CNI document alongside
	// our custom/inline Cilium manifest - see the comment on GenerateSingleNodeConfig.
	assert.Nil(t, cfg.K8sFlannelCNIConfig(), "Flannel must be disabled when using a custom CNI")

	inlineManifests := cfg.K8sInlineManifestConfigs()
	names := make(map[string]string, len(inlineManifests))

	for _, m := range inlineManifests {
		names[m.Name()] = m.Contents()
	}

	assert.Len(t, inlineManifests, 4)
	assert.Equal(t, manifests.CiliumYAML, names["cilium"])
	assert.Equal(t, manifests.TraefikYAML, names["traefik"])
	assert.Equal(t, manifests.NirvatiInitYAML, names["nirvati-init"])
	assert.Equal(t, manifests.AutoBootstrapMarkerYAML, names[manifests.AutoBootstrapMarkerName])

	warnings, err := cfg.ValidateAsClient(machinedruntime.ModeMetal, validation.WithLocal())
	require.NoError(t, err, "warnings: %v", warnings)
}
