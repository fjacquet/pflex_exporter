package models

import (
	"encoding/json"
	"strings"
)

// PowerFlex object type names (normalized).
const (
	TypeSystem           = "System"
	TypeSds              = "Sds"         // Gen1 storage server
	TypeSdc              = "Sdc"         // storage client (both gens)
	TypeVolume           = "Volume"      // both gens
	TypeStoragePool      = "StoragePool" // both gens
	TypeDevice           = "Device"      // both gens
	TypeProtectionDomain = "ProtectionDomain"

	// Gen2-only object types.
	TypeStorageNode = "StorageNode" // Gen2 storage server (renamed from Sds)
	TypeDeviceGroup = "DeviceGroup" // Gen2 grouping of devices by media type
	TypeSdt         = "Sdt"         // Gen2 NVMe/TCP target
)

// Link is a PowerFlex object relationship link as returned in an object's "links" array.
type Link struct {
	Rel  string `json:"rel"`
	HRef string `json:"href"`
}

// Instance is a single PowerFlex object. Only the fields the exporter needs are modeled;
// the rest of the API payload is ignored. Name may be null in the API (-> empty string),
// in which case callers fall back to ID.
type Instance struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Links                 []Link `json:"links"`
	DeviceCurrentPathName string `json:"deviceCurrentPathName,omitempty"`
	SdcIP                 string `json:"sdcIp,omitempty"`
	DataLayout            string `json:"dataLayout,omitempty"` // StoragePool: generation discriminator
	VolumeType            string `json:"volumeType,omitempty"` // Gen2 Volume
	MediaType             string `json:"mediaType,omitempty"`  // Gen2 DeviceGroup
}

// DisplayName returns Name, or ID when Name is empty (mirrors Dell's fallback).
func (i *Instance) DisplayName() string {
	if i.Name != "" {
		return i.Name
	}
	return i.ID
}

// Instances holds the parsed result of GET /api/instances: the single System object
// plus every other object grouped by normalized type name.
type Instances struct {
	System *Instance
	ByType map[string][]*Instance
}

// Get returns the objects of a given normalized type (e.g. TypeSds).
func (in *Instances) Get(objType string) []*Instance {
	return in.ByType[objType]
}

// ParseInstances decodes a GET /api/instances response into typed Instances and the
// parent/child Relations graph derived from each object's links.
//
// The raw response is a JSON object whose keys are "System" (a single object) and
// "<type>List" arrays. Keys are normalized to type names: "sdsList" -> "Sds",
// "storagePoolList" -> "StoragePool", etc.
func ParseInstances(body []byte) (*Instances, *Relations, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil, err
	}

	instances := &Instances{ByType: make(map[string][]*Instance)}
	relations := NewRelations()

	for key, msg := range raw {
		if key != TypeSystem && !strings.HasSuffix(key, "List") {
			continue
		}
		objType := normalizeType(key)

		var objs []*Instance
		if key == TypeSystem {
			var sys Instance
			if err := json.Unmarshal(msg, &sys); err != nil {
				return nil, nil, err
			}
			instances.System = &sys
			objs = []*Instance{&sys}
		} else {
			if err := json.Unmarshal(msg, &objs); err != nil {
				return nil, nil, err
			}
			instances.ByType[objType] = objs
		}

		for _, obj := range objs {
			relations.addFromLinks(objType, obj)
		}
	}

	return instances, relations, nil
}

// normalizeType converts an /api/instances key to a normalized type name:
// capitalize the first rune and strip a trailing "List".
func normalizeType(key string) string {
	t := strings.TrimSuffix(key, "List")
	if t == "" {
		return t
	}
	return strings.ToUpper(t[:1]) + t[1:]
}
