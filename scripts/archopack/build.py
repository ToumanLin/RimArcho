"""Build Archo's figure pack (16 §7.1 "Archo figure") from the baked expression GIFs.

    python scripts/archopack/build.py [--src DIR] [--out DIR]

--src is crayon-rig's GIF folder, by default
D:/Game/RimModManager_Upgrade/crayon-rig/characters/archo/build/gif-notalk, baked with

    node engine/bake/bake_gifs.mjs --no-talk --top 59 --out characters/archo/build/gif-notalk characters/archo <expr...>

(crayon-rig engine/bake/README.md): no talking mouth, since RimArcho has no chat, and 20 rows
above the old top edge, which the ribbon tip used to touch. Each GIF in its Selected/ folder is
one expression. When _frames/<expr>/NN.png holds the same frames with full alpha (crayon-rig
writes them with the GIF), those are used for smooth edges; a frame that differs from its GIF
frame fails the build, so a stale _frames folder never ships.

Every frame gets the same crop: Archo's head and chest. The box keeps the frame's whole width,
because her hair swings to both edges (spiral, dizzy, shake), and runs from ABOVE px above the
old top edge (row 0 of a 400 x 400 bake; the bake's own extra rows sit above it) down to the
chest line. A bake without extra rows is padded with transparent rows instead.

Output, in --out (default internal/surface/gui/assets/archo/archo-s):
  stills/<expr>@360.webp  frame 0, 360 px wide, for ui.archo.motion still and reduced motion
  anim/<expr>.webp        the loop, 300 px wide, every baked frame at FRAME_MS, loops forever
Frames all get the same delay: a mixed 67/67/66 ms pattern measured more uneven in Edge than a
uniform 66 ms (headless Edge, 2026-11-19). The GIF's own delay is not used, as
GIF stores centiseconds (crayon-rig's 67 ms reads back as 60 ms).
Needs Pillow (with WebP).
"""
import argparse
import os
import shutil
import sys

from PIL import Image

SRC_W, CROP_BOTTOM, ABOVE = 400, 252, 18  # source px: full width, head to chest, room above the old top edge
STILL_W, ANIM_W, ANIM_Q, STILL_Q = 360, 300, 70, 82
FRAME_MS = 66  # crayon-rig bakes at 15 fps (66.7 ms); a whole number of ms, the same for every frame
H = CROP_BOTTOM + ABOVE


def frames(src, name):
    gif = Image.open(os.path.join(src, "Selected", name + ".gif"))
    if gif.width != SRC_W or gif.height < SRC_W:
        sys.exit(f"{name}.gif is {gif.size}, expected {SRC_W} wide and at least {SRC_W} high")
    pngs = os.path.join(src, "_frames", name)
    out = []
    for i in range(gif.n_frames):
        gif.seek(i)
        g = gif.convert("RGBA")
        p = os.path.join(pngs, f"{i:02d}.png")
        if os.path.exists(p):
            f = Image.open(p).convert("RGBA")
            if not same(g, f):
                sys.exit(f"{p} differs from frame {i} of {name}.gif: re-bake crayon-rig or remove _frames")
        else:
            f = g
        out.append(f)
    return out


def same(gif, png):
    """The GIF frame is the PNG quantized to 256 colours with 1-bit alpha."""
    if gif.size != png.size:
        return False
    a, b = gif.getchannel("A"), png.getchannel("A")
    ga, pa = a.tobytes(), b.point(lambda v: 255 if v > 127 else 0).tobytes()
    if sum(1 for x, y in zip(ga[::7], pa[::7]) if (x > 0) != (y > 0)) > len(ga[::7]) // 200:
        return False
    gr, pr = gif.convert("RGB").tobytes(), png.convert("RGB").tobytes()
    step = 3 * 97
    diff = [abs(gr[i] - pr[i]) for i in range(0, len(gr), step) if ga[i // 3] and pa[i // 3]]
    return not diff or sum(diff) / len(diff) < 8


def framed(f, width):
    extra = f.height - SRC_W  # rows the bake added above the old top edge (--top)
    top = extra - ABOVE
    c = Image.new("RGBA", (SRC_W, H), (0, 0, 0, 0))
    c.paste(f.crop((0, max(top, 0), SRC_W, extra + CROP_BOTTOM)), (0, max(-top, 0)))
    return c.resize((width, round(H * width / SRC_W)), Image.LANCZOS)


def top_clear(fr):
    """Transparent rows above the highest stroke over all frames (the ribbon's room)."""
    rows = []
    for f in fr:
        box = framed(f, SRC_W).getchannel("A").point(lambda v: 255 if v > 8 else 0).getbbox()
        rows.append(box[1] if box else H)
    return min(rows)


def main():
    root = os.path.normpath(os.path.join(os.path.dirname(__file__), "..", ".."))
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", default="D:/Game/RimModManager_Upgrade/crayon-rig/characters/archo/build/gif-notalk")
    ap.add_argument("--out", default=os.path.join(root, "internal", "surface", "gui", "assets", "archo", "archo-s"))
    a = ap.parse_args()
    names = sorted(n[:-4] for n in os.listdir(os.path.join(a.src, "Selected")) if n.endswith(".gif"))
    if not names:
        sys.exit("no GIFs in Selected/")
    shutil.rmtree(a.out, ignore_errors=True)
    for d in ("stills", "anim"):
        os.makedirs(os.path.join(a.out, d))
    total = 0
    for n in names:
        fr = frames(a.src, n)
        clear = top_clear(fr)
        if clear < 2:
            print(f"warning: {n} touches the top edge (bake with crayon-rig --top)")
        still = os.path.join(a.out, "stills", f"{n}@360.webp")
        framed(fr[0], STILL_W).save(still, quality=STILL_Q, method=6, alpha_quality=85)
        loop = [framed(f, ANIM_W) for f in fr]
        anim = os.path.join(a.out, "anim", f"{n}.webp")
        loop[0].save(anim, save_all=True, append_images=loop[1:], duration=FRAME_MS, loop=0,
                     quality=ANIM_Q, method=6, alpha_quality=80, minimize_size=True)
        size = os.path.getsize(still) + os.path.getsize(anim)
        total += size
        print(f"{n:10} still {os.path.getsize(still) // 1024} KB, loop {len(loop)} x {FRAME_MS} ms "
              f"{os.path.getsize(anim) // 1024} KB, {clear} px clear above")
    print(f"{len(names)} expressions, {total / 1e6:.2f} MB")


if __name__ == "__main__":
    main()
