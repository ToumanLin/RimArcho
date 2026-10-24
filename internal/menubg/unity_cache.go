package menubg

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

func prepareUnity(ctx context.Context, game, cache, lang, stamp string) ([]Item, error) {
	defs, err := expansions(game, lang)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(game, "RimWorldWin64_Data")
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
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return nil, err
	}
	var out []Item
	var failures []error
	for _, d := range defs {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if d.Name != "Core" {
			st, err := os.Stat(filepath.Join(game, "Data", d.Name))
			if err != nil || !st.IsDir() {
				continue
			}
		}
		o, err := resolveUnityTexture(u, manager, resources, d.Path)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", d.Name, err))
			continue
		}
		hash := sha256.Sum256([]byte("menu-unity-v1|" + stamp + "|" + d.Path))
		base := filepath.Join(cache, fmt.Sprintf("menu-%x", hash[:12]))
		it := Item{Key: "game:" + strings.ToLower(d.Mod), Label: d.Label, Full: base + "-1920.jpg", Thumb: base + "-480.jpg"}
		if err := prepareUnityPair(ctx, u, o, it); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", d.Name, err))
			continue
		}
		out = append(out, it)
	}
	if stamp != Stamp(game) {
		return nil, errors.New("Unity resources changed while reading menu backgrounds")
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

func prepareUnityPair(ctx context.Context, u *unityFile, o unityObject, it Item) error {
	decodeMu.Lock()
	defer decodeMu.Unlock()
	if regular(it.Full) && regular(it.Thumb) {
		return nil
	}
	defer debug.FreeOSMemory()
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
		if err := writeJPEG(images[i], path); err != nil {
			return err
		}
	}
	return nil
}
