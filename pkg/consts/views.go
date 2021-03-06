package consts

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
)

// IndexViewsVersion is the version of current definition of views & indexes.
// This number should be incremented when this file changes.
const IndexViewsVersion int = 14

// GlobalIndexes is the index list required on the global databases to run
// properly.
var GlobalIndexes = []*mango.Index{
	mango.IndexOnFields(Instances, "by-domain", []string{"domain"}),
}

// Indexes is the index list required by an instance to run properly.
var Indexes = []*mango.Index{
	// Permissions
	mango.IndexOnFields(Permissions, "by-source-and-type", []string{"source_id", "type"}),

	// Used to lookup over the children of a directory
	mango.IndexOnFields(Files, "dir-children", []string{"dir_id", "_id"}),
	// Used to lookup a directory given its path
	mango.IndexOnFields(Files, "dir-by-path", []string{"path"}),

	// Used to lookup a queued and running jobs
	mango.IndexOnFields(Jobs, "by-worker-and-state", []string{"worker", "state"}),
	mango.IndexOnFields(Jobs, "by-trigger-id", []string{"trigger_id", "queued_at"}),

	// Used to lookup oauth clients by name
	mango.IndexOnFields(OAuthClients, "by-client-name", []string{"client_name"}),

	// Used to looked login history by OS, browser, and IP
	mango.IndexOnFields(SessionsLogins, "by-os-browser-ip", []string{"os", "browser", "ip"}),
}

// DiskUsageView is the view used for computing the disk usage
var DiskUsageView = &couchdb.View{
	Name:    "disk-usage",
	Doctype: Files,
	Map: `
function(doc) {
  if (doc.type === 'file') {
    emit(doc._id, +doc.size);
  }
}
`,
	Reduce: "_sum",
}

// FilesReferencedByView is the view used for fetching files referenced by a
// given document
var FilesReferencedByView = &couchdb.View{
	Name:    "referenced-by",
	Doctype: Files,
	Reduce:  "_count",
	Map: `
function(doc) {
  if (isArray(doc.referenced_by)) {
    for (var i = 0; i < doc.referenced_by.length; i++) {
      emit([doc.referenced_by[i].type, doc.referenced_by[i].id]);
    }
  }
}`,
}

// ReferencedBySortedByDatetimeView is the view used for fetching files referenced by a
// given document, sorted by the datetime
var ReferencedBySortedByDatetimeView = &couchdb.View{
	Name:    "referenced-by-sorted-by-datetime",
	Doctype: Files,
	Reduce:  "_count",
	Map: `
function(doc) {
  if (isArray(doc.referenced_by)) {
    for (var i = 0; i < doc.referenced_by.length; i++) {
      var datetime = (doc.metadata && doc.metadata.datetime) || '';
      emit([doc.referenced_by[i].type, doc.referenced_by[i].id, datetime]);
    }
  }
}`,
}

// FilesByParentView is the view used for fetching files referenced by a
// given document
var FilesByParentView = &couchdb.View{
	Name:    "by-parent-type-name",
	Doctype: Files,
	Map: `
function(doc) {
  emit([doc.dir_id, doc.type, doc.name])
}`,
	Reduce: "_count",
}

// PermissionsShareByCView is the view for fetching the permissions associated
// to a document via a token code.
var PermissionsShareByCView = &couchdb.View{
	Name:    "byToken",
	Doctype: Permissions,
	Map: `
function(doc) {
  if (doc.type && doc.type.slice(0, 5) === "share" && doc.codes) {
    Object.keys(doc.codes).forEach(function(k) {
      emit(doc.codes[k]);
    })
  }
}`,
}

// PermissionsShareByDocView is the view for fetching a list of permissions
// associated to a list of IDs.
var PermissionsShareByDocView = &couchdb.View{
	Name:    "byDoc",
	Doctype: Permissions,
	Map: `
function(doc) {
  if (doc.type === "share" && doc.permissions) {
    Object.keys(doc.permissions).forEach(function(k) {
      var p = doc.permissions[k];
      var selector = p.selector || "_id";
      for (var i=0; i<p.values.length; i++) {
        emit([p.type, selector, p.values[i]], p.verbs);
      }
    });
  }
}`,
}

// PermissionsByDoctype returns a list of permissions that have at least one
// rule for the given doctype.
var PermissionsByDoctype = &couchdb.View{
	Name:    "permissions-by-doctype",
	Doctype: Permissions,
	Map: `
function(doc) {
  if (doc.permissions) {
    Object.keys(doc.permissions).forEach(function(k) {
	  emit([doc.permissions[k].type, doc.type]);
	});
  }
}
`,
}

// Views is the list of all views that are created by the stack.
var Views = []*couchdb.View{
	DiskUsageView,
	FilesReferencedByView,
	ReferencedBySortedByDatetimeView,
	FilesByParentView,
	PermissionsShareByCView,
	PermissionsShareByDocView,
	PermissionsByDoctype,
}

// ViewsByDoctype returns the list of views for a specified doc type.
func ViewsByDoctype(doctype string) []*couchdb.View {
	var views []*couchdb.View
	for _, view := range Views {
		if view.Doctype == doctype {
			views = append(views, view)
		}
	}
	return views
}

// IndexesByDoctype returns the list of indexes for a specified doc type.
func IndexesByDoctype(doctype string) []*mango.Index {
	var indexes []*mango.Index
	for _, index := range Indexes {
		if index.Doctype == doctype {
			indexes = append(indexes, index)
		}
	}
	return indexes
}
