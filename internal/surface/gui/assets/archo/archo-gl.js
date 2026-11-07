// Archo puppet (v2): WebGL2 mesh renderer for archo-rig.js + archo-motion.js.
//
//   const p = await ArchoPuppet.create(el, { base: 'archo/', size: 240, view: 'bust' });
//   p.setState('thinking'); p.setExpression('zany'); p.setTalking(true); p.play('nod'); p.destroy();
//
// One <canvas>, redrawn at up to 30 fps only while visible; a still pose under
// prefers-reduced-motion. Textures are released on destroy() and on context loss.

import { ArchoMotion, EXPRESSIONS, STATES } from './archo-motion.js?v=2';
import { makeRig, gridMesh } from './archo-rig.js?v=2';
export { EXPRESSIONS, STATES };

const VS = `#version 300 es
in vec2 pos; in vec2 uv; uniform vec4 view; out vec2 v;
void main(){ v = uv; vec2 p = (pos - view.xy) / view.zw; gl_Position = vec4(p.x * 2.0 - 1.0, 1.0 - p.y * 2.0, 0, 1); }`;
const FS = `#version 300 es
precision mediump float; in vec2 v; uniform sampler2D tex; uniform float alpha; uniform vec3 clip; out vec4 o;
// clip = (box y, box h, split y): when box h > 0, keep only what lies above the split (the eyes)
void main(){ float k = 1.0; if (clip.y > 0.0) { float y = clip.x + v.y * clip.y; k = 1.0 - smoothstep(clip.z - 5.0, clip.z + 5.0, y); }
  o = texture(tex, v) * alpha * k; }`;

const ORDER = ['hair_back', 'body', 'arm_L', 'arm_R', 'face', '#patches', 'hair_front', 'bow_L', 'bow_R', 'earring_L', 'earring_R', '#gestures', '#fx'];

export class ArchoPuppet {
  static async create(el, opts = {}) {
    const base = (opts.base || 'archo/').replace(/\/?$/, '/');
    const model = opts.model || await (await fetch(base + 'archo.json')).json();
    const p = new ArchoPuppet(el, model, { ...opts, base });
    await p.ready;
    return p;
  }

  constructor(el, model, { base, size = 240, view = 'bust', seed, fps = 30, state = 'idle', manual = false, dpr, mesh } = {}) {
    this.el = el; this.m = model; this.base = base; this.fps = fps; this.manual = manual;
    const mq = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)');
    this.motion = new ArchoMotion(model, { seed, reducedMotion: !manual && !!mq?.matches });
    this.rig = makeRig(model);
    this.view = typeof view === 'string' ? model.views[view] : view;
    const ratio = dpr || Math.min(2, globalThis.devicePixelRatio || 1);
    const w = size, h = Math.round(size * this.view[3] / this.view[2]);
    const cv = document.createElement('canvas');
    cv.width = Math.round(w * ratio); cv.height = Math.round(h * ratio);
    cv.style.cssText = `width:${w}px;height:${h}px;display:block`;
    cv.setAttribute('role', 'img'); cv.setAttribute('aria-label', 'Archo');
    el.append(cv); this.cv = cv;
    const gl = cv.getContext('webgl2', { premultipliedAlpha: true, alpha: true, antialias: true, preserveDrawingBuffer: manual, failIfMajorPerformanceCaveat: !manual });
    if (!gl) throw new Error('WebGL2 unavailable');
    this.gl = gl;
    cv.addEventListener('webglcontextlost', e => { e.preventDefault(); this.lost = true; this.sync(); });
    this.prog = program(gl, VS, FS);
    this.loc = { pos: gl.getAttribLocation(this.prog, 'pos'), uv: gl.getAttribLocation(this.prog, 'uv'),
                 view: gl.getUniformLocation(this.prog, 'view'), alpha: gl.getUniformLocation(this.prog, 'alpha'), clip: gl.getUniformLocation(this.prog, 'clip') };
    this.items = {}; this.patchItems = {}; this.gestureItems = {}; this.fxItems = {};
    const loads = [];
    // mesh: grid cell scale. Small figures need fewer vertices (2 = a quarter of them).
    const coarse = mesh || (size <= 200 ? 2 : 1);
    const add = (bucket, key, src, box, kind, cell) => {
      const mesh = gridMesh(box.x, box.y, box.w, box.h, cell * coarse);
      const it = { kind, box, ...mesh, out: new Float32Array(mesh.rest.length) };
      it.vao = gl.createVertexArray(); gl.bindVertexArray(it.vao);
      it.pbuf = gl.createBuffer(); gl.bindBuffer(gl.ARRAY_BUFFER, it.pbuf); gl.bufferData(gl.ARRAY_BUFFER, mesh.rest, gl.DYNAMIC_DRAW);
      gl.enableVertexAttribArray(this.loc.pos); gl.vertexAttribPointer(this.loc.pos, 2, gl.FLOAT, false, 0, 0);
      const ub = gl.createBuffer(); gl.bindBuffer(gl.ARRAY_BUFFER, ub); gl.bufferData(gl.ARRAY_BUFFER, mesh.uv, gl.STATIC_DRAW);
      gl.enableVertexAttribArray(this.loc.uv); gl.vertexAttribPointer(this.loc.uv, 2, gl.FLOAT, false, 0, 0);
      const ib = gl.createBuffer(); gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, ib); gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, mesh.idx, gl.STATIC_DRAW);
      it.bufs = [it.pbuf, ub, ib];
      gl.bindVertexArray(null);
      it.url = base + src;
      loads.push(loadTexture(gl, it.url).then(tex => { it.tex = tex; }));
      bucket[key] = it; return it;
    };
    for (const l of model.layers) add(this.items, l.id, `layers/${l.id}.webp`, l, l.id, l.id === 'body' ? 18 : 12);
    for (const [k, b] of Object.entries(model.patches)) add(this.patchItems, k, `patches/${k}.webp`, b, 'patch', 12);
    for (const [k, b] of Object.entries(model.gestures)) add(this.gestureItems, k, `gestures/${k}.webp`, b, 'gesture', 10);
    for (const [k, f] of Object.entries(model.fx)) add(this.fxItems, k, `fx/${k}.webp`, { x: -f.w / 2, y: -f.h / 2, w: f.w, h: f.h }, 'fx', 1000);
    this.ready = Promise.all(loads).then(() => { this.motion.setState(state); this.render(this.motion.step(0)); });

    this.running = false; this.visible = true; this.last = 0;
    this.tick = this.tick.bind(this);
    if (!manual) {
      this.onVis = () => this.sync();
      document.addEventListener('visibilitychange', this.onVis);
      if ('IntersectionObserver' in globalThis) {
        this.io = new IntersectionObserver(es => { this.visible = es[es.length - 1].isIntersecting; this.sync(); });
        this.io.observe(el);
      }
      mq?.addEventListener?.('change', e => { this.motion.reduced = e.matches; this.render(this.motion.step(0)); this.sync(); });
      this.ready.then(() => this.sync());
    }
  }

  /** Move the canvas into another element (one figure shared by every wait). */
  mount(el) {
    this.el = el; el.append(this.cv);
    if (this.io) { this.io.disconnect(); this.io.observe(el); }
    this.sync();
  }
  /** Take the canvas out of the page; drawing stops until mount(). */
  unmount() { this.cv.remove(); this.io?.disconnect(); this.visible = false; this.sync(); }

  setState(s) { this.motion.setState(s); this.kick(); }
  setExpression(e) { this.motion.setExpression(e); this.kick(); }
  setTalking(on) { this.motion.setTalking(on); this.kick(); }
  play(a) { this.motion.play(a); this.kick(); }
  get state() { return this.motion.state; }
  get expression() { return this.motion.expr; }

  kick() { if (this.motion.reduced) this.render(this.motion.step(0)); this.sync(); }

  sync() {
    if (this.manual) return;
    const want = !this.motion.reduced && this.visible && !document.hidden && !this.dead && !this.lost;
    if (want && !this.running) { this.running = true; this.last = 0; this.raf = requestAnimationFrame(this.tick); }
    if (!want && this.running) { this.running = false; cancelAnimationFrame(this.raf); }
  }

  tick(now) {
    if (!this.running) return;
    this.raf = requestAnimationFrame(this.tick);
    if (!this.last) { this.last = now; return; }
    const dt = (now - this.last) / 1000;
    if (dt < 1 / this.fps - 0.004) return;
    this.last = now;
    this.render(this.motion.step(dt));
  }

  /** Advance by dt seconds and draw (manual mode, for baking). */
  advance(dt) { const p = this.motion.step(dt); this.render(p); return p; }

  render(pose) {
    const gl = this.gl;
    if (this.lost || this.dead) return;
    gl.viewport(0, 0, this.cv.width, this.cv.height);
    gl.clearColor(0, 0, 0, 0); gl.clear(gl.COLOR_BUFFER_BIT);
    gl.enable(gl.BLEND); gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    gl.useProgram(this.prog);
    gl.uniform4f(this.loc.view, ...this.view);
    const draw = (it, kind, alpha, extra) => {
      if (!it || !it.tex || alpha <= 0.003) return;
      this.rig.deform(kind, it.rest, it.out, pose, extra);
      gl.bindVertexArray(it.vao);
      gl.bindBuffer(gl.ARRAY_BUFFER, it.pbuf); gl.bufferSubData(gl.ARRAY_BUFFER, 0, it.out);
      gl.bindTexture(gl.TEXTURE_2D, it.tex);
      gl.uniform1f(this.loc.alpha, alpha);
      gl.uniform3f(this.loc.clip, it.box.y, extra?.eyes ? it.box.h : 0, this.m.geom.split_y);
      gl.drawElements(gl.TRIANGLES, it.idx.length, gl.UNSIGNED_SHORT, 0);
    };
    for (const k of ORDER) {
      if (k === '#patches') { for (const [n, a] of Object.entries(pose.show)) { const [name, part] = n.split('#'); draw(this.patchItems[name], 'patch', a, { eyes: part === 'eyes' }); } continue; }
      if (k === '#gestures') { for (const g of pose.gestures) draw(this.gestureItems[g.name], 'gesture', g.a, { enter: g.enter }); continue; }
      if (k === '#fx') { if (pose.fx) this.drawFx(pose); continue; }
      const a = k === 'arm_L' ? pose.arms.L : k === 'arm_R' ? pose.arms.R : 1;
      draw(this.items[k], k, a);
    }
    gl.bindVertexArray(null);
  }

  drawFx(pose) {
    const f = pose.fx, it = this.fxItems[f.name]; if (!it || !it.tex) return;
    const [cx, cy] = this.rig.headPoint(f.x, f.y, pose);
    const a = f.r * Math.PI / 180, c = Math.cos(a) * f.s, s = Math.sin(a) * f.s;
    for (let i = 0; i < it.rest.length; i += 2) {
      const x = it.rest[i], y = it.rest[i + 1];
      it.out[i] = cx + x * c - y * s; it.out[i + 1] = cy + x * s + y * c;
    }
    const gl = this.gl;
    gl.bindVertexArray(it.vao);
    gl.bindBuffer(gl.ARRAY_BUFFER, it.pbuf); gl.bufferSubData(gl.ARRAY_BUFFER, 0, it.out);
    gl.bindTexture(gl.TEXTURE_2D, it.tex); gl.uniform1f(this.loc.alpha, f.o); gl.uniform3f(this.loc.clip, 0, 0, 0);
    gl.drawElements(gl.TRIANGLES, it.idx.length, gl.UNSIGNED_SHORT, 0);
  }

  destroy() {
    this.dead = true; this.running = false; cancelAnimationFrame(this.raf);
    if (this.onVis) document.removeEventListener('visibilitychange', this.onVis);
    this.io?.disconnect();
    const gl = this.gl;
    for (const b of [this.items, this.patchItems, this.gestureItems, this.fxItems]) for (const it of Object.values(b)) {
      gl.deleteTexture(it.tex); gl.deleteVertexArray(it.vao); it.bufs.forEach(x => gl.deleteBuffer(x)); releaseImage(it.url);
    }
    gl.getExtension('WEBGL_lose_context')?.loseContext();
    this.cv.remove();
  }
}

function program(gl, vs, fs) {
  const p = gl.createProgram();
  for (const [type, src] of [[gl.VERTEX_SHADER, vs], [gl.FRAGMENT_SHADER, fs]]) {
    const s = gl.createShader(type); gl.shaderSource(s, src); gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(s));
    gl.attachShader(p, s);
  }
  gl.linkProgram(p);
  if (!gl.getProgramParameter(p, gl.LINK_STATUS)) throw new Error(gl.getProgramInfoLog(p));
  return p;
}

// Decoded images are shared by every puppet on the page and dropped when the last one goes.
const IMAGES = new Map();   // url -> {p: Promise<Image>, n: users}
function getImage(url) {
  let e = IMAGES.get(url);
  if (!e) {
    const img = new Image(); img.decoding = 'async'; img.src = url;
    e = { p: img.decode().then(() => img), n: 0 }; IMAGES.set(url, e);
  }
  e.n++; return e.p;
}
function releaseImage(url) {
  const e = IMAGES.get(url); if (e && --e.n <= 0) IMAGES.delete(url);
}

async function loadTexture(gl, url) {
  const img = await getImage(url);
  const t = gl.createTexture();
  gl.bindTexture(gl.TEXTURE_2D, t);
  gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, true);
  gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, img);
  gl.generateMipmap(gl.TEXTURE_2D);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  return t;
}
