// Progress and field errors for agents (06 §11 A11, A12): attributes on what is already shown,
// never a second, hidden copy of the text.
let feedbackID = 0;
const newID = (prefix) => `${prefix}-${++feedbackID}`;

// progress makes node a progressbar named by visible text; no value means indeterminate,
// never a false 0%.
export function progress(node, label, value, max = 100, valueText = "") {
  node.setAttribute("role", "progressbar");
  const labels = Array.isArray(label) ? label : [label];
  for (const text of labels) text.id ||= newID("progress-label");
  node.setAttribute("aria-labelledby", labels.map(text => text.id).join(" "));
  if (Number.isFinite(value) && max > 0) {
    node.setAttribute("aria-valuemin", "0");
    node.setAttribute("aria-valuemax", String(max));
    node.setAttribute("aria-valuenow", String(Math.max(0, Math.min(max, value))));
  } else {
    for (const attr of ["aria-valuemin", "aria-valuemax", "aria-valuenow"]) node.removeAttribute(attr);
  }
  if (valueText) node.setAttribute("aria-valuetext", valueText);
  else node.removeAttribute("aria-valuetext");
  return node;
}

// linkError ties field to the node that says why it is refused (null unties it); other
// descriptions of the field stay.
const linked = new WeakMap();
export function linkError(field, node) {
  const old = linked.get(field);
  if (old && old !== node) describe(field, old.id, false);
  if (node) {
    node.id ||= newID("field-error");
    field.setAttribute("aria-invalid", "true");
    describe(field, node.id, true);
    linked.set(field, node);
  } else {
    field.removeAttribute("aria-invalid");
    linked.delete(field);
  }
}

// fieldError follows an error node that is shown and emptied in place; invalid says whether
// its text is about this field (two fields can share one error line).
export function fieldError(field, error, invalid = () => !!error.textContent.trim()) {
  const sync = () => linkError(field, invalid() ? error : null);
  sync();
  new MutationObserver(sync).observe(error, { childList: true, characterData: true, subtree: true, attributes: true, attributeFilter: ["data-field-error"] });
  return sync;
}

// fieldStatusError ties a field to a refusal said on the status line; the next message or
// the next edit of the field unties it.
let statusField = null;
export function clearStatusError() {
  if (statusField) linkError(statusField, null);
  statusField = null;
}
export function fieldStatusError(field, status) {
  clearStatusError();
  statusField = field;
  linkError(field, status);
  field.addEventListener("input", () => { if (statusField === field) clearStatusError(); }, { once: true });
}

function describe(field, id, on) {
  const ids = new Set((field.getAttribute("aria-describedby") || "").split(/\s+/).filter(Boolean));
  if (on) ids.add(id); else ids.delete(id);
  if (ids.size) field.setAttribute("aria-describedby", [...ids].join(" "));
  else field.removeAttribute("aria-describedby");
}
