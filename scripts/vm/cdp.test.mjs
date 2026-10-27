import { createServer } from 'node:http';
import { createHash } from 'node:crypto';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { connect } from './cdp.mjs';

async function mock(t) {
  const received=[], sockets=new Set();
  const server=createServer((req,res)=>{res.setHeader('content-type','application/json');res.end(JSON.stringify([{type:'page',url:'http://wails.localhost',webSocketDebuggerUrl:`ws://127.0.0.1:${server.address().port}/devtools/page/1`}]));});
  server.on('upgrade',(req,socket)=>{
    sockets.add(socket);socket.on('close',()=>sockets.delete(socket));
    const accept=createHash('sha1').update(req.headers['sec-websocket-key']+'258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
    socket.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`);
    let buffer=Buffer.alloc(0);
    const send=d=>{const data=Buffer.from(JSON.stringify(d));assert(data.length<126);socket.write(Buffer.concat([Buffer.from([0x81,data.length]),data]));};
    socket.on('data',chunk=>{
      buffer=Buffer.concat([buffer,chunk]);
      while(buffer.length>=6){const opcode=buffer[0]&15;let n=buffer[1]&127,offset=2;if(n===126){if(buffer.length<8)return;n=buffer.readUInt16BE(2);offset=4;}if(n===127)throw Error('unexpected large test frame');if(buffer.length<offset+4+n)return;const mask=buffer.subarray(offset,offset+4),data=buffer.subarray(offset+4,offset+4+n);buffer=buffer.subarray(offset+4+n);for(let i=0;i<data.length;i++)data[i]^=mask[i%4];if(opcode===8){socket.end(Buffer.from([0x88,0]));return;}const d=JSON.parse(data.toString());received.push(d);
        if(d.method==='test.hang')continue;
        if(d.method==='test.disconnect'){socket.destroy();return;}
        if(d.method==='test.error')send({id:d.id,error:{code:-1,message:'fixture protocol error'}});
        else send({id:d.id,result:{}});
      }
    });
  });
  await new Promise(ok=>server.listen(0,'127.0.0.1',ok));
  t.after(()=>{for(const socket of sockets)socket.destroy();server.closeAllConnections();server.close();});
  const c=await connect(`127.0.0.1:${server.address().port}`,{timeoutMs:2000});t.after(()=>c.close());return {c,received};
}
test('Space, Backslash, Shift, held keys and left/right clicks produce real protocol inputs',async t=>{
  const {c,received}=await mock(t);await c.key('Space');await c.key('Control+Backslash');await c.key('Shift+KeyS');await c.key('Shift+KeyS',{uppercase:true,repeats:1});await c.mouse('left',12,34,3);await c.mouse('right',12,34);
  const keys=received.filter(d=>d.method==='Input.dispatchKeyEvent').map(d=>d.params);
  assert.equal(keys[0].key,' ');assert.equal(keys[0].windowsVirtualKeyCode,32);assert.equal(keys[2].key,'\\');assert.equal(keys[2].windowsVirtualKeyCode,220);assert.equal(keys[4].key,'s');assert.equal(keys[4].modifiers,8);assert.equal(keys[6].key,'S');assert.equal(keys[7].autoRepeat,true);
  const clicks=received.filter(d=>d.method==='Input.dispatchMouseEvent'&&d.params.type==='mousePressed').map(d=>d.params);assert.equal(clicks[0].buttons,1);assert.equal(clicks[0].clickCount,3);assert.equal(clicks[1].buttons,2);
});
test('protocol errors and timeouts reject, never silently succeed',async t=>{const {c}=await mock(t);await assert.rejects(c.call('test.error'),/fixture protocol error/);await assert.rejects(c.call('test.hang',{},25),/timed out/);});
test('disconnect rejects every pending call',async t=>{const {c}=await mock(t);const waiting=assert.rejects(c.call('test.hang'),/disconnect/);await assert.rejects(c.call('test.disconnect'),/disconnect/);await waiting;});

test('a denied held repeat releases the physical key and sends no extra keydown',async t=>{
  const {c,received}=await mock(t);
  await assert.rejects(c.key('Space',{repeats:3,beforeRepeat:()=>{throw Error('dialog changed to unknown');}}),/dialog changed/);
  const keys=received.filter(d=>d.method==='Input.dispatchKeyEvent').map(d=>d.params);
  assert.deepEqual(keys.map(k=>k.type),['rawKeyDown','keyUp']);
  assert.equal(keys[0].autoRepeat,false);
});
