"""Build Archo's figure pack (16 §7.1 "Archo figure") from the baked expression GIFs.

    python scripts/archopack/build.py [--src DIR] [--out DIR]

--src is crayon-rig's GIF folder, by default
D:/Game/RimModManager_Upgrade/crayon-rig/characters/archo/build/gif. Each GIF in its
Selected/ folder is one expression. When _frames/<expr>/NN.png holds the same frames with
full alpha (crayon-rig writes them with the GIF), those are used for smooth edges; a frame
that differs from its GIF frame fails the build, so a stale _frames folder never ships.

Every frame gets the same crop: Archo's head and chest. The box keeps the GIF's whole width,
because her hair swings to both edges (spiral, dizzy, shake), and the top 252 px, down to the
chest line, under a 12 px transparent margin so no box ever touches her ribbon.

Output, in --out (default internal/surface/gui/assets/archo/archo-s):
  stills/<expr>@360.webp  frame 0, 360 px wide, the image every figure shows first
  anim/<expr>.webp        the loop, 300 px wide, every second frame (8.3 fps), loops forever
The loop's first frame is the still, so swapping one for the other does not jump.
Needs Pillow (with WebP).
"""
import argparse
import os
import shutil
import sys

from PIL import Image

CROP_W, CROP_TOP, TOP_PAD = 400, 252, 12  # source px: full width, head to chest, margin above
STILL_W, ANIM_W, ANIM_STEP, ANIM_Q, STILL_Q = 360, 300, 2, 70, 82
H = CROP_TOP + TOP_PAD


def frames(src, name):
    gif = Image.open(os.path.join(src, "Selected", name + ".gif"))
    if gif.size != (CROP_W, CROP_W):
        sys.exit(f"{name}.gif is {gif.size}, expected {CROP_W}x{CROP_W}")
    dur = gif.info.get("duration", 60)
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
    return out, dur


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
    c = Image.new("RGBA", (CROP_W, H), (0, 0, 0, 0))
    c.paste(f.crop((0, 0, CROP_W, CROP_TOP)), (0, TOP_PAD))
    return c.resize((width, round(H * width / CROP_W)), Image.LANCZOS)


def main():
    root = os.path.normpath(os.path.join(os.path.dirname(__file__), "..", ".."))
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", default="D:/Game/RimModManager_Upgrade/crayon-rig/characters/archo/build/gif")
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
        fr, dur = frames(a.src, n)
        still = os.path.join(a.out, "stills", f"{n}@360.webp")
        framed(fr[0], STILL_W).save(still, quality=STILL_Q, method=6, alpha_quality=85)
        loop = [framed(f, ANIM_W) for f in fr[::ANIM_STEP]]
        anim = os.path.join(a.out, "anim", f"{n}.webp")
        loop[0].save(anim, save_all=True, append_images=loop[1:], duration=dur * ANIM_STEP, loop=0,
                     quality=ANIM_Q, method=6, alpha_quality=80, minimize_size=True)
        size = os.path.getsize(still) + os.path.getsize(anim)
        total += size
        print(f"{n:10} still {os.path.getsize(still) // 1024} KB, loop {len(loop)} x {dur * ANIM_STEP} ms {os.path.getsize(anim) // 1024} KB")
    print(f"{len(names)} expressions, {total / 1e6:.2f} MB")


if __name__ == "__main__":
    main()
