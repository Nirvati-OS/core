// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package runtime

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	configconfig "github.com/siderolabs/talos/pkg/machinery/config/config"
	"github.com/siderolabs/talos/pkg/maintenanceweb/manifests"
)

type fakeInlineManifest struct {
	name string
}

func (f fakeInlineManifest) Name() string                   { return f.name }
func (f fakeInlineManifest) Contents() string               { return "" }
func (f fakeInlineManifest) K8sInlineManifestConfigSignal() {}

func TestHasAutoBootstrapMarker(t *testing.T) {
	tests := []struct {
		name      string
		manifests []configconfig.K8sInlineManifestConfig
		want      bool
	}{
		{
			name:      "no manifests",
			manifests: nil,
			want:      false,
		},
		{
			name: "unrelated manifests only",
			manifests: []configconfig.K8sInlineManifestConfig{
				fakeInlineManifest{name: "cilium"},
				fakeInlineManifest{name: "nirvati-init"},
			},
			want: false,
		},
		{
			name: "marker present among others",
			manifests: []configconfig.K8sInlineManifestConfig{
				fakeInlineManifest{name: "cilium"},
				fakeInlineManifest{name: manifests.AutoBootstrapMarkerName},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasAutoBootstrapMarker(tt.manifests))
		})
	}
}

func TestMatchesNodeAddress(t *testing.T) {
	selfIP := netip.MustParseAddr("10.0.0.5")
	otherIP := netip.MustParseAddr("10.0.0.9")
	// IsULA checks byte[0]==0xfd and byte[7]==purpose, so the SideroLink purpose byte (0x03)
	// must land in the low byte of the 4th 16-bit group - "fd00:0:0:3::" - not right after the
	// "fd" prefix as a naive reading of "SideroLink ULA" might suggest.
	sideroLinkIP := netip.MustParseAddr("fd00:0:0:3::")

	tests := []struct {
		name       string
		endpointIP netip.Addr
		ips        []netip.Addr
		want       bool
	}{
		{
			name:       "endpoint matches one of the node's addresses",
			endpointIP: selfIP,
			ips:        []netip.Addr{otherIP, selfIP},
			want:       true,
		},
		{
			name:       "endpoint matches no address - foreign/load-balanced endpoint",
			endpointIP: netip.MustParseAddr("192.168.1.1"),
			ips:        []netip.Addr{selfIP, otherIP},
			want:       false,
		},
		{
			name:       "endpoint only matches via a SideroLink address - must not count",
			endpointIP: sideroLinkIP,
			ips:        []netip.Addr{sideroLinkIP, otherIP},
			want:       false,
		},
		{
			name:       "no node addresses discovered yet",
			endpointIP: selfIP,
			ips:        nil,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesNodeAddress(tt.endpointIP, tt.ips))
		})
	}
}

func TestEtcdDataDirEmpty(t *testing.T) {
	// constants.EtcdDataPath is a compile-time constant, so this only exercises the
	// "does not exist" branch in this sandboxed test environment (no real etcd data dir) -
	// which is itself a meaningful case: a node that has never run etcd must be treated as
	// empty/safe to bootstrap, not as an error.
	empty, err := etcdDataDirEmpty()
	assert.NoError(t, err)
	assert.True(t, empty)
}
