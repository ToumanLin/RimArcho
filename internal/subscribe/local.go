package subscribe

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/go-git/go-git/v5/utils/ioutil"
)

// go-git's embedded server lacks shallow requests and peeled tags, and its
// filesystem loader does not open a worktree's .git. Supply those for local URLs.
type localLoader struct{}

func (localLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	p := ep.Path
	if len(p) > 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	r, err := git.PlainOpen(filepath.FromSlash(p))
	if err != nil {
		return nil, err
	}
	return r.Storer, nil
}

type localTransport struct{ transport.Transport }

func newLocalTransport() transport.Transport {
	return localTransport{server.NewClient(localLoader{})}
}

func (t localTransport) NewUploadPackSession(ep *transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	s, err := t.Transport.NewUploadPackSession(ep, auth)
	if err != nil {
		return nil, err
	}
	sto, err := (localLoader{}).Load(ep)
	if err != nil {
		s.Close()
		return nil, err
	}
	return &localUpload{UploadPackSession: s, store: sto}, nil
}

type localUpload struct {
	transport.UploadPackSession
	store storer.Storer
}

func (s *localUpload) AdvertisedReferences() (*packp.AdvRefs, error) {
	return s.AdvertisedReferencesContext(context.Background())
}

func (s *localUpload) AdvertisedReferencesContext(ctx context.Context) (*packp.AdvRefs, error) {
	ar, err := s.UploadPackSession.AdvertisedReferencesContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := ar.Capabilities.Set(capability.Shallow); err != nil {
		return nil, err
	}
	for name, hash := range ar.References {
		if !plumbing.ReferenceName(name).IsTag() {
			continue
		}
		peeled := hash
		for {
			tag, err := object.GetTag(s.store, peeled)
			if err == plumbing.ErrObjectNotFound {
				break
			}
			if err != nil {
				return nil, err
			}
			peeled = tag.Target
		}
		if peeled != hash {
			ar.Peeled[name] = peeled
		}
	}
	return ar, nil
}

func (s *localUpload) UploadPack(ctx context.Context, req *packp.UploadPackRequest) (*packp.UploadPackResponse, error) {
	if req.Depth.IsZero() {
		return s.UploadPackSession.UploadPack(ctx, req)
	}
	if req.Depth != packp.DepthCommits(1) {
		return nil, fmt.Errorf("local subscriptions support depth 1 only")
	}
	var hashes, shallow []plumbing.Hash
	seen := map[plumbing.Hash]bool{}
	var visit func(plumbing.Hash) error
	visit = func(hash plumbing.Hash) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[hash] {
			return nil
		}
		seen[hash] = true
		o, err := object.GetObject(s.store, hash)
		if err != nil {
			return err
		}
		hashes = append(hashes, hash)
		switch o := o.(type) {
		case *object.Tag:
			return visit(o.Target)
		case *object.Commit:
			if len(o.ParentHashes) > 0 {
				shallow = append(shallow, hash)
			}
			return visit(o.TreeHash)
		case *object.Tree:
			for _, e := range o.Entries {
				if e.Mode == filemode.Submodule {
					continue
				}
				if err := visit(e.Hash); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, hash := range req.Wants {
		if err := visit(hash); err != nil {
			return nil, err
		}
	}
	r, w := io.Pipe()
	go func() {
		_, err := packfile.NewEncoder(w, s.store, false).Encode(hashes, 0)
		w.CloseWithError(err)
	}()
	resp := packp.NewUploadPackResponseWithPackfile(req, ioutil.NewContextReadCloser(ctx, r))
	resp.Shallows = shallow
	return resp, nil
}
