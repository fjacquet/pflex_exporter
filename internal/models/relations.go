package models

import "strings"

// Relations is the parent/child graph derived from PowerFlex object links.
// It is required to attach parent labels (e.g. a Device's SDS, StoragePool and
// ProtectionDomain) to metrics, mirroring Dell's relations dict.
type Relations struct {
	// Parents maps objectID -> parentType -> []parentID.
	Parents map[string]map[string][]string
	// Children maps parentID -> childType -> []childID.
	Children map[string]map[string][]string
}

// NewRelations returns an empty Relations graph.
func NewRelations() *Relations {
	return &Relations{
		Parents:  make(map[string]map[string][]string),
		Children: make(map[string]map[string][]string),
	}
}

// addFromLinks records parent relationships found in an object's links. A parent link
// has rel starting with "/api/parent" and an href like ".../<ParentType>::<parentID>".
func (r *Relations) addFromLinks(objType string, obj *Instance) {
	for _, link := range obj.Links {
		if !strings.HasPrefix(link.Rel, "/api/parent") {
			continue
		}
		href := link.HRef
		firstColon := strings.Index(href, ":")
		lastColon := strings.LastIndex(href, ":")
		if firstColon == -1 || lastColon == -1 {
			continue
		}
		beforeColon := href[:firstColon]
		parentType := beforeColon[strings.LastIndex(beforeColon, "/")+1:]
		parentID := href[lastColon+1:]
		if parentType == "" || parentID == "" {
			continue
		}

		if r.Parents[obj.ID] == nil {
			r.Parents[obj.ID] = make(map[string][]string)
		}
		r.Parents[obj.ID][parentType] = append(r.Parents[obj.ID][parentType], parentID)

		if r.Children[parentID] == nil {
			r.Children[parentID] = make(map[string][]string)
		}
		r.Children[parentID][objType] = append(r.Children[parentID][objType], obj.ID)
	}
}

// ParentIDs returns the parent IDs of the given object for a parent type.
func (r *Relations) ParentIDs(objID, parentType string) []string {
	if m, ok := r.Parents[objID]; ok {
		return m[parentType]
	}
	return nil
}

// FirstParent returns the first parent instance of parentType for objID, found by
// scanning candidates. Returns nil if no matching parent exists.
func (r *Relations) FirstParent(objID, parentType string, candidates []*Instance) *Instance {
	ids := r.ParentIDs(objID, parentType)
	if len(ids) == 0 {
		return nil
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	for _, c := range candidates {
		if _, ok := idSet[c.ID]; ok {
			return c
		}
	}
	return nil
}
