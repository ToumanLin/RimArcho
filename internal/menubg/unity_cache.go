package menubg

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
)

func prepareUnity(ctx context.Context, game, cache string, defs []expansion) ([]Item, error) {
	stamp := Stamp(game)
	root := env.UnityDataDir(game)
	u, err := openUnity(filepath.Join(root, "resources.assets"))
	if err != nil {
		return nil, err
	}
	defer u.f.Close()
	var resources map[string]unityPtr
	manager, err := openUnity(filepath.Join(root, "globalgamemanagers"))
	if err == nil {
		defer manager.f.Close()
		resources, _ = manager.resources()
	}
	var out []Item
	var failures []error
	for _, d := range defs {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		o, err := resolveUnityTexture(u, manager, resources, d.Path)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", d.Name, err))
			continue
		}
		it := cacheItem(cache, d)
		if err := prepareSharedPair(ctx, game, stamp, it, func(staged Item) error {
			return extractUnityPair(ctx, u, o, staged)
		}); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", d.Name, err))
			continue
		}
		out = append(out, it)
	}
	return out, errors.Join(failures...)
}

func resolveUnityTexture(u, manager *unityFile, resources map[string]unityPtr, path string) (unityObject, error) {
	if resources == nil {
		return u.textureByName(filepath.Base(strings.ReplaceAll(path, `\`, "/")))
	}
	p, ok := resources["textures/"+strings.ToLower(strings.Trim(path, "/"))]
	if !ok {
		return unityObject{}, fmt.Errorf("resource path %s not found", path)
	}
	target := manager
	if p.file != 0 {
		if p.file < 1 || int(p.file) > len(manager.externals) {
			return unityObject{}, errors.New("invalid Unity resource file ID")
		}
		external := strings.ReplaceAll(manager.externals[p.file-1], `\`, "/")
		if external != "resources.assets" {
			return unityObject{}, fmt.Errorf("unsupported Unity resource file %s", external)
		}
		target = u
	}
	if target != u {
		return unityObject{}, errors.New("texture is not in resources.assets")
	}
	o, ok := u.objects[p.id]
	if !ok || o.class != 28 {
		return unityObject{}, errors.New("resource Texture2D object missing")
	}
	return o, nil
}

func extractUnityPair(ctx context.Context, u *unityFile, o unityObject, it Item) error {
	t, err := u.texture(o)
	if err != nil {
		return err
	}
	d, closer, err := u.textureReader(t)
	if err != nil {
		return err
	}
	defer closer.Close()
	images, err := d.Scaled(ctx, 1920, 480)
	if err != nil {
		return err
	}
	for i, path := range []string{it.Full, it.Thumb} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeJPEG(images[i], path); err != nil {
			return err
		}
	}
	return nil
}
