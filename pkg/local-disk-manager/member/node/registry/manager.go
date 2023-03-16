package registry

import "github.com/hwameistor/hwameistor/pkg/local-disk-manager/member/types"

type Manager interface {
	// DiscoveryResources discovery disks and volumes
	DiscoveryResources()

	// ListDisks list all registered disks
	ListDisks() []types.Disk

	ListDisksByType(devType types.DevType) []types.Disk

	GetDiskByPath(devPath string) types.Disk

	// ListVolumes list all registered volumes
	ListVolumes() []types.Volume

	// ListVolumesByType list all registered volumes
	ListVolumesByType(devType types.DevType) []types.Volume

	GetVolumeByName() types.Volume
}
