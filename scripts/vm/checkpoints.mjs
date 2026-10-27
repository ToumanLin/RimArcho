import {createHash} from 'node:crypto';
export const projection=d=>({
  ...Object.fromEntries(['environments','plans','sources','instances','lineage_data','lineage_tag','instance_note','plan_collection','user_rule','modsConfigHashes','links'].map(k=>[k,d[k]])),
  problems:{count:d.problems?.count,severity:d.problems?.severity,error:d.problems?.error,
    mods:Object.fromEntries(Object.entries(d.problems?.mods||{}).map(([id,p])=>[id,{count:p.count,severity:p.severity}]))}
});
export const fingerprints=d=>Object.fromEntries(Object.entries(projection(d)).map(([key,value])=>[key,createHash('sha256').update(JSON.stringify(value)??'undefined').digest('hex')]));
