"""Build Archo's figure pack (16 §7.1 "Archo figure") from crayon-rig's baked clips.

    python scripts/archopack/build.py [--src DIR] [--out DIR]

--src is crayon-rig's clip folder, by default
D:/Game/RimModManager_Upgrade/crayon-rig/characters/archo/build/clips-notalk, baked with

    node engine/bake/bake_clips.mjs --no-talk --top 59 --out characters/archo/build/clips-notalk characters/archo <expr...>

(crayon-rig engine/bake/README.md "Clips"): no talking mouth, since RimArcho has no chat, and
20 rows above the old top edge, which the ribbon tip used to touch. The folder holds
clips.json and _frames/<clip>/NN.png (full alpha). Every clip runs on the same 2.4 s clock of
36 phases (P = 36, K = clips.json "k", 12):

  <e>       the expression's loop, phases 0..35
  <e>-out   from <e> back to the idle, phases 0..K-1 (follows the loop's last frame)
  bridge    the idle, phases K..35-K (between an out and the next in)
  <e>-in    from the idle to <e>, phases 36-K..35 (the loop's frame 0 follows)
  idle      the calm idle, phases 36-K..35-K twice (blinks on the first pass only)

so any frame sequence that follows those joins is seamless, and js/archo.js splices clips into
one animated WebP (each clip starts with a key frame that covers the whole canvas).

Every frame gets the same crop: Archo's head and chest. The box keeps the frame's whole width,
because her hair swings to both edges (spiral, dizzy, shake), and runs from ABOVE px above the
old top edge (row 0 of a 400 x 400 bake; the bake's own extra rows sit above it) down to the
chest line.

Output, in --out (default internal/surface/gui/assets/archo/archo-s):
  stills/<e>@360.webp   frame 0 of the loop, 360 px wide, for ui.archo.motion still and reduced motion
  stills/idle@360.webp  the idle's frame 0 (a paused idle shows it)
  anim/<clip>.webp      every clip, 300 px wide, FRAME_MS per frame; transitions at TRANS_Q
The idle shows every IDLE_STEP-th frame (its breath and sway are slow) and every frame of its
blink, holding the previous frame longer, so it decodes fewer frames per second; its two passes
keep their 36-phase lengths, so a join can follow either.
Frames all get the same delay: a mixed 67/67/66 ms pattern measured more uneven in Edge than a
uniform 66 ms (headless Edge, 2026-11-19).
Needs Pillow (with WebP).
"""
import argparse
import json
import os
import shutil
import sys

from PIL import Image

SRC_W, CROP_BOTTOM, ABOVE = 400, 252, 18  # source px: full width, head to chest, room above the old top edge
STILL_W, ANIM_W, ANIM_Q, TRANS_Q, STILL_Q = 360, 300, 70, 60, 82
FRAME_MS = 66  # crayon-rig bakes at 15 fps (66.7 ms); a whole number of ms, the same for every frame
IDLE_STEP = 2  # the idle shows every 2nd frame (7.5 fps: its breath is slow), but every frame of a blink
H = CROP_BOTTOM + ABOVE


def frames(src, clip):
    d = os.path.join(src, "_frames", clip)
    names = sorted(n for n in os.listdir(d) if n.endswith(".png"))
    if not names:
        sys.exit(f"{d}: no frames")
    out = [Image.open(os.path.join(d, n)).convert("RGBA") for n in names]
    if out[0].width != SRC_W or out[0].height < SRC_W:
        sys.exit(f"{clip} is {out[0].size}, expected {SRC_W} wide and at least {SRC_W} high")
    return out


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


def held(fr):
    """The idle at IDLE_STEP: a dropped frame lengthens the one before it. Its two passes are the
    same but for the blink, so a frame where they differ (and the one after) is a blink and kept;
    each pass starts on a kept frame, so a join can follow either."""
    p = len(fr) // 2
    blink = [fr[i % p].tobytes() != fr[i % p + p].tobytes() for i in range(len(fr))]
    out, ms = [], []
    for i, f in enumerate(fr):
        keep = i % p == 0 or i % IDLE_STEP == 0 or blink[i] or blink[i - 1]
        if out and not keep:
            ms[-1] += FRAME_MS
            continue
        out.append(f)
        ms.append(FRAME_MS)
    return out, ms


def corners(f):
    """Two opposite corner pixels at alpha 1 (invisible): the encoder crops a key frame to what
    is not fully transparent, and a spliced clip's first frame must cover the whole canvas, or
    the clip before it would show through around her."""
    f = f.copy()
    for xy in ((0, 0), (f.width - 1, f.height - 1)):
        f.putpixel(xy, (255, 255, 255, 1))
    return f


def save(path, fr, q, ms=FRAME_MS):
    fr = [corners(f) for f in fr]
    fr[0].save(path, save_all=True, append_images=fr[1:], duration=ms, loop=0,
               quality=q, method=6, alpha_quality=80, minimize_size=True)
    return os.path.getsize(path)


def main():
    root = os.path.normpath(os.path.join(os.path.dirname(__file__), "..", ".."))
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", default="D:/Game/RimModManager_Upgrade/crayon-rig/characters/archo/build/clips-notalk")
    ap.add_argument("--out", default=os.path.join(root, "internal", "surface", "gui", "assets", "archo", "archo-s"))
    a = ap.parse_args()
    meta = json.load(open(os.path.join(a.src, "clips.json")))
    if meta["phases"] != 36 or meta["k"] != 12:
        sys.exit(f"clips.json: phases {meta['phases']}, k {meta['k']}; js/archo.js expects 36 and 12")
    names = sorted(meta["expressions"])
    shutil.rmtree(a.out, ignore_errors=True)
    for d in ("stills", "anim"):
        os.makedirs(os.path.join(a.out, d))
    total = trans = 0
    for n in names:
        fr = frames(a.src, n)
        clear = top_clear(fr)
        if clear < 2:
            print(f"warning: {n} touches the top edge (bake with crayon-rig --top)")
        still = os.path.join(a.out, "stills", f"{n}@360.webp")
        framed(fr[0], STILL_W).save(still, quality=STILL_Q, method=6, alpha_quality=85)
        loop = save(os.path.join(a.out, "anim", f"{n}.webp"), [framed(f, ANIM_W) for f in fr], ANIM_Q)
        t = sum(save(os.path.join(a.out, "anim", f"{n}-{s}.webp"), [framed(f, ANIM_W) for f in frames(a.src, f"{n}-{s}")], TRANS_Q)
                for s in ("in", "out"))
        size = os.path.getsize(still) + loop + t
        total += size
        trans += t
        print(f"{n:10} still {os.path.getsize(still) // 1024} KB, loop {len(fr)} x {FRAME_MS} ms {loop // 1024} KB, "
              f"in + out {t // 1024} KB, {clear} px clear above")
    idle = [framed(f, ANIM_W) for f in frames(a.src, "idle")]
    framed(frames(a.src, "idle")[0], STILL_W).save(os.path.join(a.out, "stills", "idle@360.webp"), quality=STILL_Q, method=6, alpha_quality=85)
    kept, ms = held(idle)
    s = save(os.path.join(a.out, "anim", "idle.webp"), kept, ANIM_Q, ms)
    b = save(os.path.join(a.out, "anim", "bridge.webp"), [framed(f, ANIM_W) for f in frames(a.src, "bridge")], TRANS_Q)
    total += s + b + os.path.getsize(os.path.join(a.out, "stills", "idle@360.webp"))
    print(f"idle       {len(kept)} of {len(idle)} frames ({len(kept) * 1000 / sum(ms):.1f} per s), {s // 1024} KB; bridge {b // 1024} KB")
    print(f"{len(names)} expressions, {total / 1e6:.2f} MB (transitions {(trans + b) / 1e6:.2f} MB)")


if __name__ == "__main__":
    main()
