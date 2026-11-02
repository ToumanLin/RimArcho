// Links to an installed copy use its Instance ID, including when several copies share a name.
import { el, T } from "./state.js";

export function openLibraryInstance(id) {
  document.dispatchEvent(new CustomEvent("library-instance", { detail: id }));
}

export function libraryLink(id, name) {
  const link = el("button", "btn link word-facts", name);
  link.type = "button";
  link.title = T("show in the mod library");
  link.onclick = () => openLibraryInstance(id);
  return link;
}
