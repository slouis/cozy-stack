package apps

import (
	"encoding/json"
	"io"
	"net/url"
	"path"
	"regexp"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

var slugReg = regexp.MustCompile(`^[A-Za-z0-9\-]+$`)

// Installer is used to install or update applications.
type Installer struct {
	fetcher Fetcher
	ctx     vfs.Context

	man  *Manifest
	src  *url.URL
	slug string

	err  error
	errc chan error
	manc chan *Manifest
}

// InstallerOptions provides the slug name of the application along with the
// source URL.
type InstallerOptions struct {
	Slug      string
	SourceURL string
}

// Fetcher interface should be implemented by the underlying transport
// used to fetch the application data.
type Fetcher interface {
	// FetchManifest should returns an io.ReadCloser to read the
	// manifest data
	FetchManifest(src *url.URL) (io.ReadCloser, error)
	// Fetch should download the application and install it in the given
	// directory.
	Fetch(src *url.URL, appDir string) error
}

// NewInstaller creates a new Installer
func NewInstaller(ctx vfs.Context, opts *InstallerOptions) (*Installer, error) {
	slug := opts.Slug
	if slug == "" || !slugReg.MatchString(slug) {
		return nil, ErrInvalidSlugName
	}

	man, err := GetBySlug(ctx, slug)
	if err != nil && !couchdb.IsNotFoundError(err) {
		return nil, err
	}

	var src *url.URL
	if opts.SourceURL != "" {
		src, err = url.Parse(opts.SourceURL)
	} else if man != nil {
		src, err = url.Parse(man.Source)
	} else {
		err = ErrNotSupportedSource
	}
	if err != nil {
		return nil, err
	}

	var fetcher Fetcher
	switch src.Scheme {
	case "git":
		fetcher = newGitFetcher(ctx)
	default:
		return nil, ErrNotSupportedSource
	}

	inst := &Installer{
		fetcher: fetcher,
		ctx:     ctx,
		src:     src,
		slug:    slug,
		man:     man,
		errc:    make(chan error),
		manc:    make(chan *Manifest, 1),
	}

	return inst, nil
}

// InstallOrUpdate will install the application linked to the installer. If the
// application is already installed, it will try to upgrade it. It will report
// its progress or error (see Poll method).
func (i *Installer) InstallOrUpdate() {
	defer i.endOfProc()

	if i.man == nil {
		i.man, i.err = i.install()
		return
	}

	state := i.man.State
	if state != Ready && state != Errored {
		i.man, i.err = nil, ErrBadState
		return
	}

	i.man, i.err = i.update()
	return
}

func (i *Installer) endOfProc() {
	man, err := i.man, i.err
	if man == nil || err == ErrBadState {
		i.errc <- err
		return
	}
	if err != nil {
		man.State = Errored
		man.Error = err.Error()
		updateManifest(i.ctx, man)
		i.errc <- err
		return
	}
	man.State = Ready
	updateManifest(i.ctx, man)
	i.manc <- i.man
}

// install will perform the installation of an application. It returns the
// freshly fetched manifest from the source along with a possible error in case
// the installation went wrong.
//
// Note that the fetched manifest is returned even if an error occured while
// upgrading.
func (i *Installer) install() (*Manifest, error) {
	man := &Manifest{}
	if err := i.ReadManifest(Installing, man); err != nil {
		return nil, err
	}

	if err := createManifest(i.ctx, man); err != nil {
		return man, err
	}

	i.manc <- man

	appdir := i.appDir()
	if _, err := vfs.MkdirAll(i.ctx, appdir, nil); err != nil {
		return man, err
	}

	if err := i.fetcher.Fetch(i.src, appdir); err != nil {
		return man, err
	}

	return man, nil
}

// update will perform the update of an already installed application. It
// returns the freshly fetched manifest from the source along with a possible
// error in case the update went wrong.
//
// Note that the fetched manifest is returned even if an error occured while
// upgrading.
func (i *Installer) update() (*Manifest, error) {
	man := i.man
	version := man.Version

	if err := i.ReadManifest(Upgrading, man); err != nil {
		return man, err
	}

	if man.Version == version {
		return man, nil
	}

	if err := updateManifest(i.ctx, man); err != nil {
		return man, err
	}

	i.manc <- man

	err := i.fetcher.Fetch(i.src, i.appDir())
	return man, err
}

// ReadManifest will fetch the manifest and read its JSON content into the
// passed manifest pointer.
//
// The State field of the manifest will be set to the specified state.
func (i *Installer) ReadManifest(state State, man *Manifest) error {
	r, err := i.fetcher.FetchManifest(i.src)
	if err != nil {
		return err
	}
	defer r.Close()

	err = json.NewDecoder(io.LimitReader(r, ManifestMaxSize)).Decode(man)
	if err != nil {
		return ErrBadManifest
	}

	man.Slug = i.slug
	man.Source = i.src.String()
	man.State = state

	if man.Routes == nil {
		man.Routes = make(Routes)
		man.Routes["/"] = Route{
			Folder: "/",
			Index:  "index.html",
			Public: false,
		}
	}

	return nil
}

func (i *Installer) appDir() string {
	return path.Join(vfs.AppsDirName, i.slug)
}

// Poll should be used to monitor the progress of the Installer.
func (i *Installer) Poll() (man *Manifest, done bool, err error) {
	select {
	case man = <-i.manc:
		done = man.State == Ready
		return
	case err = <-i.errc:
		return
	}
}

func updateManifest(db couchdb.Database, man *Manifest) error {

	if err := permissions.Destoy(db, man.Slug); err != nil {
		return err
	}

	if err := couchdb.UpdateDoc(db, man); err != nil {
		return err
	}

	return permissions.Create(db, man.Slug)
}

func createManifest(db couchdb.Database, man *Manifest) error {

	if err := couchdb.CreateNamedDoc(db, man); err != nil {
		return err
	}

	return permissions.Create(db, man.Slug)

}
