const variants = {
  original: ['原版对照', '直接运行现有 home.js、shell.js 和 style.css：跨游戏的最近列表在左，动态在左下，启动在右下。'],
  a: ['A · 底部并排', '保留原版最近列表与背景，把右下角启动区展开为「游戏 → 列表 / 整合包 → 启动游戏」一行。'],
  b: ['B · 上部并排', '同样的三个控件移到页面左上，先选游戏和列表，下面仍是跨游戏的最近列表；动态留在左下。'],
  c: ['C · 分列选择', '左列选择游戏，中列显示该游戏的列表与整合包，右下启动；这一版把原来混排的跨游戏内容分开。'],
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
