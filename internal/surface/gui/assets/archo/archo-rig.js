// Archo rig: moves mesh vertices for a pose. Pure functions, no DOM: the WebGL puppet and the
// offline bake both call deform().
//
// Pose (from archo-motion.js), angles in degrees, lengths in model px:
//   head  {yaw, pitch, roll, tx, ty}      2.5D turn on a sphere, then roll about the neck
//   body  {roll, tx, ty, breath, shL, shR} torso lean about the hips, breathing, shoulder raise
//   hair  {bx, by, wave, phase}            how far hair tips swing, plus a travelling wave
//   acc   {bow_L, bow_R, earring_L, earring_R}
//
// Every head-attached layer goes through the same functions, and hair bending is a field over
// position (not per layer), so front and back hair stay joined wherever they meet.

const D2R = Math.PI / 180;

// How each picture is attached. depth: parallax on a head turn (front +, behind -).
export const ATTACH = {
  hair_back:  { to: 'head', depth: -0.35, hair: true },
  body:       { to: 'body' },
  arm_L:      { to: 'arm', side: 'L' },
  arm_R:      { to: 'arm', side: 'R' },
  face:       { to: 'head', depth: 0 },
  hair_front: { to: 'head', depth: 0.16, hair: true },
  bow_L:      { to: 'head', depth: 0.22, pivot: 'bow_L', acc: 'bow_L' },
  bow_R:      { to: 'head', depth: 0.1, pivot: 'bow_R', acc: 'bow_R' },
  earring_L:  { to: 'head', depth: 0.05, pivot: 'earring_L', acc: 'earring_L' },
  earring_R:  { to: 'head', depth: 0.05, pivot: 'earring_R', acc: 'earring_R' },
  patch:      { to: 'head', depth: 0.02 },
  gesture:    { to: 'gesture', depth: 0.1 },
};

const smooth = (a, b, x) => { const t = Math.min(1, Math.max(0, (x - a) / (b - a))); return t * t * (3 - 2 * t); };

export function makeRig(model) {
  const g = model.geom, H = g.head;
  const C = model.canvas[0];

  // Hair stiffness: 0 at the crown and over the forehead, 1 at the tips (low and to the sides).
  const hairU = (x, y) => {
    const lat = Math.abs(x - H.cx) / H.rx;
    const low = (y - (H.cy - H.ry * 0.35)) / (H.ry * 1.25);
    const u = Math.max(0, Math.min(1, 0.75 * lat * lat + 0.65 * Math.max(0, low)));
    return u * Math.sqrt(u);
  };

  function hairDisp(x, y, hair) {
    const u = hairU(x, y);
    if (u <= 0) return [0, 0];
    const L = H.rx * 0.16;
    const wave = hair.wave * Math.sin(hair.phase - (y - H.cy) / (H.ry * 0.45));
    // tips swing sideways; sideways locks also lift a little when swung outward
    const dx = (hair.bx + wave) * u * L;
    const dy = hair.by * u * L * 0.6 + Math.abs(hair.bx) * u * L * 0.08;
    return [dx, dy];
  }

  // 2.5D head turn: points on the front of the head (a sphere) move more than the rim.
  function sphere(x, y, depth, head) {
    const nx = (x - H.cx) / H.rx, ny = (y - H.cy) / H.ry;
    const z = Math.sqrt(Math.max(0, 1 - nx * nx - ny * ny));
    const zz = z * (0.62 + depth);
    const ya = head.yaw * D2R, pa = head.pitch * D2R;
    let X = H.cx + (x - H.cx) * Math.cos(ya) + zz * H.rx * Math.sin(ya) * 0.55;
    let Y = H.cy + (y - H.cy) * Math.cos(pa) + zz * H.ry * Math.sin(pa) * 0.55;
    // the head drifts a little toward where it turns
    X += Math.sin(ya) * H.rx * 0.08; Y += Math.sin(pa) * H.ry * 0.06;
    return [X, Y];
  }

  const rot = (x, y, px, py, deg) => {
    const a = deg * D2R, c = Math.cos(a), s = Math.sin(a);
    return [px + (x - px) * c - (y - py) * s, py + (x - px) * s + (y - py) * c];
  };

  // Body-local: breathing (upper body grows from the hips) and shoulder raise.
  function bodyLocal(x, y, body) {
    const hipY = g.hip[1], k = 0.011 * body.breath;
    let X = g.hip[0] + (x - g.hip[0]) * (1 + k * 0.3);
    let Y = hipY - (hipY - y) * (1 + k);
    const sig = C * 0.11;
    for (const [sh, p] of [[body.shL, g.shoulder_L], [body.shR, g.shoulder_R]]) {
      if (!sh) continue;
      const d2 = (x - p[0]) ** 2 + (y - p[1]) ** 2;
      Y -= sh * Math.exp(-d2 / (2 * sig * sig));
    }
    return [X, Y];
  }
  const bodyWorld = (x, y, body) => {
    let [X, Y] = rot(x, y, g.hip[0], g.hip[1], body.roll);
    return [X + body.tx, Y + body.ty];
  };
  const bodyFull = (x, y, pose) => { const [a, b] = bodyLocal(x, y, pose.body); return bodyWorld(a, b, pose.body); };

  // Where the neck pivot ended up after breathing and the shoulders: the head rides on it.
  function headFull(x, y, depth, pose) {
    const [sx, sy] = sphere(x, y, depth, pose.head);
    let [X, Y] = rot(sx, sy, g.neck[0], g.neck[1], pose.head.roll);
    const [nx, ny] = bodyLocal(g.neck[0], g.neck[1], pose.body);
    X += pose.head.tx + (nx - g.neck[0]); Y += pose.head.ty + (ny - g.neck[1]);
    return bodyWorld(X, Y, pose.body);
  }

  // Arms hang from the shoulder: they follow it rigidly.
  function armFull(x, y, side, pose) {
    const p = side === 'L' ? g.shoulder_L : g.shoulder_R;
    const [sx, sy] = bodyLocal(p[0], p[1], pose.body);
    return bodyWorld(x + (sx - p[0]), y + (sy - p[1]), pose.body);
  }

  // Head weight for a gesture arm: hands at the face move with the head, elbows with the body.
  const headWeight = (x, y) => {
    const e = Math.hypot((x - H.cx) / H.rx, (y - H.cy) / H.ry);
    return smooth(1.18, 0.78, e);
  };

  /** Deform rest vertices (Float32Array [x0,y0,x1,y1,...]) of one picture into out. */
  function deform(kind, rest, out, pose, extra = {}) {
    const A = ATTACH[kind] || ATTACH.patch;
    const n = rest.length;
    if (A.to === 'body') {
      const [n0, n1] = g.neck_x, chin = g.chin_y, col = g.collar_y;
      for (let i = 0; i < n; i += 2) {
        const x = rest[i], y = rest[i + 1];
        let [X, Y] = bodyFull(x, y, pose);
        // the neck stretches between body and head
        if (y < col && x > n0 - 20 && x < n1 + 20) {
          const w = smooth(col, chin, y) * smooth(n0 - 20, n0 + 10, x) * smooth(n1 + 20, n1 - 10, x);
          if (w > 0) { const [hx, hy] = headFull(x, y, 0, pose); X += (hx - X) * w; Y += (hy - Y) * w; }
        }
        out[i] = X; out[i + 1] = Y;
      }
      return;
    }
    if (A.to === 'arm') {
      for (let i = 0; i < n; i += 2) { const [X, Y] = armFull(rest[i], rest[i + 1], A.side, pose); out[i] = X; out[i + 1] = Y; }
      return;
    }
    if (A.to === 'gesture') {
      const side = extra.side || 'L';
      const enter = extra.enter ?? 0;   // swing in from the shoulder (degrees left to go)
      for (let i = 0; i < n; i += 2) {
        let x = rest[i], y = rest[i + 1];
        const sp = x < g.neck[0] ? g.shoulder_L : g.shoulder_R;
        if (enter) [x, y] = rot(x, y, sp[0], sp[1], (x < g.neck[0] ? -1 : 1) * enter);
        const w = enter ? 0 : headWeight(rest[i], rest[i + 1]);
        let [X, Y] = armFull(x, y, x < g.neck[0] ? 'L' : 'R', pose);
        if (w > 0) { const [hx, hy] = headFull(x, y, A.depth, pose); X += (hx - X) * w; Y += (hy - Y) * w; }
        out[i] = X; out[i + 1] = Y;
      }
      return;
    }
    // head-attached
    const piv = A.pivot ? g[A.pivot] : null, ang = A.acc ? pose.acc[A.acc] || 0 : 0;
    for (let i = 0; i < n; i += 2) {
      let x = rest[i], y = rest[i + 1];
      if (piv && ang) [x, y] = rot(x, y, piv[0], piv[1], ang);
      if (A.hair) { const [dx, dy] = hairDisp(x, y, pose.hair); x += dx; y += dy; }
      else if (piv) {  // accessories ride on the hair they sit in
        const [dx, dy] = hairDisp(piv[0], piv[1], pose.hair); x += dx; y += dy;
      }
      const [X, Y] = headFull(x, y, A.depth, pose);
      out[i] = X; out[i + 1] = Y;
    }
  }

  /** Where a point fixed to the head lands (for effect stickers). */
  const headPoint = (x, y, pose) => headFull(x, y, 0.3, pose);

  return { deform, headPoint };
}

/** Grid mesh over a w*h box at (x,y). Returns rest positions, uvs and triangle indices. */
export function gridMesh(x, y, w, h, cell) {
  const nx = Math.max(1, Math.ceil(w / cell)), ny = Math.max(1, Math.ceil(h / cell));
  const rest = new Float32Array((nx + 1) * (ny + 1) * 2), uv = new Float32Array((nx + 1) * (ny + 1) * 2);
  let k = 0;
  for (let j = 0; j <= ny; j++) for (let i = 0; i <= nx; i++) {
    rest[k] = x + w * i / nx; rest[k + 1] = y + h * j / ny;
    uv[k] = i / nx; uv[k + 1] = j / ny; k += 2;
  }
  const idx = new Uint16Array(nx * ny * 6); k = 0;
  for (let j = 0; j < ny; j++) for (let i = 0; i < nx; i++) {
    const a = j * (nx + 1) + i, b = a + 1, c = a + nx + 1, d = c + 1;
    idx.set([a, b, c, b, d, c], k); k += 6;
  }
  return { rest, uv, idx };
}
