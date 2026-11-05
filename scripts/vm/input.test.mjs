import test from 'node:test';
import assert from 'node:assert/strict';
import { keyOf, comboEvents, dragPath, solveAxis, toScreen, parseAddr } from './input.mjs';

test('key names', () => {
  assert.deepEqual(keyOf('k'), { vk: 0x4b, ext: false });
  assert.deepEqual(keyOf('KeyK'), { vk: 0x4b, ext: false });
  assert.deepEqual(keyOf('Digit2'), { vk: 0x32, ext: false });
  assert.deepEqual(keyOf('ArrowLeft'), { vk: 0x25, ext: true });
  assert.deepEqual(keyOf('Escape'), { vk: 0x1b, ext: false });
  assert.deepEqual(keyOf('F4'), { vk: 0x73, ext: false });
  assert.deepEqual(keyOf('f24'), { vk: 0x87, ext: false });
  assert.deepEqual(keyOf('win'), { vk: 0x5b, ext: true });
  assert.deepEqual(keyOf('/'), { vk: 0xbf, ext: false });
  assert.deepEqual(keyOf('delete'), { vk: 0x2e, ext: true });
  assert.throws(() => keyOf('f25'));
  assert.throws(() => keyOf('hyper'));
});

test('combos press modifiers around the key', () => {
  const ev = comboEvents('ctrl+shift+k');
  assert.deepEqual(ev.map(e => [e.vk, e.up]), [[0x11, false], [0x10, false], [0x4b, false], [0x4b, true], [0x10, true], [0x11, true]]);
  assert.deepEqual(comboEvents('down', 3).map(e => e.up), [false, true, false, true, false, true]);
  assert.deepEqual(comboEvents('win+up').map(e => [e.vk, e.ext, e.up]), [[0x5b, true, false], [0x26, true, false], [0x26, true, true], [0x5b, true, true]]);
  assert.deepEqual(comboEvents('Shift+Digit2').map(e => e.vk), [0x10, 0x32, 0x32, 0x10]);
  assert.throws(() => comboEvents('k+ctrl'), /only the last key/);
  assert.throws(() => comboEvents(''), /empty/);
});

test('drag path starts with a small step, ends on the target, no repeated points', () => {
  const p = dragPath(100, 100, 100, 300, 10);
  assert.deepEqual(p[0], [100, 100]);
  assert.deepEqual(p[1], [100, 106]);
  assert.deepEqual(p.at(-1), [100, 300]);
  for (let i = 1; i < p.length; i++) assert.notDeepEqual(p[i], p[i - 1]);
  assert.deepEqual(dragPath(5, 5, 5, 5), [[5, 5]]);
});

test('calibration maps page coordinates to screen pixels', () => {
  // 150% scaling, page origin at screen 12,40
  const cal = { x: solveAxis(12 + 300, 200, 12 + 900, 600), y: solveAxis(40 + 150, 100, 40 + 600, 400) };
  assert.equal(cal.x.scale, 1.5);
  assert.deepEqual(toScreen(cal, 0, 0), [12, 40]);
  assert.deepEqual(toScreen(cal, 10.4, 20), [28, 70]);
  assert.throws(() => solveAxis(1, 5, 2, 5));
});

test('addresses', () => {
  assert.deepEqual(parseAddr('192.168.1.5', 9231), { host: '192.168.1.5', port: 9231 });
  assert.deepEqual(parseAddr('192.168.1.5:9000', 9231), { host: '192.168.1.5', port: 9000 });
});
