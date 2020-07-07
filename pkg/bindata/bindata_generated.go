// Code generated for package bindata by go-bindata DO NOT EDIT. (@generated)
// sources:
// resources/hub/managedcluster/manifests/managedcluster-clusterrole.yaml
// resources/hub/managedcluster/manifests/managedcluster-clusterrolebinding.yaml
// resources/hub/managedcluster/manifests/managedcluster-service-account.yaml
// resources/klusterlet/bootstrap_secret.yaml
// resources/klusterlet/cluster_role.yaml
// resources/klusterlet/cluster_role_binding.yaml
// resources/klusterlet/crds/0000_00_operator.open-cluster-management.io_klusterlets.crd.yaml
// resources/klusterlet/image_pull_secret.yaml
// resources/klusterlet/klusterlet.yaml
// resources/klusterlet/klusterlet_admin_aggregate_clusterrole.yaml
// resources/klusterlet/namespace.yaml
// resources/klusterlet/operator.yaml
// resources/klusterlet/service_account.yaml
package bindata

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func bindataRead(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, gz)
	clErr := gz.Close()

	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}
	if clErr != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

// Name return file name
func (fi bindataFileInfo) Name() string {
	return fi.name
}

// Size return file size
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}

// Mode return file mode
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}

// Mode return file modify time
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}

// IsDir return file whether a directory
func (fi bindataFileInfo) IsDir() bool {
	return fi.mode&os.ModeDir != 0
}

// Sys return file is sys mode
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _hubManagedclusterManifestsManagedclusterClusterroleYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x74\x90\x31\x4f\xc3\x40\x0c\x85\xf7\xfb\x15\x56\x58\x69\x10\x1b\xba\x0d\x31\x30\xc1\xc0\xc0\x82\x3a\xb8\xa9\x49\xad\xde\x9d\x83\xed\x6b\x05\x55\xff\x3b\x6a\x93\x81\xaa\x64\x7a\x27\xbd\x4f\xe7\xf7\x1e\x0e\xfc\x4e\x6a\x2c\x25\x82\xae\xb0\x6b\xb1\xfa\x46\x94\x7f\xd0\x59\x4a\xbb\x7d\xb0\x96\xe5\x6e\x77\x1f\xb6\x5c\xd6\x11\x9e\x52\x35\x27\x7d\x93\x44\x21\x93\xe3\x1a\x1d\x63\x00\x28\x98\x29\x82\x7d\x9b\x53\x8e\x32\x50\x59\x74\x23\xb9\xc8\x58\xb0\xa7\x4c\xc5\xe3\xf8\x5c\x4f\x4e\x5c\x89\xb8\xb9\xe2\x10\x0f\x07\x68\x5f\x46\x73\x3a\xf0\x8a\x99\xe0\x78\x0c\x5a\x13\x59\x0c\x37\xf0\x98\x92\xec\x61\xfa\x01\xb0\xa7\xe2\xe0\x02\x2a\x8e\x4e\xc0\x6e\xd0\x91\x3a\x7f\x72\x87\x4e\x61\x01\x38\xf0\xb3\x4a\x1d\x2c\xc2\x47\xf3\xc7\xb2\xa9\x52\xb3\x0c\x00\x4a\x26\x55\x3b\xba\x82\xb8\x2f\x5c\x7a\xa5\xaf\x4a\xe6\x76\x66\x77\xa4\xab\x91\x53\x42\xa7\xe6\x16\x9a\x9e\xfc\x24\x89\xed\xac\x7b\xf4\x6e\xd3\x2c\xe7\xc3\xf6\xe4\x57\xc9\xc6\xba\xed\xcc\x64\xff\x06\xbd\x9c\xd1\x2e\x80\xd3\x6e\x67\x68\x76\xd2\xcb\x32\xa7\x0a\xcb\xf0\x1b\x00\x00\xff\xff\x88\xb9\xb0\x2d\x05\x02\x00\x00")

func hubManagedclusterManifestsManagedclusterClusterroleYamlBytes() ([]byte, error) {
	return bindataRead(
		_hubManagedclusterManifestsManagedclusterClusterroleYaml,
		"hub/managedcluster/manifests/managedcluster-clusterrole.yaml",
	)
}

func hubManagedclusterManifestsManagedclusterClusterroleYaml() (*asset, error) {
	bytes, err := hubManagedclusterManifestsManagedclusterClusterroleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "hub/managedcluster/manifests/managedcluster-clusterrole.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _hubManagedclusterManifestsManagedclusterClusterrolebindingYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xb4\x8e\xb1\x4e\xc3\x40\x0c\x86\xf7\x7b\x8a\x7b\x81\x04\xb1\x21\x6f\x94\x81\x09\x86\x22\xb1\x3b\x17\x53\x4c\x73\xf6\xe9\xec\xab\x04\x55\xdf\x1d\x85\xa4\x48\xa8\x12\x5b\x37\x4b\xff\xe7\xff\xfb\xb1\xf0\x2b\x55\x63\x15\x88\x75\xc0\xd4\x63\xf3\x77\xad\xfc\x85\xce\x2a\xfd\xfe\xce\x7a\xd6\x9b\xc3\x6d\xd8\xb3\x8c\x10\x1f\xa6\x66\x4e\x75\xab\x13\x6d\x58\x46\x96\x5d\xc8\xe4\x38\xa2\x23\x84\x18\x05\x33\x41\xb4\x4f\x73\xca\xa0\x85\xa4\x4b\xcb\x43\x97\x51\x70\x47\x99\xc4\x61\x39\xc7\x35\x81\x41\xd5\xcd\x2b\x16\x38\x1e\x63\xff\xb4\x84\xab\xe7\x19\x33\xc5\xd3\x29\x54\x9d\x68\x4b\x6f\xb3\x02\x0b\x3f\x56\x6d\xe5\x9f\xb9\x21\xc6\x8b\xb5\x57\x1c\x67\x6d\xf8\xa0\xe4\x06\xa1\x5b\xbd\x2f\x54\x0f\x9c\xe8\x3e\x25\x6d\xe2\xbf\xea\xb9\x62\x73\x2e\xfc\xcb\x9c\xbb\x16\xd4\x0a\xa6\x95\xbf\x54\xfe\x84\x33\xfb\x1d\x00\x00\xff\xff\x8e\x12\x91\x7d\xbb\x01\x00\x00")

func hubManagedclusterManifestsManagedclusterClusterrolebindingYamlBytes() ([]byte, error) {
	return bindataRead(
		_hubManagedclusterManifestsManagedclusterClusterrolebindingYaml,
		"hub/managedcluster/manifests/managedcluster-clusterrolebinding.yaml",
	)
}

func hubManagedclusterManifestsManagedclusterClusterrolebindingYaml() (*asset, error) {
	bytes, err := hubManagedclusterManifestsManagedclusterClusterrolebindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "hub/managedcluster/manifests/managedcluster-clusterrolebinding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _hubManagedclusterManifestsManagedclusterServiceAccountYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x54\xcb\xb1\x0a\xc2\x30\x14\x85\xe1\x3d\x4f\x71\xe9\x03\x08\xae\xd9\xd4\xd9\x2e\x82\xfb\x21\x39\x48\xd0\xdc\x84\xe4\xb6\x4b\xe9\xbb\x0b\x62\x86\xce\xff\xf7\xa3\xa6\x27\x5b\x4f\x45\xbd\xac\x67\xf7\x4e\x1a\xbd\x3c\xd8\xd6\x14\x78\x09\xa1\x2c\x6a\x2e\xd3\x10\x61\xf0\x4e\x44\x91\xe9\x65\xda\x36\x39\x5d\x4b\xb1\x6e\x0d\xf5\xc8\x67\x64\xca\xbe\x4f\x7f\xdc\x2b\xc2\x38\xee\x50\xbc\x18\x6f\x9f\xa5\x1b\xdb\x3c\xea\x4f\x7f\x03\x00\x00\xff\xff\xfa\x75\x6d\xe8\x89\x00\x00\x00")

func hubManagedclusterManifestsManagedclusterServiceAccountYamlBytes() ([]byte, error) {
	return bindataRead(
		_hubManagedclusterManifestsManagedclusterServiceAccountYaml,
		"hub/managedcluster/manifests/managedcluster-service-account.yaml",
	)
}

func hubManagedclusterManifestsManagedclusterServiceAccountYaml() (*asset, error) {
	bytes, err := hubManagedclusterManifestsManagedclusterServiceAccountYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "hub/managedcluster/manifests/managedcluster-service-account.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletBootstrap_secretYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x44\x8d\xb1\x0a\xc2\x30\x14\x45\xf7\x7c\xc5\x23\x7b\x05\xd7\x8c\xae\x05\x1d\x04\xf7\x97\xf4\xaa\xa1\x6d\x12\x93\x17\x41\x4a\xff\x5d\x94\x46\xe7\x7b\xce\xb9\x9c\xfc\x05\xb9\xf8\x18\x0c\x3d\xf7\x6a\xf4\x61\x30\x74\x86\xcb\x10\x35\x43\x78\x60\x61\xa3\x88\x02\xcf\x30\xa4\x6d\x8c\x52\x24\x73\xea\xee\xd5\x76\x63\xb5\x70\x31\x5c\xfd\x4d\x6f\x48\x49\xec\x3e\xdc\xb2\xd0\xae\x9f\x6a\x11\xe4\x09\x72\x6c\x0b\xad\xab\x56\xf2\x4a\x30\x74\x4a\xfc\xa8\x50\xad\xff\x4f\x6d\xf6\xa1\x3d\xf5\xbf\xe5\x6b\xbf\x03\x00\x00\xff\xff\x3c\x92\x16\x61\xb1\x00\x00\x00")

func klusterletBootstrap_secretYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletBootstrap_secretYaml,
		"klusterlet/bootstrap_secret.yaml",
	)
}

func klusterletBootstrap_secretYaml() (*asset, error) {
	bytes, err := klusterletBootstrap_secretYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/bootstrap_secret.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletCluster_roleYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xcc\x53\x3d\x8f\xdb\x30\x0c\xdd\xfd\x2b\x04\x77\xbd\xf8\xd0\xad\xf0\x56\x74\xe8\xde\xa1\x4b\x91\x81\x91\x5f\x73\x6a\x64\x51\x20\x29\x07\xed\xaf\x2f\xfc\xd1\x5c\xce\x4e\xd0\x02\xbd\xe1\x26\x7d\x50\x7c\x7c\xef\x89\xa4\x1c\xbe\x42\x34\x70\x6a\x9d\x1c\xc8\x37\x54\xec\x89\x25\xfc\x22\x0b\x9c\x9a\xd3\x07\x6d\x02\x3f\x0e\xef\xab\x53\x48\x5d\xeb\x3e\xc5\xa2\x06\xf9\xc2\x11\x55\x0f\xa3\x8e\x8c\xda\xca\xb9\x44\x3d\x5a\x77\x9a\xa3\x11\x56\x49\x89\xd0\xb6\x7a\xe7\x3e\xc6\xc8\xe7\xab\x88\x33\x76\x5e\x40\x06\x77\x66\x39\x45\xa6\xae\xda\x39\xca\xe1\xb3\x70\xc9\xda\xba\x6f\x75\xbd\xaf\x9c\x13\x28\x17\xf1\x98\x6e\x14\x5e\x60\x5a\x3f\xb8\xda\x73\xfa\x1e\x8e\x3d\xe5\xe9\xa4\x90\x21\x78\x90\xf7\x5c\x92\xe9\x94\x39\x40\x0e\x53\xd6\x5c\x66\x7c\x76\x84\x8d\x4b\x0c\x3a\xad\x25\x77\x4b\xe0\x4c\xe6\x9f\xc6\x4d\xfe\xb3\xe9\x10\x61\xa8\xf7\x6b\x52\xb7\x7c\xb9\x41\xb4\x1c\x7e\xc0\x1b\x79\x0f\x55\xc1\x10\x70\xbe\x4d\x6a\x83\xbf\xc5\x1a\x3d\xd5\x4c\x1e\x2b\x84\x95\x98\x45\xc2\x7d\xe0\x07\x57\x63\x40\x32\xbd\xcb\x7a\x0e\xbf\xa8\xf2\xec\xdd\xc5\x99\xc5\xb5\xad\x33\x39\xeb\x16\xb3\x43\x8e\xfc\xb3\xdf\x00\xbf\xee\xaf\xdc\x6d\xd9\x2d\x21\x3f\xb7\xa0\x70\xc4\x21\xa4\x2e\xa4\xe3\xd4\x41\x2f\xce\x6f\x8d\xe8\x85\xe1\x2b\x52\x1b\xdb\x41\x3d\xc5\xe5\xdd\xa8\xbd\xde\x5f\x06\x35\x15\x1f\x0b\x74\x9c\xd2\x9e\x12\x1d\x71\x3d\xba\x94\x83\x36\x6b\x65\x9c\x21\x64\x2c\x0d\x67\xa4\xdd\x42\x7e\x37\x27\x8f\xdf\x7f\x53\xe3\x33\xe8\x3f\x35\xf7\xb5\xb0\xbf\x5a\xfd\xbf\x84\x1e\xd5\xc8\xca\x8a\xd7\xba\xfe\xbe\xfa\x1d\x00\x00\xff\xff\xa1\x25\x7d\xa3\x3a\x05\x00\x00")

func klusterletCluster_roleYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletCluster_roleYaml,
		"klusterlet/cluster_role.yaml",
	)
}

func klusterletCluster_roleYaml() (*asset, error) {
	bytes, err := klusterletCluster_roleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/cluster_role.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletCluster_role_bindingYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x7c\x8d\xb1\x6e\xc3\x30\x0c\x05\x77\x7d\x05\xe1\xbd\x2a\xba\x15\xda\xda\x0e\x1d\x0a\x74\x70\x81\xee\xb4\xcc\x24\x8c\x65\x51\x90\x28\x0f\x31\xfc\xef\x41\xe0\x24\x4b\x8c\xac\x3c\xde\x3b\x4c\xfc\x4f\xb9\xb0\x44\x07\xb9\x43\x6f\xb1\xea\x41\x32\x9f\x50\x59\xa2\x1d\xde\x8b\x65\x79\x9d\xde\xcc\xc0\xb1\x77\xf0\x15\x6a\x51\xca\xad\x04\xfa\xe4\xd8\x73\xdc\x9b\x91\x14\x7b\x54\x74\x06\x20\xe2\x48\x0e\x86\xf5\x29\x90\x9a\x2c\x81\x5a\xda\x5d\x18\x26\xfe\xce\x52\xd3\x93\x8e\x01\x78\xc8\x6c\xad\x96\xda\x1d\xc9\x6b\x71\xe6\xe5\x2a\xfc\x51\x9e\xd8\xd3\x87\xf7\x52\xa3\x6e\x39\xeb\xa9\x24\xf4\xe4\xa0\x99\x67\xb0\x3f\x77\xf8\x7b\x23\xb0\x2c\x8d\x39\x07\x00\x00\xff\xff\xdc\x27\xb7\x62\x13\x01\x00\x00")

func klusterletCluster_role_bindingYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletCluster_role_bindingYaml,
		"klusterlet/cluster_role_binding.yaml",
	)
}

func klusterletCluster_role_bindingYaml() (*asset, error) {
	bytes, err := klusterletCluster_role_bindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/cluster_role_binding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletCrds0000_00_operatorOpenClusterManagementIo_klusterletsCrdYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xb4\x58\xcd\x6e\xe4\xb8\x11\xbe\xf7\x53\x14\x9c\x83\x13\xc0\x2d\x63\x90\x4b\xd0\x37\xc7\xbb\x01\x8c\x9d\x9d\x0c\x6c\xef\xec\x61\xb1\x87\x92\x58\xdd\x62\x4c\x91\x0a\x59\xec\xd9\x4e\x90\x77\x0f\x8a\x94\xd4\xfa\x69\x79\x3c\x99\xac\x4e\xdd\x64\xb1\xf8\xd5\xc7\xfa\x23\xb1\xd5\x9f\xc8\x07\xed\xec\x0e\xb0\xd5\xf4\x1b\x93\x95\x7f\xa1\x78\xf9\x4b\x28\xb4\xbb\x3d\xbe\x2b\x89\xf1\xdd\xe6\x45\x5b\xb5\x83\xfb\x18\xd8\x35\x8f\x14\x5c\xf4\x15\x7d\x47\x7b\x6d\x35\x6b\x67\x37\x0d\x31\x2a\x64\xdc\x6d\x00\x2a\x4f\x28\x83\xcf\xba\xa1\xc0\xd8\xb4\x3b\xb0\xd1\x98\x0d\x80\xc5\x86\x76\xf0\x62\x62\x60\xf2\x86\x38\x14\xae\x25\x8f\xec\xbc\xfc\xb0\xdb\x2a\xcf\x6c\x1b\xb4\x78\xa0\x86\x2c\x17\xda\x6d\x42\x4b\x95\xe8\x3d\x78\x17\xdb\x1d\xbc\x65\x49\xde\x2a\xc8\x2a\x80\x0c\xfd\x87\x61\xd7\x34\x68\x74\xe0\x1f\x66\x13\xef\x75\xc8\x93\xad\x89\x1e\xcd\x04\x69\x1a\x0f\xda\x1e\xa2\x41\x3f\x9e\xd9\x00\x84\xca\xb5\xb4\x83\xfb\x3c\x26\x03\xb1\xf4\x1d\x47\x1d\x86\xc0\xc8\x31\xec\xe0\xdf\xff\xd9\x00\x1c\xd1\x68\x95\x28\xca\x93\x62\xc8\xdd\xc7\x87\x4f\x7f\x7e\xaa\x6a\x6a\x30\x0f\x02\x28\x0a\x95\xd7\x6d\x92\x1b\xa1\x04\x4f\xad\xa7\x40\x96\x03\x54\xce\xb2\x77\xc6\x90\x0f\xe0\x2c\x70\x4d\x90\x89\x50\xd0\x11\x53\xc0\xcf\x35\xd9\x4e\x23\xc8\x82\xbd\x3e\x44\x4f\xea\x26\x49\x4f\xd4\xfe\x33\x6a\x4f\x01\x10\x02\x55\x9e\x38\x71\xa8\xc0\xed\xa1\x74\x8e\x03\x7b\x6c\xb7\x75\x2c\xb7\x2f\xb1\xa4\xac\x67\x50\xab\xf3\xde\x01\x1b\xca\xcc\xb7\x58\x11\xb0\x03\x34\xc6\x7d\x86\xbb\x8f\x0f\x49\x3d\x05\x0e\x32\x2a\xb2\x75\x2c\x61\xef\x7c\xfa\xed\xe9\xa0\x45\x7f\x72\xa5\x5e\x67\xeb\x1d\xbb\xca\x99\xa2\x1b\xe1\x93\x90\xec\xca\x7f\x50\xc5\x9b\x41\xa4\x25\xcf\xba\x67\x59\xbe\x91\x47\x0f\x63\x33\x2e\xaf\x85\xec\x2c\x03\x4a\x7c\x98\x42\x82\x71\xcc\x63\xa4\x20\xa4\x83\x10\xd3\xb9\xd6\xe1\xcc\xf8\x14\x61\x3a\xbb\x3d\xa0\xed\x50\x15\xf0\x44\x5e\x94\x40\xa8\x5d\x34\x4a\xd8\x3e\x92\x17\x6a\x2b\x77\xb0\xfa\x5f\x83\xe6\x81\x05\x83\x4c\x81\x27\x1a\xb5\x65\xf2\x16\x8d\xb8\x49\xa4\x1b\x40\xab\xa0\xc1\x13\x78\x92\x3d\x20\xda\x91\xb6\x24\x12\x0a\xf8\xd1\x79\x02\x6d\xf7\x6e\x07\x35\x73\x1b\x76\xb7\xb7\x07\xcd\x7d\x0c\x57\xae\x69\xa2\xd5\x7c\xba\x4d\xfe\xa2\xcb\xc8\xce\x87\x5b\x45\x47\x32\xb7\x41\x1f\xb6\xe8\xab\x5a\x33\x55\x1c\x3d\xdd\x62\xab\xb7\x09\xb8\xe5\x94\x08\x1a\xf5\x87\xc1\x99\xaf\x47\x48\xf3\x79\x04\xf6\xda\x9e\x1d\x21\xc5\xda\x2a\xef\x12\x70\xa0\x93\x87\xa5\x65\x19\xff\x99\x5e\x19\x12\x56\x1e\xbf\x7f\x7a\x86\x7e\xd3\x74\x04\x53\xce\x13\xdb\xa3\x38\x38\x13\x2f\x44\x69\xbb\x27\x9f\x0f\x6e\xef\x5d\x93\x34\x92\x55\xad\xd3\x96\xd3\x9f\xca\x68\xb2\x53\xd2\x43\x2c\x1b\xcd\x61\xec\xa5\x05\xdc\xa3\xb5\x8e\xa1\x24\x88\xad\x42\x26\x55\xc0\x83\x85\x7b\x6c\xc8\xdc\x63\xa0\xdf\x9d\x76\x61\x38\x6c\x85\xd2\x2f\x13\x3f\x4e\xc0\x53\xc1\x49\xc4\x00\xf4\xd9\xf4\xe2\x09\x3d\xb5\x54\x8d\xf3\x8b\xb0\xa5\x28\x68\x4f\x0a\x14\xb5\xc6\x9d\x24\xc3\x0e\x59\x24\x85\x83\x84\xc0\x2c\xb9\x0e\xb1\x78\x90\x7c\xfc\x25\x44\x97\xe3\x58\xbe\x2e\x87\x7d\x90\xb2\x31\x99\x98\xc1\xbe\x3f\xcb\x89\x7b\x09\x6a\xc9\x42\x39\x7e\x17\x29\x51\x62\xaf\xa4\x5c\xa6\x48\xcd\xf4\x82\xe4\xd1\x3a\x96\x05\x3c\x4f\xd3\x63\xb2\x05\x0e\x64\xa5\xfa\xa4\x2c\xe9\xd1\x2a\xd7\xe4\x9d\xf4\x1e\x34\xcb\xde\xd6\xf1\x42\x63\x20\xbe\x01\xe7\x41\xe9\x50\xb9\xe4\xa6\x82\x0a\x5b\x31\xdb\x6b\x64\x1a\x90\x65\xd4\x36\x55\x84\x50\xeb\xfd\x84\xbc\xd5\xb3\x97\x4f\x2a\xb7\x64\x8d\x1c\x08\x3f\x3d\xbe\x0f\xaf\x32\xf6\xfd\x42\x7c\x7e\xec\x98\x4a\x64\xca\x6f\xad\x0e\x49\x0c\xa2\x37\x61\x61\x9d\xe4\xa7\x0a\xa1\x8c\x56\x99\x94\x48\x31\x11\x81\x55\x45\x21\xe8\xd2\xd0\x80\xcd\x9c\xe0\xa1\xe7\x29\x10\x03\x35\x2d\x9f\x6e\xfa\xe3\x59\x28\xee\x49\xa9\x51\x68\x1d\x6b\x19\xe9\x8e\xde\xe4\x2d\xa5\x9e\xf4\x2b\x2a\xb4\x70\xd4\x41\xaf\xd0\x87\xde\xe3\x69\x36\xa3\x99\x9a\x05\x65\xf3\xe8\xe8\xc9\x5a\x70\x35\x66\x68\x4a\xc8\x42\x23\xbc\xce\xd0\x42\x7e\x25\x64\xf2\xb7\x16\x38\x1d\x81\xf8\xd7\x84\xe1\xd2\xdc\x3c\x82\xee\xb2\x68\x1f\x3e\x03\x7e\x09\x96\xca\x59\x2b\x09\x57\xea\x79\x6f\xe9\x45\x95\xb0\x12\x71\x05\x3c\x9d\x02\x53\x03\x15\x79\x0e\x80\x9e\x20\x06\x52\x93\xa8\x11\x8f\x98\x1f\xd7\x98\x81\x0b\x3e\xdf\x7f\x7b\xe7\x1b\xe4\x1d\x94\x27\xbe\xc4\x77\xf4\xe6\x0d\x0c\xc8\xb1\x76\xc6\xcb\x21\x4e\xfc\x7e\xa8\x1e\x53\xf3\x56\x38\xe8\x8d\xfe\x3a\x63\x86\xb6\xe9\xd5\xb8\xfd\x30\x34\x57\xa3\x3c\x37\x74\x5b\x39\x45\x67\x97\x4c\xa9\x37\x25\xb1\x41\x64\x01\xa8\x89\x81\xa1\xc6\xa3\x44\x7b\xeb\x69\xaf\x7f\x13\x0b\xaf\x56\x1a\xeb\xed\x55\x6e\x46\xbe\x9c\xeb\xa6\xc0\x5e\x53\x99\x60\x5e\x89\xb2\xe4\x10\x83\x0d\xcb\x2c\x33\x2f\x25\xaf\x92\x39\x6e\x28\x1f\x1a\x3c\xd0\xc7\x68\xcc\xd3\xac\xf2\x2d\xc8\x7d\x5c\x5b\xb5\x56\x12\xb5\x08\x2d\xf3\xd6\xbc\x3a\x8e\xd1\x7c\xa5\x21\x9f\x9d\x7f\x79\xbb\x01\x3f\xcf\xa5\x5f\x05\x3e\x05\xba\xac\x83\xfb\xb4\xfb\x57\x00\xee\x2e\x39\xab\xcd\x45\x9a\x9e\x43\xaa\xa2\xf7\x52\x59\xf3\xe2\x69\x33\xf1\xed\x0d\x84\xb3\x2a\x5d\x50\x5f\xaf\x86\xd7\xf7\x83\x5c\xba\x52\x61\x77\x9f\x51\x7a\xbf\x27\xdf\x75\x3c\x59\xa0\xc3\x49\x41\xb2\xce\x32\x59\x4b\xd3\x79\xc6\x5f\xc0\x27\xb9\xea\x8d\x56\xa7\x96\x4e\x12\xe0\x0e\xee\xda\xd6\x68\x52\x3b\xa8\x5c\xd3\x3a\x9b\x08\x91\x58\x5c\x28\x2d\x89\xac\x74\x0b\x22\xdd\xdf\xb4\x16\x09\xf6\xee\x88\xda\x60\x69\x68\xa2\x2f\x4b\x2f\xe3\x7e\xd6\x10\x49\x42\xc6\x5e\x41\x8a\x71\x4f\xa8\x4e\x12\x8e\x29\x03\x16\xf0\xd1\xbb\x83\x97\x6a\x65\x0f\xe3\x0d\x16\x9a\x2f\xc3\x4b\x1b\x68\x0b\x08\xec\xd1\x86\x44\x85\xf4\xfa\xc2\x25\x15\xf0\x1d\x1d\x3c\xaa\x29\x15\x6f\xd5\xac\x5c\x2a\x1e\x0d\x72\x55\x4f\x5c\x7c\x1a\x85\x68\x2f\x35\x7a\xe6\x24\x9e\x73\xd4\x4a\x96\x65\x0c\xc9\x60\x5d\x51\x71\xfd\xff\x6d\x1d\x92\xd7\x0c\x6e\xd6\x7b\x59\x18\xb9\x86\xdc\x23\xa4\x8e\x69\x67\x97\xa5\xe3\x1b\xba\x00\x83\x81\x9f\x07\xda\x9f\xf5\xb2\x97\xbe\x80\xf7\xfd\x62\x51\x5f\x70\x44\x1d\xb0\x0c\xa4\xe8\xed\xe1\xaf\xd5\xc2\x1a\xad\x1c\x57\xba\x88\x39\x4b\x7d\x98\x4b\x2b\x61\x1d\xd7\x5f\x5d\x26\xfb\xaf\xaf\xf9\x72\x2f\xdb\x0a\x9c\x0b\x52\x0d\x85\x80\x87\xb7\x98\xfb\x63\x96\xcc\x77\xd3\x3a\x36\x68\xb7\x12\x01\x29\x1c\x9a\x7e\xce\x2a\x5d\x61\xba\xa3\x2a\x62\xd4\x17\x5a\xe1\xfc\x61\xe9\x22\x9f\xb9\xea\x2c\xce\x4c\xfc\x4f\xd6\x7a\xc2\x30\x7d\xce\x58\x31\xe3\x31\x09\x66\x2b\xfe\x58\x7a\x4d\xfb\x3f\x75\x8b\x87\xa7\x96\xe1\xc0\xae\x43\x82\xb7\x62\xc3\xb7\x83\x5e\x16\x83\x15\xd0\x5d\x59\xe8\xdc\xeb\x5c\x06\x26\x68\x0b\xf8\xbb\x4d\x9d\xc4\xb3\x8f\x74\xb3\x02\xfa\x6f\x68\x02\xdd\xc0\x4f\xf6\xc5\xba\xcf\x17\x82\xe8\x0d\xa8\xd3\xf4\x97\x31\x3f\x9f\xda\x21\x20\x64\xc9\x80\xb7\xbf\x80\x0c\xb8\xdf\x02\xe2\xd8\xbf\xc0\x1e\xdf\x9d\xff\x25\xea\xb6\xdd\x93\x69\x9a\x80\x9c\x8c\xd5\x0e\xd8\x47\xea\x9e\x15\x9d\x17\x0f\xcf\x23\x67\xca\xe5\x6a\xd1\x32\xa9\x0f\xf3\x57\xd0\xab\xab\xc9\x03\x67\xfa\x3b\x2a\x92\xf0\xcb\xaf\x9b\xac\x95\xd4\xa7\x1e\x07\xfc\xf2\xeb\x7f\x03\x00\x00\xff\xff\x5b\x7c\x3a\xfa\x27\x16\x00\x00")

func klusterletCrds0000_00_operatorOpenClusterManagementIo_klusterletsCrdYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletCrds0000_00_operatorOpenClusterManagementIo_klusterletsCrdYaml,
		"klusterlet/crds/0000_00_operator.open-cluster-management.io_klusterlets.crd.yaml",
	)
}

func klusterletCrds0000_00_operatorOpenClusterManagementIo_klusterletsCrdYaml() (*asset, error) {
	bytes, err := klusterletCrds0000_00_operatorOpenClusterManagementIo_klusterletsCrdYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/crds/0000_00_operator.open-cluster-management.io_klusterlets.crd.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletImage_pull_secretYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x6c\xce\xbf\x0a\xc2\x30\x10\xc7\xf1\x3d\x4f\x71\x4f\x50\x70\xcd\xec\x22\x82\x08\x8a\xfb\x8f\xe4\x2c\xb1\xf9\x47\x72\x15\x4a\xe9\xbb\x4b\xda\xba\x75\xbe\xcf\xf7\xee\x90\xdd\x8b\x4b\x75\x29\x6a\xfa\x9e\xd4\xe0\xa2\xd5\xf4\x60\x53\x58\x54\x60\x81\x85\x40\x2b\xa2\x88\xc0\x9a\xe6\x99\xba\x4b\x40\xcf\xf7\xd1\xfb\x4d\xdd\x10\x98\x96\x65\x27\x35\xc3\xec\xee\xea\xc7\x2a\x5c\xfc\x46\xd6\x41\x73\x32\xe5\xe3\x45\xcf\x29\xaf\xe0\x7f\x91\xa8\xb3\xc9\x0c\x5c\x4c\x8a\x6f\xd7\x7f\x6a\x7b\xf1\xa0\x3b\x43\xd0\xba\x5f\x00\x00\x00\xff\xff\xb3\xa8\xd8\x2e\xca\x00\x00\x00")

func klusterletImage_pull_secretYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletImage_pull_secretYaml,
		"klusterlet/image_pull_secret.yaml",
	)
}

func klusterletImage_pull_secretYaml() (*asset, error) {
	bytes, err := klusterletImage_pull_secretYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/image_pull_secret.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletKlusterletYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x6c\x8f\x3d\x8a\xc3\x30\x10\x46\x7b\x9d\x62\x2e\x60\x2f\xdb\xaa\xdd\x6a\x59\xd6\x84\x04\x92\x7a\xb0\x07\x23\x2c\x8d\xc4\x68\x9c\x14\xc6\x77\x0f\xfe\x43\x24\xa4\xd4\xf7\x9e\xe0\x0d\x26\x77\x25\xc9\x2e\xb2\x85\x98\x48\x50\xa3\xd4\x31\x11\x57\xad\x1f\xb3\x92\x54\x01\x19\x7b\x0a\xc4\x5a\xbb\xf8\x75\xff\x36\x83\xe3\xce\xc2\xdf\x86\x3d\xa9\x09\xa4\xd8\xa1\xa2\x35\x00\x8c\x81\x2c\x0c\x05\xe6\x44\xed\x02\x84\x7a\x97\x55\x50\x5d\xe4\xdf\x80\x3d\x9d\x46\xef\x2f\x0b\x84\x69\x82\xfa\xfc\x8e\x1b\x0c\x04\xf3\x6c\x00\x1e\x51\x86\x0f\x3f\x6e\xc7\x5c\xcc\x3d\xb9\x59\x1b\x16\xe7\x7f\x6d\xef\x7e\xca\x9e\x13\xb6\xbb\xcd\xc7\x73\x73\xcb\x41\x2f\xde\x33\x00\x00\xff\xff\x40\xbb\xda\x62\x21\x01\x00\x00")

func klusterletKlusterletYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletKlusterletYaml,
		"klusterlet/klusterlet.yaml",
	)
}

func klusterletKlusterletYaml() (*asset, error) {
	bytes, err := klusterletKlusterletYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/klusterlet.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletKlusterlet_admin_aggregate_clusterroleYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x7c\x8e\xbd\x4e\xec\x30\x10\x85\x7b\x3f\xc5\xc8\xf5\x4d\xae\xe8\x90\x5b\x0a\x7a\x0a\x1a\xb4\xc5\x24\x39\xca\x5a\xb1\x3d\xd6\x78\xbc\x48\x3c\x3d\x4a\xc2\x6a\x2b\xa8\xe6\x68\xce\x8f\x3e\xae\xf1\x1d\xda\xa2\x94\x40\x3a\xf1\x3c\x72\xb7\xab\x68\xfc\x62\x8b\x52\xc6\xed\xb9\x8d\x51\xfe\xdf\x9e\xdc\x16\xcb\x12\xe8\x25\xf5\x66\xd0\x37\x49\x70\x19\xc6\x0b\x1b\x07\x47\x54\x38\x23\x90\x54\x94\x61\x3e\x23\x43\xe6\xc2\x2b\x32\x8a\x85\xed\x7c\x25\xd8\xc0\x4b\x8e\x65\xe0\x75\x55\xac\x6c\xb8\xa7\x75\x1f\x24\x4a\x3c\x21\xb5\x7d\x90\xfe\xa0\x79\xb4\x4d\xce\xc1\x40\xde\xb4\xc3\x3b\xed\x09\x2d\xb8\x81\xb8\xc6\x57\x95\x5e\x5b\xa0\x0f\x2f\x15\xca\x26\x3a\xfe\x02\x38\x46\xf1\x17\x47\xa4\x68\xd2\x75\xc6\x51\x7a\x40\xb7\xc3\xbc\x41\xa7\xc3\x58\x61\xfe\x1f\xf9\x14\xdb\x71\x3f\xd9\xe6\xeb\x2e\x66\x05\x1b\x76\xd5\xeb\xf2\xa3\xea\xdd\x5c\x90\x60\xf0\x97\xef\x00\x00\x00\xff\xff\xa9\x93\x0a\x52\x70\x01\x00\x00")

func klusterletKlusterlet_admin_aggregate_clusterroleYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletKlusterlet_admin_aggregate_clusterroleYaml,
		"klusterlet/klusterlet_admin_aggregate_clusterrole.yaml",
	)
}

func klusterletKlusterlet_admin_aggregate_clusterroleYaml() (*asset, error) {
	bytes, err := klusterletKlusterlet_admin_aggregate_clusterroleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/klusterlet_admin_aggregate_clusterrole.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletNamespaceYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x4a\x2c\xc8\x0c\x4b\x2d\x2a\xce\xcc\xcf\xb3\x52\x28\x33\xe4\xca\xce\xcc\x4b\xb1\x52\xf0\x4b\xcc\x4d\x2d\x2e\x48\x4c\x4e\xe5\xca\x4d\x2d\x49\x4c\x49\x2c\x49\xb4\xe2\x52\x50\xc8\x4b\xcc\x4d\xb5\x52\x50\xaa\xae\x56\xd0\xf3\xce\x29\x2d\x2e\x49\x2d\xca\x49\x2d\x81\x2b\x55\xa8\xad\x55\x02\x04\x00\x00\xff\xff\xeb\x15\x94\xaf\x4d\x00\x00\x00")

func klusterletNamespaceYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletNamespaceYaml,
		"klusterlet/namespace.yaml",
	)
}

func klusterletNamespaceYaml() (*asset, error) {
	bytes, err := klusterletNamespaceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/namespace.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletOperatorYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xcc\x91\x41\x6b\x1b\x31\x10\x85\xef\xfb\x2b\x06\xdf\x53\xd7\x6d\x0e\x45\xb7\x42\xa0\x0d\x2d\xe9\xd2\x84\xde\x27\xda\x57\xaf\xc8\xac\x24\x46\x63\x83\x1b\xfc\xdf\x8b\x12\x67\xa3\x25\xfe\x01\xd1\x69\x99\xf7\xde\xc7\x9b\xd9\x87\x10\x07\x47\x57\xc8\x92\x0e\x13\xa2\x75\x9c\xc3\x1f\x68\x09\x29\x3a\xe2\x9c\xcb\x7a\xbf\xe9\x26\x18\x0f\x6c\xec\x3a\xa2\xc8\x13\x1c\x3d\xc8\xae\x18\x54\x60\xa7\x51\xc9\xec\xe1\x68\xf5\xf8\x48\x1f\x7e\xcc\xe2\xcd\x8b\x42\xc7\xe3\xaa\x23\x12\xbe\x87\x94\x8a\xa1\x0a\x5f\x70\x4a\x86\xaf\x8a\x22\x4b\xf0\x5c\x1c\x6d\x3a\xa2\x02\x81\xb7\xa4\xcf\x99\x89\xcd\x8f\x3f\x1b\xc8\x5b\x0c\x91\x61\xca\xc2\x86\x53\xa4\xe9\x5e\x9f\x2c\xd2\xe7\xf2\x44\x2f\x55\x9e\xbe\xa1\xfb\xe0\xf1\xd5\xfb\xb4\x8b\x4f\x0b\xbd\xb1\x13\xf9\x14\x8d\x43\x84\xce\xe0\x8b\x73\x87\x7a\x7e\x61\xe2\x2d\x1c\xd5\x4b\xfd\xc6\x36\x14\x53\xb6\x90\xe2\xaf\x0c\x65\x4b\x7a\x5d\x65\x3a\x1e\x97\xfe\x7e\x27\xd2\x27\x09\xfe\xe0\xe8\xfa\xef\x4d\xb2\x5e\x51\xea\xff\x9a\xf7\xd0\x6d\xb3\x55\x2d\xb0\x5a\x6b\x83\xbf\x48\x27\xfe\x6a\x69\x7a\x2d\xf8\x2a\x48\xd8\x23\xa2\x94\x5e\xd3\x3d\x5a\xe8\x68\x96\xbf\xc1\xda\x11\x51\x66\x1b\x1d\xad\x47\xb0\xd8\xf8\x6f\x21\x15\x3f\xa2\x5e\xe1\xfb\xdd\x5d\x7f\xbb\x0c\x25\x35\x47\x5f\x2e\x2f\x3f\x37\xe3\x10\x83\x05\x96\x2b\x08\x1f\x6e\xe1\x53\x1c\x8a\xa3\x4f\x8d\x21\x43\x43\x1a\x66\x69\xf3\x71\xd6\x14\x3c\x84\xf7\xd3\xf9\x7f\x00\x00\x00\xff\xff\xc8\xab\x89\x51\x57\x03\x00\x00")

func klusterletOperatorYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletOperatorYaml,
		"klusterlet/operator.yaml",
	)
}

func klusterletOperatorYaml() (*asset, error) {
	bytes, err := klusterletOperatorYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/operator.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _klusterletService_accountYaml = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x4a\x2c\xc8\x0c\x4b\x2d\x2a\xce\xcc\xcf\xb3\x52\x28\x33\xe4\xca\xce\xcc\x4b\xb1\x52\x08\x4e\x2d\x2a\xcb\x4c\x4e\x75\x4c\x4e\xce\x2f\xcd\x2b\xe1\xca\x4d\x2d\x49\x4c\x49\x2c\x49\xb4\xe2\x52\x50\xc8\x4b\xcc\x4d\xb5\x52\xc8\xce\x29\x2d\x2e\x49\x2d\xca\x49\x2d\x81\x0a\x15\x17\x24\x26\xa7\x5a\x29\x28\x55\x57\x2b\xe8\x79\xc3\x25\xfd\x60\x32\x0a\xb5\xb5\x4a\x5c\x80\x00\x00\x00\xff\xff\x42\xa3\x6c\x0b\x6b\x00\x00\x00")

func klusterletService_accountYamlBytes() ([]byte, error) {
	return bindataRead(
		_klusterletService_accountYaml,
		"klusterlet/service_account.yaml",
	)
}

func klusterletService_accountYaml() (*asset, error) {
	bytes, err := klusterletService_accountYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "klusterlet/service_account.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"hub/managedcluster/manifests/managedcluster-clusterrole.yaml":                     hubManagedclusterManifestsManagedclusterClusterroleYaml,
	"hub/managedcluster/manifests/managedcluster-clusterrolebinding.yaml":              hubManagedclusterManifestsManagedclusterClusterrolebindingYaml,
	"hub/managedcluster/manifests/managedcluster-service-account.yaml":                 hubManagedclusterManifestsManagedclusterServiceAccountYaml,
	"klusterlet/bootstrap_secret.yaml":                                                 klusterletBootstrap_secretYaml,
	"klusterlet/cluster_role.yaml":                                                     klusterletCluster_roleYaml,
	"klusterlet/cluster_role_binding.yaml":                                             klusterletCluster_role_bindingYaml,
	"klusterlet/crds/0000_00_operator.open-cluster-management.io_klusterlets.crd.yaml": klusterletCrds0000_00_operatorOpenClusterManagementIo_klusterletsCrdYaml,
	"klusterlet/image_pull_secret.yaml":                                                klusterletImage_pull_secretYaml,
	"klusterlet/klusterlet.yaml":                                                       klusterletKlusterletYaml,
	"klusterlet/klusterlet_admin_aggregate_clusterrole.yaml":                           klusterletKlusterlet_admin_aggregate_clusterroleYaml,
	"klusterlet/namespace.yaml":                                                        klusterletNamespaceYaml,
	"klusterlet/operator.yaml":                                                         klusterletOperatorYaml,
	"klusterlet/service_account.yaml":                                                  klusterletService_accountYaml,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"hub": &bintree{nil, map[string]*bintree{
		"managedcluster": &bintree{nil, map[string]*bintree{
			"manifests": &bintree{nil, map[string]*bintree{
				"managedcluster-clusterrole.yaml":        &bintree{hubManagedclusterManifestsManagedclusterClusterroleYaml, map[string]*bintree{}},
				"managedcluster-clusterrolebinding.yaml": &bintree{hubManagedclusterManifestsManagedclusterClusterrolebindingYaml, map[string]*bintree{}},
				"managedcluster-service-account.yaml":    &bintree{hubManagedclusterManifestsManagedclusterServiceAccountYaml, map[string]*bintree{}},
			}},
		}},
	}},
	"klusterlet": &bintree{nil, map[string]*bintree{
		"bootstrap_secret.yaml":     &bintree{klusterletBootstrap_secretYaml, map[string]*bintree{}},
		"cluster_role.yaml":         &bintree{klusterletCluster_roleYaml, map[string]*bintree{}},
		"cluster_role_binding.yaml": &bintree{klusterletCluster_role_bindingYaml, map[string]*bintree{}},
		"crds": &bintree{nil, map[string]*bintree{
			"0000_00_operator.open-cluster-management.io_klusterlets.crd.yaml": &bintree{klusterletCrds0000_00_operatorOpenClusterManagementIo_klusterletsCrdYaml, map[string]*bintree{}},
		}},
		"image_pull_secret.yaml":                      &bintree{klusterletImage_pull_secretYaml, map[string]*bintree{}},
		"klusterlet.yaml":                             &bintree{klusterletKlusterletYaml, map[string]*bintree{}},
		"klusterlet_admin_aggregate_clusterrole.yaml": &bintree{klusterletKlusterlet_admin_aggregate_clusterroleYaml, map[string]*bintree{}},
		"namespace.yaml":                              &bintree{klusterletNamespaceYaml, map[string]*bintree{}},
		"operator.yaml":                               &bintree{klusterletOperatorYaml, map[string]*bintree{}},
		"service_account.yaml":                        &bintree{klusterletService_accountYaml, map[string]*bintree{}},
	}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}