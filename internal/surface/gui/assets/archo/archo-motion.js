// Archo motion (v2): time + state -> pose for archo-rig.js. No DOM; shared by the puppet and the bake.
//
// pose = { head:{yaw,pitch,roll,tx,ty}, body:{roll,tx,ty,breath,shL,shR}, hair:{bx,by,wave,phase},
//          acc:{bow_L,bow_R,earring_L,earring_R}, show:{patch:opacity}, gestures:[{name,a,enter}],
//          arms:{L,R} (opacity of the hanging arms), fx:{name,x,y,s,r,o}|null }   (x,y: model px, head space)

const TAU = Math.PI * 2;

// Expressions. pose: where the head and shoulders settle (deg / px). style: how they keep moving.
// patch: the face; eyes: keep its eye half while talking; gesture: hand pose.
export const EXPRESSIONS = {
  neutral:   { emoji: '🙂' },
  smile:     { emoji: '😊', patch: 'love', eyes: true, blink: false, pose: { roll: -4 } },
  laugh:     { emoji: '😆', patch: 'laugh', blink: false, talk: false, fx: 'sparkle', pose: { roll: -6, pitch: -5 }, style: 'bounce' },
  wink:      { emoji: '😜', patch: 'wink', eyes: true, blink: false, fx: 'heart', pose: { roll: 10, yaw: 7 } },
  think:     { emoji: '🧐', patch: 'think', eyes: true, fx: 'dots', pose: { roll: 6, pitch: -5, yaw: -6 } },
  ponder:    { emoji: '🤔', patch: 'ponder', eyes: true, gesture: 'ponder', fx: 'question', pose: { roll: 8, pitch: -6, yaw: -5, shL: 5 } },
  surprised: { emoji: '😳', patch: 'surprised', eyes: true, fx: 'exclaim', pose: { pitch: -8, shL: 7, shR: 7 }, style: 'tremble', amp: 0.5 },
  scream:    { emoji: '😱', patch: 'scream', gesture: 'scream', blink: false, talk: false, fx: 'exclaim', pose: { pitch: -5, shL: 14, shR: 14 }, style: 'tremble', amp: 1.4 },
  cry:       { emoji: '😭', patch: 'cry', blink: false, talk: false, pose: { pitch: 9, shL: 8, shR: 8 }, style: 'sob' },
  plead:     { emoji: '🥺', patch: 'plead', eyes: true, gesture: 'plead', pose: { roll: 9, pitch: 6, shL: 9, shR: 9 }, style: 'tremble', amp: 0.25 },
  smug:      { emoji: '😁', patch: 'smug', eyes: true, fx: 'sparkle', pose: { roll: -5, pitch: -7 } },
  smirk:     { emoji: '😏', patch: 'smirk', eyes: true, pose: { roll: -8, yaw: 10 } },
  salute:    { emoji: '🫡', patch: 'salute', eyes: true, gesture: 'salute', pose: { roll: -3, pitch: -3, shL: 6 } },
  yum:       { emoji: '😋', patch: 'yum', blink: false, talk: false, fx: 'note', pose: { roll: -9 }, style: 'sway' },
  zany:      { emoji: '🤪', patch: 'zany', eyes: true, gesture: 'zany', blink: false, pose: { roll: 13, yaw: 5, shR: 6 }, style: 'wobble' },
  dizzy:     { emoji: '😵', patch: 'dizzy', blink: false, talk: false, pose: { roll: 7, pitch: 4 }, style: 'wobble' },
  spiral:    { emoji: '😵‍💫', patch: 'spiral', blink: false, talk: false, fx: 'sparkle', fxOrbit: true, style: 'circle' },
  drool:     { emoji: '🤤', patch: 'drool', blink: false, talk: false, fx: 'heart', pose: { roll: 11, pitch: -8 }, style: 'sway', amp: 0.6 },
  love:      { emoji: '🥰', patch: 'love', eyes: true, blink: false, fx: 'heart', pose: { shL: 6, shR: 6 }, style: 'sway' },
  sleepy:    { emoji: '😪', patch: 'sleepy', eyes: true, slowBlink: true, fx: 'zzz', pose: { roll: 12, pitch: 5 }, style: 'droop' },
};

// Agent states: an expression plus breathing / sway, optionally an action and a timed return.
export const STATES = {
  idle:     { expression: 'neutral' },
  loading:  { expression: 'neutral', breath: 2.2, sway: 1.2 },
  thinking: { expression: 'ponder', sway: 0.6 },
  working:  { expression: 'think', sway: 0.6 },
  talking:  { expression: 'neutral', talking: true },
  success:  { expression: 'smug', action: 'hop', then: 'idle', hold: 2.6 },
  done:     { expression: 'salute', action: 'nod', then: 'idle', hold: 2.4 },
  happy:    { expression: 'laugh', action: 'hop', then: 'idle', hold: 2.4 },
  playful:  { expression: 'zany', then: 'idle', hold: 2.6 },
  asking:   { expression: 'plead' },
  thanks:   { expression: 'love', then: 'idle', hold: 2.6 },
  warning:  { expression: 'surprised', action: 'jolt', then: 'idle', hold: 2.4 },
  error:    { expression: 'cry', action: 'shake' },
  panic:    { expression: 'scream', action: 'jolt' },
  confused: { expression: 'spiral' },
  sleeping: { expression: 'sleepy', breath: 4.6, sway: 0.5 },
};

function rng(seed) {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// Damped spring; returns its position.
class Spring {
  constructor(k, c, x = 0) { this.k = k; this.c = c; this.x = x; this.v = 0; }
  step(dt, target, kick = 0) {
    // two half steps keep it stable at 12 fps bakes
    for (let i = 0; i < 2; i++) {
      const h = dt / 2;
      this.v += (-this.k * (this.x - target) - this.c * this.v + kick) * h;
      this.x += this.v * h;
    }
    return this.x;
  }
}

const easeOutBack = u => { const c = 1.9; return 1 + (c + 1) * Math.pow(u - 1, 3) + c * Math.pow(u - 1, 2); };

export class ArchoMotion {
  constructor(model, { seed = 7, reducedMotion = false, blinkEvery = 0, blinkAt = 0 } = {}) {
    this.m = model;
    this.H = model.geom.head;
    this.reduced = reducedMotion;
    this.rand = rng(seed);
    this.blinkEvery = blinkEvery; this.blinkAt = blinkAt;
    this.t = 0;
    // pose springs: a little under-damped so the head settles with a small overshoot
    this.ps = { roll: new Spring(70, 11), yaw: new Spring(70, 11), pitch: new Spring(70, 11),
                shL: new Spring(90, 13), shR: new Spring(90, 13) };
    this.hs = { bx: new Spring(38, 4.2), by: new Spring(60, 6), bow_L: new Spring(55, 4.5), bow_R: new Spring(65, 5),
                earring_L: new Spring(42, 2.6), earring_R: new Spring(42, 2.6) };
    this.prev = null;
    this.state = 'idle'; this.expr = 'neutral'; this.talking = false;
    this.action = null; this.revertAt = null;
    this.nextBlink = 1.2 + this.rand() * 2; this.blink = null;
    this.mouth = 'closed'; this.nextMouth = 0;
    this.fx = null;
    this.gest = [];                // [{name, t0, out: t|null}]
    this.bph = 0; this.sph = 0; this.tiltS = 0; this.swayS = 1;
  }

  // ---- control --------------------------------------------------------------------------

  setState(name) {
    const s = STATES[name] || STATES.idle;
    this.state = name in STATES ? name : 'idle';
    this.setExpression(s.expression);
    this.talking = !!s.talking;
    if (s.action) this.play(s.action);
    this.revertAt = s.then && s.hold ? this.t + s.hold : null;
  }

  setExpression(name) {
    if (!EXPRESSIONS[name]) name = 'neutral';
    if (name === this.expr) return;
    const old = EXPRESSIONS[this.expr], e = EXPRESSIONS[name];
    this.expr = name;
    this.fx = e.fx ? { name: e.fx, t0: this.t } : null;
    this.blink = null;
    if (old.gesture !== e.gesture) {
      for (const g of this.gest) if (g.out === null) g.out = this.t;
      if (e.gesture) this.gest.push({ name: e.gesture, t0: this.t, out: null });
    }
  }

  setTalking(on) { this.talking = !!on; }
  play(action) { this.action = { name: action, t0: this.t }; }

  // ---- stepping -----------------------------------------------------------------------

  step(dt) {
    dt = Math.min(Math.max(dt, 0), 0.1);
    this.t += dt;
    if (this.revertAt !== null && this.t >= this.revertAt) {
      const then = STATES[this.state]?.then || 'idle';
      this.revertAt = null;
      this.setState(then);
    }
    return this.pose(dt);
  }

  pose(dt) {
    const t = this.t, H = this.H;
    const st = STATES[this.state] || STATES.idle;
    const e = EXPRESSIONS[this.expr] || EXPRESSIONS.neutral;
    const ep = e.pose || {};
    const pose = {
      head: { yaw: 0, pitch: 0, roll: 0, tx: 0, ty: 0 },
      body: { roll: 0, tx: 0, ty: 0, breath: 0, shL: 0, shR: 0 },
      hair: { bx: 0, by: 0, wave: 0, phase: 0 },
      acc: { bow_L: 0, bow_R: 0, earring_L: 0, earring_R: 0 },
    };
    if (this.reduced) {
      Object.assign(pose.head, { roll: ep.roll || 0, yaw: ep.yaw || 0, pitch: ep.pitch || 0 });
      Object.assign(pose.body, { shL: ep.shL || 0, shR: ep.shR || 0 });
      return { ...pose, show: this.face(0, true), ...this.gestures(true), fx: this.fxPose(true) };
    }

    // breathing and idle sway: phases accumulate, amplitudes ease (no snaps on state change)
    const Tb = st.breath || (e.style === 'bounce' ? 1.5 : e.style === 'sob' ? 1.1 : 3.2);
    const Ts = Tb * 2;
    this.bph = (this.bph + dt / Tb) % 1; this.sph = (this.sph + dt / Ts) % 1;
    const ease = Math.min(1, dt * 4);
    this.swayS += ((st.sway ?? 1) - this.swayS) * ease;
    const ph = this.bph, sp = this.sph;
    const b = ph < 0.42 ? 0.5 - 0.5 * Math.cos(Math.PI * ph / 0.42) : 0.5 + 0.5 * Math.cos(Math.PI * (ph - 0.42) / 0.58);
    pose.body.breath = b;

    // where the head wants to be: expression pose + idle sway + style
    const sw = this.swayS;
    let roll = (ep.roll || 0) + sw * 1.8 * Math.sin(TAU * sp);
    let yaw = (ep.yaw || 0) + sw * 2.5 * Math.sin(TAU * sp + 1.1);
    let pitch = (ep.pitch || 0) + sw * 1.2 * Math.sin(TAU * 2 * sp + 0.4);
    let shL = ep.shL || 0, shR = ep.shR || 0;
    let ty = 0;
    const amp = e.amp ?? 1;
    switch (e.style) {
      case 'bounce': ty -= 5 * Math.abs(Math.sin(TAU * t / 0.42)); shL += 2 * Math.abs(Math.sin(TAU * t / 0.42)); shR = shL; break;
      case 'sob': { const s = Math.max(0, Math.sin(TAU * t / 0.55)); pitch += 3 * s; shL += 4 * s; shR += 4 * s; roll += 1.5 * Math.sin(TAU * t / 1.1); break; }
      case 'tremble': yaw += 1.6 * amp * Math.sin(TAU * t * 9); roll += 0.9 * amp * Math.sin(TAU * t * 7.3); break;
      case 'wobble': roll += 7 * amp * Math.sin(TAU * t / 0.95); yaw += 6 * amp * Math.sin(TAU * t / 0.65 + 0.5); break;
      case 'circle': { const w = TAU * t / 1.25; roll += 8 * Math.sin(w); pitch += 7 * Math.cos(w); yaw += 6 * Math.sin(w); break; }
      case 'sway': roll += 7 * amp * Math.sin(TAU * t / 2.1); yaw += 3 * amp * Math.sin(TAU * t / 2.1 - 0.8); break;
      case 'droop': { const c = (t % 5.5) / 5.5; pitch += c < 0.85 ? 12 * (c / 0.85) ** 2 : 12 * (1 - (c - 0.85) / 0.15); break; }
    }

    // one-shot actions (added on top of the springs' target, so they also settle smoothly)
    let rootTy = 0, act = { yaw: 0, pitch: 0, roll: 0 };
    if (this.action) {
      const u = t - this.action.t0, n = this.action.name;
      if (n === 'hop') {
        const d = 0.48;
        if (u < d) { rootTy = -0.05 * this.m.canvas[0] * Math.sin(Math.PI * u / d); pose.body.breath += 0.8 * Math.sin(Math.PI * u / d); act.pitch = -4 * Math.sin(Math.PI * u / d); }
        else if (u < d + 0.2) { const v = (u - d) / 0.2; pose.body.breath -= 1.2 * Math.sin(Math.PI * v); act.pitch = 5 * Math.sin(Math.PI * v); }
        else this.action = null;
      } else if (n === 'nod') {           // 🙂‍↕️ chin goes down and back up, twice
        const d = 0.34;
        if (u < 2 * d) { const k = u < d ? 1 : 0.6; act.pitch = 14 * k * Math.sin(Math.PI * (u % d) / d); ty += 0.008 * this.m.canvas[0] * k * Math.sin(Math.PI * (u % d) / d); }
        else this.action = null;
      } else if (n === 'shake') {         // 🙂‍↔️ head turns left-right
        const d = 1.1;
        if (u < d) { act.yaw = 18 * Math.sin(TAU * 2.5 * u / d) * (1 - u / d); act.roll = 2 * Math.sin(TAU * 2.5 * u / d) * (1 - u / d); }
        else this.action = null;
      } else if (n === 'jolt') {
        if (u < 0.35) { const s = Math.sin(Math.PI * u / 0.35); rootTy = -0.02 * this.m.canvas[0] * s; act.pitch = -9 * s; shL += 8 * s; shR += 8 * s; }
        else this.action = null;
      } else if (n === 'tilt') {          // a curious head tilt
        if (u < 1.2) act.roll = 13 * Math.sin(Math.PI * Math.min(1, u / 1.2));
        else this.action = null;
      } else this.action = null;
    }

    const P = this.ps;
    // actions drive the head directly (they are already smooth); poses go through springs
    pose.head.roll = P.roll.step(dt, roll) + act.roll;
    pose.head.yaw = P.yaw.step(dt, yaw) + act.yaw;
    pose.head.pitch = P.pitch.step(dt, pitch) + act.pitch;
    pose.head.ty = ty; pose.head.tx = 0;
    pose.body.shL = P.shL.step(dt, shL); pose.body.shR = P.shR.step(dt, shR);
    pose.body.roll = pose.head.roll * 0.12 + 0.4 * Math.sin(TAU * sp - 0.5);
    pose.body.tx = pose.head.yaw * 0.12;
    pose.body.ty = rootTy;

    // secondary motion: hair, bows and earrings lag behind the head
    const hd = pose.head;
    const cur = { r: hd.roll, y: hd.yaw, p: hd.pitch, ty: hd.ty + rootTy };
    if (!this.prev || dt <= 0) this.prev = { ...cur, vr: 0, vy: 0, vp: 0, vt: 0 };
    const q = this.prev, inv = 1 / Math.max(dt, 1e-3);
    const vr = (cur.r - q.r) * inv, vy = (cur.y - q.y) * inv, vp = (cur.p - q.p) * inv, vt = (cur.ty - q.ty) * inv;
    const ar = (vr - q.vr) * inv, ay = (vy - q.vy) * inv, ap = (vp - q.vp) * inv, at = (vt - q.vt) * inv;
    this.prev = { ...cur, vr, vy, vp, vt };
    const side = -(ar * 0.012 + ay * 0.012);
    const up = -(at * 0.004 + ap * 0.006);
    const wind = 0.35 * Math.sin(TAU * 2 * sp) + 0.2 * Math.sin(TAU * 3 * sp + 1);
    const S = this.hs;
    // hair hangs: on a tilt the tips swing back toward vertical
    pose.hair.bx = Math.max(-1.8, Math.min(1.8, S.bx.step(dt, wind - hd.roll * 0.035, side * 30)));
    pose.hair.by = Math.max(-1.5, Math.min(1.5, S.by.step(dt, 0, up * 30)));
    pose.hair.wave = 0.25 + Math.min(0.6, Math.abs(S.bx.v) * 0.08);
    pose.hair.phase = TAU * 2 * sp;
    pose.acc.bow_L = S.bow_L.step(dt, wind * 5, side * 40);
    pose.acc.bow_R = S.bow_R.step(dt, -wind * 4, side * 30);
    const dangle = 3 * Math.sin(TAU * 2 * sp + 0.8);
    pose.acc.earring_L = S.earring_L.step(dt, -hd.roll * 0.9 + dangle, side * 60 + up * 20);
    pose.acc.earring_R = S.earring_R.step(dt, -hd.roll * 0.9 + dangle * 0.9, side * 60 - up * 20);

    return { ...pose, show: this.face(dt, false), ...this.gestures(false), fx: this.fxPose(false) };
  }

  // ---- hands ---------------------------------------------------------------------------

  gestures(still) {
    const t = this.t, list = [];
    let L = 1, R = 1;
    this.gest = this.gest.filter(g => g.out === null || t - g.out < 0.2);
    for (const g of this.gest) {
      const info = this.m.gestures[g.name]; if (!info) continue;
      let a, enter = 0;
      if (still) { if (g.out !== null) continue; a = 1; }
      else if (g.out !== null) { const u = (t - g.out) / 0.16; a = Math.max(0, 1 - u); enter = 26 * u * u; }
      else { const u = Math.min(1, (t - g.t0) / 0.26); a = Math.min(1, u * 3); enter = 30 * (1 - easeOutBack(u)); }
      list.push({ name: g.name, a, enter });
      if (info.hide.includes('arm_L')) L = Math.min(L, 1 - a);
      if (info.hide.includes('arm_R')) R = Math.min(R, 1 - a);
    }
    return { gestures: list, arms: { L, R } };
  }

  // ---- face ----------------------------------------------------------------------------

  face(dt, still) {
    const e = EXPRESSIONS[this.expr] || EXPRESSIONS.neutral;
    const show = {};
    const talking = this.talking && e.talk !== false && !still;
    if (talking) {
      if (e.eyes && e.patch) show[e.patch + '#eyes'] = 1;   // the expression's eyes stay, the mouth talks
      if (this.t >= this.nextMouth) {
        const r = this.rand();
        this.mouth = this.mouth === 'open' ? (r < 0.6 ? 'half' : 'closed')
                   : this.mouth === 'half' ? (r < 0.55 ? 'open' : 'closed')
                   : (r < 0.7 ? 'half' : 'open');
        this.nextMouth = this.t + 0.075 + this.rand() * 0.07;
      }
      if (this.mouth !== 'closed') show['talk_' + this.mouth] = 1;
    } else {
      if (e.patch) show[e.patch] = 1;
      this.mouth = 'closed';
    }
    if (!still && e.blink !== false) {
      const slow = !!e.slowBlink;
      const due = this.blinkEvery
        ? Math.floor((this.t - this.blinkAt) / this.blinkEvery) > Math.floor((this.t - this.blinkAt - dt) / this.blinkEvery)
        : this.t >= this.nextBlink;
      if (!this.blink && due) {
        const fr = slow ? [['blink_half', 0.14], ['blink_closed', 0.5], ['blink_half', 0.18]]
                        : [['blink_half', 0.045], ['blink_closed', 0.075], ['blink_half', 0.05]];
        if (!slow && !this.blinkEvery && this.rand() < 0.12) fr.push(['', 0.12], ['blink_half', 0.04], ['blink_closed', 0.06], ['blink_half', 0.045]);
        this.blink = { t0: this.t, fr };
      }
      if (this.blink) {
        let u = this.t - this.blink.t0, cur = null;
        for (const [n, d] of this.blink.fr) { if (u < d) { cur = n; break; } u -= d; }
        if (cur === null) { this.blink = null; this.nextBlink = this.t + (slow ? 1.6 + this.rand() * 2 : 2.2 + this.rand() * 3.6); }
        else if (cur) show[cur] = 1;
      }
    }
    return show;
  }

  // effect sticker, in head space (the renderer carries it with the head)
  fxPose(still) {
    if (!this.fx) return null;
    const H = this.H, u = still ? 1 : this.t - this.fx.t0, n = this.fx.name;
    const at = { exclaim: [0.92, -0.78], question: [0.92, -0.8], dots: [0.95, -0.82], sparkle: [0.95, -0.35],
                 heart: [0.95, -0.25], note: [0.95, -0.55], zzz: [0.88, -0.85], sweat: [0.7, -0.2] }[n] || [0.9, -0.8];
    let x = H.cx + at[0] * H.rx, y = H.cy + at[1] * H.ry;
    const pop = u < 0.12 ? u / 0.12 * 1.15 : u < 0.22 ? 1.15 - (u - 0.12) / 0.1 * 0.15 : 1;
    let s = still ? 1 : pop, r = 0, o = 1;
    if (!still) {
      if (EXPRESSIONS[this.expr]?.fxOrbit) {   // stars circling the head
        const w = TAU * u / 1.25; x = H.cx + Math.cos(w) * H.rx * 0.95; y = H.cy - H.ry * 0.75 + Math.sin(w) * H.ry * 0.18;
        s = 0.8 + 0.2 * Math.sin(w); r = u * 200;
      } else if (n === 'zzz') { const k = (u % 2.4) / 2.4; y -= k * H.ry * 0.25; x += k * H.rx * 0.08; o = k < 0.15 ? k / 0.15 : k > 0.75 ? (1 - k) / 0.25 : 1; s = 0.8 + 0.25 * k; }
      else if (n === 'dots' || n === 'question') { y += H.ry * 0.025 * Math.sin(TAU * u / 1.2); r = n === 'question' ? 8 * Math.sin(TAU * u / 1.6) : 0; }
      else if (n === 'sparkle') { r = 8 * Math.sin(TAU * u / 1.1); s *= 1 + 0.08 * Math.sin(TAU * u / 0.55); }
      else if (n === 'exclaim') { r = 6 * Math.sin(TAU * u / 0.3) * Math.max(0, 1 - u / 0.9); }
      else if (n === 'heart' || n === 'note') { y -= H.ry * 0.04 * Math.sin(TAU * u / 1.4); s *= 1 + 0.06 * Math.max(0, Math.sin(TAU * u / 0.7)); r = n === 'note' ? 10 * Math.sin(TAU * u / 0.9) : 0; }
    }
    return { name: n, x, y, s, r, o };
  }
}
