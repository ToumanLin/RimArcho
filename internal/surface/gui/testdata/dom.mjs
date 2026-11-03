import { readFile } from "node:fs/promises";
import { SourceTextModule, SyntheticModule, createContext } from "node:vm";

// Minimal DOM for page behavior tests, independent of a browser installation.
export class Element {
  children = []; dataset = {}; attributes = {}; text = ""; disabled = false;
  style = { setProperty(k, v) { this[k] = v; }, getPropertyValue(k) { return this[k] || ""; } };
  constructor(tag, className = "", text = "") {
    Object.assign(this, { tag, tagName: tag.toUpperCase(), className, text });
    this.classList = {
      add: (...cs) => { this.className = [...new Set([...this.className.split(" "), ...cs])].join(" "); },
      remove: (...cs) => { this.className = this.className.split(" ").filter((c) => !cs.includes(c)).join(" "); },
      contains: (c) => this.className.split(" ").includes(c),
      toggle: (c, on) => { if (on ?? !this.className.split(" ").includes(c)) this.classList.add(c); else this.classList.remove(c); },
    };
  }
  append(...nodes) {
    for (let n of nodes) {
      if (typeof n !== "object") n = new Element("#text", "", String(n));
      n.remove(); n.parent = this; this.children.push(n);
    }
  }
  insertBefore(n, ref) {
    n.remove();
    const i = ref ? this.children.indexOf(ref) : this.children.length;
    n.parent = this; this.children.splice(i, 0, n);
  }
  remove() { if (this.parent) { this.parent.children.splice(this.parent.children.indexOf(this), 1); this.parent = null; } }
  get childNodes() { return this.children; }
  get parentNode() { return this.parent; }
  get nodeType() { return this.tag === "#text" ? 3 : 1; }
  get isConnected() { return !!this.parent; }
  addEventListener(type, fn) { (this.listeners ||= {})[type] ||= []; this.listeners[type].push(fn); }
  dispatchEvent(e) { for (const fn of this.listeners?.[e.type] || []) fn(e); }
  closest(s) { for (let n = this; n; n = n.parent) if (n.matches(s)) return n; return null; }
  contains(o) { for (let n = o; n; n = n.parent) if (n === this) return true; return false; }
  get firstChild() { return this.children[0]; }
  get lastChild() { return this.children.at(-1); }
  set innerHTML(_) { for (const c of [...this.children]) c.remove(); this.text = ""; }
  set textContent(t) { this.innerHTML = ""; this.text = t; }
  get textContent() { return this.text + this.children.map((n) => n.textContent).join(""); }
  setAttribute(k, v) { this.attributes[k] = v; }
  getAttribute(k) { return this.attributes[k] ?? null; }
  removeAttribute(k) { delete this.attributes[k]; }
  focus() {}
  blur() {}
  setSelectionRange() {}
  scrollIntoView() {}
  setPointerCapture(id) { this.captured = id; }
  getBoundingClientRect() {
    const i = this.parent ? this.parent.children.indexOf(this) : 0;
    return { left: i * 100, right: (i + 1) * 100, bottom: 28, width: 100 };
  }
  after(n) { this.parent.insertBefore(n, this.parent.children[this.parent.children.indexOf(this) + 1]); }
  replaceWith(n) { const p = this.parent; p.insertBefore(n, this); this.remove(); }
  replaceChildren(...nodes) { this.innerHTML = ""; this.append(...nodes); }
  isEqualNode(n) {
    return ["tag", "className", "text", "disabled", "hidden"].every((k) => this[k] === n[k]) &&
      JSON.stringify(this.dataset) === JSON.stringify(n.dataset) && JSON.stringify(this.attributes) === JSON.stringify(n.attributes) &&
      this.children.length === n.children.length && this.children.every((child, i) => child.isEqualNode(n.children[i]));
  }
  matches(s) {
    const attr = s.match(/\[data-([\w-]+)(?:="([^"]*)")?\]$/);
    if (attr) {
      const key = attr[1].replace(/-([a-z])/g, (_, c) => c.toUpperCase());
      if (!(key in this.dataset) || (attr[2] !== undefined && this.dataset[key] !== attr[2])) return false;
      s = s.slice(0, attr.index);
    }
    if (!s || s === "*") return true;
    if (s.startsWith("#")) return this.id === s.slice(1);
    if (s.startsWith(".")) return s.slice(1).split(".").every((c) => this.className.split(" ").includes(c));
    return this.tag === s;
  }
  querySelectorAll(s) {
    if (s.includes(",")) return s.split(",").flatMap((p) => this.querySelectorAll(p.trim()));
    const parts = s.split(" "), last = parts.pop();
    const all = this.children.flatMap((n) => [n, ...n.querySelectorAll("*")]);
    return all.filter((n) => {
      if (!n.matches(last)) return false;
      let parent = n.parent;
      for (const p of [...parts].reverse()) {
        while (parent && !parent.matches(p)) parent = parent.parent;
        if (!parent) return false;
        parent = parent.parent;
      }
      return true;
    });
  }
  querySelector(s) { return this.querySelectorAll(s)[0] || null; }
}

Element.TEXT_NODE = 3;
Element.ELEMENT_NODE = 1;

export async function loadModules(entry, deps, globals = {}) {
  deps["/wails/runtime.js"] ||= {};
  const state = deps["./state.js"];
  if (state) { state.esc ||= s => String(s); state.T ||= s => s; state.keyCaps ||= k => (state.el ? state.el("kbd", "", k) : new Element("kbd")); }
  const document = globals.document || {};
  document.createElement ||= state?.el || (tag => new Element(tag));
  globals = { ...globals, document };
  const context = createContext({ Node: Element, ResizeObserver: class { observe() {} unobserve() {} }, MutationObserver: class { observe() {} }, ...globals }), cache = new Map();
  let factFactory;
  if (state?.el && !state.factText) {
    const source = await readFile(new URL("../assets/js/state.js", import.meta.url), "utf8");
    const helper = source.match(/export function factText[\s\S]*?\n}/)[0];
    factFactory = new SourceTextModule('import { pathContent } from "./fields.js";\n' + helper, { context });
    state.factText = (...args) => factFactory.namespace.factText(...args);
  }
  async function get(name) {
    if (cache.has(name)) return cache.get(name);
    let module;
    if (deps[name]) module = new SyntheticModule(Object.keys(deps[name]), function () {
      for (const [key, value] of Object.entries(deps[name])) this.setExport(key, value);
    }, { context });
    else module = new SourceTextModule(await readFile(new URL("../assets/js/" + name.replace(/^\.\//, ""), import.meta.url), "utf8"), { context });
    cache.set(name, module);
    await module.link(get);
    return module;
  }
  if (factFactory) { await factFactory.link(get); await factFactory.evaluate(); }
  const module = await get(entry);
  await module.evaluate();
  return module.namespace;
}

export const event = (key, more = {}) => ({ key, preventDefault() {}, stopPropagation() {}, ...more });
export const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; };
export const flush = () => new Promise(setImmediate);
