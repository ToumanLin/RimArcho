// Only preview.html maps the Wails bridge to this module. No game API is called.
import { fixture, homeData, shellData, applyRow } from './fixtures.js';
const settings = new Map();
export const Call = { async ByName(name, ...args) {
  const method = name.slice(name.lastIndexOf('.') + 1);
  switch (method) {
    case 'Home': return homeData();
    case 'HomeBackground': return fixture.background === 'plain' ? {} : {url:'/background.png',dim:30,blur:0,focus:'center'};
    case 'Shell': return shellData();
    case 'Setting': return settings.get(args[0]) || '';
    case 'SetSetting': settings.set(args[0], args[1]); return '';
    case 'Leave': return;
    case 'PackLaunch': {
      const row = fixture.rows.find(r => r.kind === 'pack' && r.name === args[0]);
      if (fixture.running) return {refused:['已有游戏在运行'],text:'已有游戏在运行'};
      fixture.running = true; fixture.runningRow = row?.id || fixture.applied;
      return {text:`已提交启动：${row?.name || args[0]}（草案预览）`};
    }
    case 'HomeApply': {
      const row = fixture.rows.find(r => r.kind === args[0] && r.envName === args[1] && (r.kind === 'plan' ? r.plan === args[2] : r.name === args[3]));
      if (row) applyRow(row.id);
      return `已应用 ${row?.name || ''}`;
    }
    default: return null;
  }
}};
export const Events = { On() {} };
