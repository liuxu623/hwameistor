package registry

import (
	"fmt"
	"github.com/hwameistor/hwameistor/pkg/local-disk-manager/member/types"
	"github.com/hwameistor/hwameistor/pkg/local-disk-manager/utils/sys"
	log "github.com/sirupsen/logrus"
	"k8s.io/kubernetes/pkg/volume/util/hostutil"
	"os"
	"path"
	"path/filepath"
	"sync"
)

type localRegistry struct {
	// disks storage node disks managed by LocalDiskManager
	// index by disk name
	disks sync.Map

	// disks storage node disks managed by LocalDiskManager
	// index by poolClass
	poolDisks sync.Map

	// volumes storage node volumes managed by LocalDiskManager
	// index by volume name
	volumes sync.Map

	// volumes storage node volumes managed by LocalDiskManager
	// index by poolClass
	poolVolumes sync.Map

	// hu helps to find and create file on host
	hu hostutil.HostUtils
}

func New() Manager {
	return &localRegistry{
		hu: hostutil.NewHostUtil(),
	}
}

// DiscoveryResources discovery disks and volumes
func (r *localRegistry) DiscoveryResources() {
	r.discoveryDisks()
	r.discoveryVolumes()
}

// ListDisks list all registered disks from cache
func (r *localRegistry) ListDisks() []types.Disk {
	var disks []types.Disk
	r.disks.Range(func(key, value any) bool {
		v, ok := value.(types.Disk)
		if ok {
			disks = append(disks, v)
		}
		return true
	})
	return disks
}

// ListDisksByType list disks from cache
func (r *localRegistry) ListDisksByType(devType types.DevType) []types.Disk {
	var disks []types.Disk
	v, ok := r.poolDisks.Load(devType)
	if !ok {
		return nil
	}
	disks, ok = v.([]types.Disk)
	if !ok {
		return nil
	}
	return disks
}

// GetDiskByPath get disk from cache
func (r *localRegistry) GetDiskByPath(devPath string) *types.Disk {
	v, ok := r.disks.Load(devPath)
	if !ok {
		return nil
	}
	disk, ok := v.(types.Disk)
	if !ok {
		return nil
	}
	return &disk
}

// ListVolumes list all registered volumes from cache
func (r *localRegistry) ListVolumes() []types.Volume {
	var volumes []types.Volume
	r.volumes.Range(func(key, value any) bool {
		v, ok := value.(types.Volume)
		if ok {
			volumes = append(volumes, v)
		}
		return true
	})
	return volumes
}

// ListVolumesByType list all registered volumes from cache
func (r *localRegistry) ListVolumesByType(devType types.DevType) []types.Volume {
	var volumes []types.Volume
	v, ok := r.poolVolumes.Load(devType)
	if !ok {
		return nil
	}
	volumes, ok = v.([]types.Volume)
	if !ok {
		return nil
	}
	return volumes
}

func (r *localRegistry) GetVolumeByName(name string) *types.Volume {
	v, ok := r.volumes.Load(name)
	if !ok {
		return nil
	}
	volume, ok := v.(types.Volume)
	if !ok {
		return nil
	}
	return &volume
}

func (r *localRegistry) DiskExist(devPath string) bool {
	_, ok := r.disks.Load(devPath)
	return ok
}

func (r *localRegistry) discoveryDisks() {
	for _, poolClass := range types.DefaultDevTypes {
		rootPath := types.GetPoolDiskPath(poolClass)
		diskNames, err := discoveryDevices(rootPath)
		if err != nil {
			log.WithError(err).Errorf("Failed to discovery devices from %s", rootPath)
			os.Exit(1)
		}

		var discoverDisks []types.Disk
		for _, disk := range diskNames {
			if discoveryDisk, err := convertDisk(disk, poolClass); err != nil {
				log.WithError(err).Errorf("Failed to convert disk %s", disk)
				os.Exit(1)
			} else {
				r.disks.Store(discoveryDisk.DevPath, discoveryDisk)
				discoverDisks = append(discoverDisks, discoveryDisk)
				log.WithFields(log.Fields{"pool": poolClass, "disks": disk}).Info("Succeed discovery disk")
			}
		}
		// store discovery discoverDisks
		r.poolDisks.Store(poolClass, discoverDisks)
		log.WithFields(log.Fields{"pool": poolClass, "disks": len(discoverDisks)}).Info("Succeed discovery pool disks")
	}

	log.Debug("Finish discovery disks")
}

func (r *localRegistry) discoveryVolumes() {
	for _, poolClass := range types.DefaultDevTypes {
		rootPath := types.GetPoolVolumePath(poolClass)
		volumes, err := discoveryDevices(rootPath)
		if err != nil {
			log.WithError(err).Errorf("Failed to discovery devices from %s", rootPath)
			os.Exit(1)
		}

		var discoverVolumes []types.Volume
		for _, volume := range volumes {
			if discoveryVolume, err := convertVolume(volume, poolClass); err != nil {
				log.WithError(err).Errorf("Failed to convert volume %s", volume)
				os.Exit(1)
			} else {
				r.volumes.Store(discoveryVolume.Name, discoveryVolume)
				discoverVolumes = append(discoverVolumes, discoveryVolume)
				log.WithFields(log.Fields{"pool": poolClass, "volume": volume}).Info("Succeed discovery volume")
			}
		}
		// store discovery discoverVolumes
		r.poolVolumes.Store(poolClass, discoverVolumes)
		log.WithFields(log.Fields{"pool": poolClass, "volumes": len(discoverVolumes)}).Info("Succeed discovery pool volumes")
	}

	log.Debug("Finish discovery volumes")
}

var hu hostutil.HostUtils = hostutil.NewHostUtil()

func discoveryDevices(rootPath string) ([]string, error) {
	ok, err := hu.PathExists(rootPath)
	if err != nil || !ok {
		return nil, err
	}

	// walk the folder and discovery devices
	var discoveryDevices []string
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		actualPath, err := hu.EvalHostSymlinks(path)
		if err != nil {
			return err
		}
		ok, err := hu.PathIsDevice(actualPath)
		if err != nil {
			return err
		}
		if ok {
			log.Infof("Found disk %s exist in %s", info.Name(), rootPath)
			discoveryDevices = append(discoveryDevices, info.Name())
		} else {
			log.Debugf("Found %s(mode: %s) in %s but not a device, skip it", info.Name(), info.Mode().Type().String(), rootPath)
		}
		return nil
	})
	if err != nil {
		log.WithError(err).Error("Failed to discovery disks")
	}
	return discoveryDevices, err
}

func convertDisk(devName string, devType types.DevType) (types.Disk, error) {
	device, err := sys.NewSysFsDeviceFromDevPath(path.Join(types.SysDeviceRoot, devName))
	if err != nil {
		return types.Disk{}, err
	}
	capacity, err := device.GetCapacityInBytes()
	if err != nil {
		return types.Disk{}, err
	}

	return types.Disk{
		Name:     devName,
		DevPath:  path.Join(types.SysDeviceRoot, devName),
		Capacity: capacity,
		DiskType: devType,
	}, nil
}

func convertVolume(volumeName, devType types.DevType) (types.Volume, error) {
	actualPath, err := hu.EvalHostSymlinks(path.Join(types.GetPoolVolumePath(devType), volumeName))
	if err != nil {
		return types.Volume{}, err
	}
	ok, err := hu.PathIsDevice(actualPath)
	if err != nil {
		return types.Volume{}, err
	}
	if !ok {
		return types.Volume{}, fmt.Errorf("volume %s not a device", volumeName)
	}
	device, err := sys.NewSysFsDeviceFromDevPath(actualPath)
	if err != nil {
		return types.Volume{}, err
	}
	capacity, err := device.GetCapacityInBytes()
	if err != nil {
		return types.Volume{}, err
	}

	return types.Volume{
		Name:       volumeName,
		Capacity:   capacity,
		AttachPath: actualPath,
	}, nil
}
