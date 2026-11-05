const variants = {
  original: ['Original comparison', 'Runs existing home.js, shell.js and style.css directly: recent lists across games on the left, activity bottom-left, launch bottom-right.'],
  a: ['A · Bottom row', 'Keeps original recent lists and background, expanding bottom-right launch into one row: game → list / pack → launch game.'],
  b: ['B · Top row', 'Moves the same three controls to top-left: choose game and list first, recent lists across games below; activity stays bottom-left.'],
  c: ['C · Columns', 'Choose game left, its lists and packs in the middle, launch bottom-right; separates previously mixed cross-game content.'],
};
const params = new URLSearchParams(location.search);
let design = Object.hasOwn(variants, params.get('design')) ? params.get('design') : 'a';
const background = document.querySelector('#background'), scenario = document.querySelector('#scenario');
background.value = params.get('background') === 'plain' ? 'plain' : 'image';
if ([...scenario.options].some(o => o.value === params.get('scenario'))) scenario.value = params.get('scenario');
function show(replace = false) {
  const query = new URLSearchParams({design, background: background.value, scenario: scenario.value});
  history[replace ? 'replaceState' : 'pushState']({}, '', `?${query}`);
  document.querySelector('#preview').src = `./preview.html?${query}`;
  document.querySelectorAll('[data-design]').forEach(a => {
    if (a.dataset.design === design) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
  });
  document.querySelector('#variant-title').textContent = variants[design][0];
  document.querySelector('#variant-note').textContent = variants[design][1];
  document.title = `RimArcho · ${variants[design][0]}`;
}
document.querySelectorAll('[data-design]').forEach(a => a.onclick = e => {
  if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
  e.preventDefault(); design = a.dataset.design; show();
});
background.onchange = scenario.onchange = () => show();
addEventListener('popstate', () => {
  const next = new URLSearchParams(location.search);
  design = Object.hasOwn(variants, next.get('design')) ? next.get('design') : 'a';
  background.value = next.get('background') === 'plain' ? 'plain' : 'image';
  scenario.value = next.get('scenario') || 'issue';
  show(true);
});
show(true);
