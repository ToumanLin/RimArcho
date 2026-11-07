import { readFileSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';
import { test } from 'node:test';
import assert from 'node:assert/strict';

test('editor resizes coalesce redraws outside ResizeObserver delivery',()=>{
  const observers=[],frames=[],draws=[];
  const node={addEventListener(){},closest(){return this;}};
  const context=createContext({
    window:{addEventListener(){}},
    $:selector=>selector.endsWith('.search input')?null:node,el:()=>node,wireBadges(){},draws,
    ResizeObserver:class {constructor(fn){observers.push(fn);}observe(){}},
    requestAnimationFrame:fn=>{frames.push(fn);return frames.length;}
  });
  const source=readFileSync(new URL('../../internal/surface/gui/assets/js/lists.js',import.meta.url),'utf8')
    .replace(/^import .*;\r?$/gm,'').replace(/\bexport /g,'');
  runInContext(source+'\nrenderRows=k=>draws.push(k);wireLists({});',context);
  observers[0]();observers[0]();observers[0]();
  assert.deepEqual(draws,[],'resize delivery must not write layout');
  assert.equal(frames.length,1,'coalesce repeated size notifications');
  frames[0]();assert.deepEqual(draws,[0]);
  observers[0]();observers[1]();observers[2]();
  assert.equal(frames.length,4,'each column can redraw on the next frame');
  for(const frame of frames.slice(1))frame();
  assert.deepEqual(draws,[0,0,1,2]);
});
